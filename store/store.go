package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"

	"sshpfm/internal/vault"

	_ "modernc.org/sqlite"
)

// ErrNotFound is returned when an operation targets a connection id that
// does not exist.
var ErrNotFound = errors.New("connection not found")

// ErrNoVault is returned by Vault when the master key has never been set.
var ErrNoVault = errors.New("no vault configured")

// ErrLocked is returned when an operation needs the master key and Unlock
// has not been called.
var ErrLocked = errors.New("vault is locked")

// Connection represents a local SSH port-forwarding profile:
// ssh -L LocalPort:localhost:RemoteBindPort Username@Host:SSHPort
type Connection struct {
	ID             int64  `json:"id"`
	Label          string `json:"label"`
	Host           string `json:"host"`
	SSHPort        int    `json:"sshPort"`
	Username       string `json:"username"`
	RemoteBindPort int    `json:"remoteBindPort"`
	LocalPort      int    `json:"localPort"`

	// Password is write-only across the Wails bridge: Redacted drops it
	// before a profile reaches the frontend. On Update, nil means "leave
	// the stored password alone" and a pointer to "" means "remove it".
	Password *string `json:"password"`

	// HasPassword is read-only. It tells the UI that a password exists
	// without ever sending the secret itself.
	HasPassword bool `json:"hasPassword"`
}

// Redacted returns a copy safe to hand to the frontend: the secret is
// dropped and only its presence survives.
func (c Connection) Redacted() Connection {
	c.Password = nil
	return c
}

// validate applies defaults and reports whether the profile is usable.
func (c *Connection) validate() error {
	if c.Label == "" {
		return fmt.Errorf("label is required")
	}
	if c.Host == "" {
		return fmt.Errorf("host is required")
	}
	if c.Username == "" {
		return fmt.Errorf("username is required")
	}
	if c.SSHPort == 0 {
		c.SSHPort = 22
	}
	if !validPort(c.SSHPort) {
		return fmt.Errorf("invalid ssh port: %d", c.SSHPort)
	}
	if !validPort(c.RemoteBindPort) {
		return fmt.Errorf("invalid remote bind port: %d", c.RemoteBindPort)
	}
	if !validPort(c.LocalPort) {
		return fmt.Errorf("invalid local port: %d", c.LocalPort)
	}
	return nil
}

func validPort(p int) bool {
	return p >= 1 && p <= 65535
}

// Store persists Connection profiles in a local SQLite database.
//
// It holds the derived master key rather than leaving encryption to its
// callers: sealing inside the store is what guarantees no code path can
// write an SSH password to disk in the clear.
type Store struct {
	db  *sql.DB
	key *vault.Key
}

// migrations are applied in order, and PRAGMA user_version records how
// many have run. Databases written before the pragma existed report 0,
// so the first step is the original schema and stays idempotent.
//
// Never edit an entry that has shipped; append a new one instead.
var migrations = []string{
	`CREATE TABLE IF NOT EXISTS connections (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		label TEXT NOT NULL,
		host TEXT NOT NULL,
		ssh_port INTEGER NOT NULL,
		username TEXT NOT NULL,
		remote_bind_port INTEGER NOT NULL,
		local_port INTEGER NOT NULL
	);`,

	`ALTER TABLE connections ADD COLUMN password_enc BLOB;`,

	// CHECK (id = 1) is the SQLite idiom for a table that may only ever
	// hold one row.
	`CREATE TABLE IF NOT EXISTS vault (
		id            INTEGER PRIMARY KEY CHECK (id = 1),
		salt          BLOB NOT NULL,
		verifier      BLOB NOT NULL,
		argon_time    INTEGER NOT NULL,
		argon_memory  INTEGER NOT NULL,
		argon_threads INTEGER NOT NULL
	);`,
}

// Open creates (if needed) and opens the SQLite database at path,
// bringing its schema up to date.
func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", dsn(path))
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	// A desktop app has a single writer; one connection avoids SQLITE_BUSY
	// entirely and keeps the WAL/pragma state consistent across queries.
	db.SetMaxOpenConns(1)

	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("open sqlite %s: %w", path, err)
	}

	if err := migrate(db); err != nil {
		db.Close()
		return nil, err
	}

	return &Store{db: db}, nil
}

// migrate runs every migration the database has not seen yet.
func migrate(db *sql.DB) error {
	var version int
	if err := db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		return fmt.Errorf("read schema version: %w", err)
	}

	for i := version; i < len(migrations); i++ {
		if _, err := db.Exec(migrations[i]); err != nil {
			return fmt.Errorf("migrate schema step %d: %w", i, err)
		}
	}

	// PRAGMA does not accept bind parameters, so the value is formatted
	// in. It is the length of a package-level slice, never user input.
	if _, err := db.Exec(fmt.Sprintf(`PRAGMA user_version = %d`, len(migrations))); err != nil {
		return fmt.Errorf("set schema version: %w", err)
	}
	return nil
}

// Unlock installs the derived master key, enabling the password column.
// Until it is called, reading a profile that has a password fails with
// ErrLocked rather than silently returning a profile without one.
func (s *Store) Unlock(k vault.Key) {
	s.key = &k
}

// Lock drops the master key. It is called at shutdown so the key does not
// outlive the window.
func (s *Store) Lock() {
	s.key = nil
}

// IsUnlocked reports whether the master key is loaded.
func (s *Store) IsUnlocked() bool {
	return s.key != nil
}

// dsn builds a modernc.org/sqlite connection string with the pragmas the
// app relies on. The path is escaped so spaces in it survive the URI form.
func dsn(path string) string {
	return "file:" + url.PathEscape(path) +
		"?_pragma=busy_timeout(5000)" +
		"&_pragma=journal_mode(WAL)" +
		"&_pragma=foreign_keys(1)"
}

// Close releases the underlying database handle.
func (s *Store) Close() error {
	return s.db.Close()
}

const selectColumns = `SELECT id, label, host, ssh_port, username, remote_bind_port, local_port, password_enc FROM connections`

// scanConnection reads one row and decrypts its password, if any.
func (s *Store) scanConnection(row interface{ Scan(...any) error }) (Connection, error) {
	var c Connection
	var enc []byte
	if err := row.Scan(&c.ID, &c.Label, &c.Host, &c.SSHPort, &c.Username, &c.RemoteBindPort, &c.LocalPort, &enc); err != nil {
		return Connection{}, err
	}
	if len(enc) == 0 {
		return c, nil
	}

	c.HasPassword = true
	if s.key == nil {
		return Connection{}, ErrLocked
	}
	password, err := s.key.Open(enc)
	if err != nil {
		return Connection{}, fmt.Errorf("connection %d: %w", c.ID, err)
	}
	c.Password = &password
	return c, nil
}

// List returns all stored connections ordered by label, case-insensitively.
// The id tiebreak keeps the order stable when two connections share a label.
func (s *Store) List(ctx context.Context) ([]Connection, error) {
	rows, err := s.db.QueryContext(ctx, selectColumns+` ORDER BY label COLLATE NOCASE, id`)
	if err != nil {
		return nil, fmt.Errorf("list connections: %w", err)
	}
	defer rows.Close()

	conns := []Connection{}
	for rows.Next() {
		c, err := s.scanConnection(rows)
		if err != nil {
			return nil, fmt.Errorf("scan connection: %w", err)
		}
		conns = append(conns, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list connections: %w", err)
	}
	return conns, nil
}

// Get returns a single connection by id, or ErrNotFound.
func (s *Store) Get(ctx context.Context, id int64) (Connection, error) {
	row := s.db.QueryRowContext(ctx, selectColumns+` WHERE id = ?`, id)
	c, err := s.scanConnection(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Connection{}, fmt.Errorf("get connection %d: %w", id, ErrNotFound)
	}
	if err != nil {
		return Connection{}, fmt.Errorf("get connection %d: %w", id, err)
	}
	return c, nil
}

// passwordArg turns the write-only password into a SQL argument. An
// absent or empty password is stored as NULL, so "" never reaches a row
// and HasPassword stays honest.
func (s *Store) passwordArg(p *string) (any, error) {
	if p == nil || *p == "" {
		return nil, nil
	}
	if s.key == nil {
		return nil, ErrLocked
	}
	return s.key.Seal(*p)
}

// Create validates and inserts a new connection, returning its assigned id.
func (s *Store) Create(ctx context.Context, c Connection) (int64, error) {
	if err := c.validate(); err != nil {
		return 0, err
	}
	enc, err := s.passwordArg(c.Password)
	if err != nil {
		return 0, fmt.Errorf("create connection: %w", err)
	}
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO connections (label, host, ssh_port, username, remote_bind_port, local_port, password_enc) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		c.Label, c.Host, c.SSHPort, c.Username, c.RemoteBindPort, c.LocalPort, enc,
	)
	if err != nil {
		return 0, fmt.Errorf("create connection: %w", err)
	}
	return res.LastInsertId()
}

// The two UPDATE statements differ only in whether they touch the
// password. Branching in Go rather than writing a CASE keeps both
// statements readable and keeps "leave it alone" from depending on SQL
// subtleties.
const (
	updateSQL = `UPDATE connections SET label = ?, host = ?, ssh_port = ?, username = ?, remote_bind_port = ?, local_port = ? WHERE id = ?`

	updateWithPasswordSQL = `UPDATE connections SET label = ?, host = ?, ssh_port = ?, username = ?, remote_bind_port = ?, local_port = ?, password_enc = ? WHERE id = ?`
)

// Update validates and persists changes to an existing connection.
// It returns ErrNotFound when no row carries the given id.
//
// A nil Password leaves the stored one untouched, which is what lets the
// edit form show an empty password field without wiping the secret.
func (s *Store) Update(ctx context.Context, c Connection) error {
	if c.ID == 0 {
		return fmt.Errorf("id is required")
	}
	if err := c.validate(); err != nil {
		return err
	}

	query := updateSQL
	args := []any{c.Label, c.Host, c.SSHPort, c.Username, c.RemoteBindPort, c.LocalPort}
	if c.Password != nil {
		enc, err := s.passwordArg(c.Password)
		if err != nil {
			return fmt.Errorf("update connection %d: %w", c.ID, err)
		}
		query = updateWithPasswordSQL
		args = append(args, enc)
	}
	args = append(args, c.ID)

	res, err := s.db.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("update connection %d: %w", c.ID, err)
	}
	return checkAffected(res, c.ID, "update")
}

// Delete removes a connection by id, returning ErrNotFound when it is absent.
func (s *Store) Delete(ctx context.Context, id int64) error {
	if id == 0 {
		return fmt.Errorf("id is required")
	}
	res, err := s.db.ExecContext(ctx, `DELETE FROM connections WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete connection %d: %w", id, err)
	}
	return checkAffected(res, id, "delete")
}

// VaultRecord is everything needed to check a master password and derive
// the same key again: the salt, the verifier, and the Argon2id cost the
// vault was created with.
type VaultRecord struct {
	Salt     []byte
	Verifier []byte
	Params   vault.Params
}

// Vault returns the stored vault metadata, or ErrNoVault when the master
// key has never been set.
func (s *Store) Vault(ctx context.Context) (VaultRecord, error) {
	var v VaultRecord
	row := s.db.QueryRowContext(ctx, `SELECT salt, verifier, argon_time, argon_memory, argon_threads FROM vault WHERE id = 1`)
	err := row.Scan(&v.Salt, &v.Verifier, &v.Params.Time, &v.Params.Memory, &v.Params.Threads)
	if errors.Is(err, sql.ErrNoRows) {
		return VaultRecord{}, ErrNoVault
	}
	if err != nil {
		return VaultRecord{}, fmt.Errorf("read vault: %w", err)
	}
	return v, nil
}

// SetVault stores the vault metadata, replacing any previous one.
func (s *Store) SetVault(ctx context.Context, v VaultRecord) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT OR REPLACE INTO vault (id, salt, verifier, argon_time, argon_memory, argon_threads) VALUES (1, ?, ?, ?, ?, ?)`,
		v.Salt, v.Verifier, v.Params.Time, v.Params.Memory, v.Params.Threads,
	)
	if err != nil {
		return fmt.Errorf("write vault: %w", err)
	}
	return nil
}

// ResetVault forgets the master key and every secret encrypted under it.
// The connection profiles survive: their passwords are unrecoverable
// without the key anyway, and re-entering host and port details would be
// a pointless second loss.
//
// Both statements run in one transaction so a crash cannot leave
// ciphertext behind that nothing can ever decrypt.
func (s *Store) ResetVault(ctx context.Context) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("reset vault: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `UPDATE connections SET password_enc = NULL`); err != nil {
		return fmt.Errorf("reset vault: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM vault`); err != nil {
		return fmt.Errorf("reset vault: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("reset vault: %w", err)
	}

	s.Lock()
	return nil
}

func checkAffected(res sql.Result, id int64, op string) error {
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("%s connection %d: %w", op, id, err)
	}
	if n == 0 {
		return fmt.Errorf("%s connection %d: %w", op, id, ErrNotFound)
	}
	return nil
}
