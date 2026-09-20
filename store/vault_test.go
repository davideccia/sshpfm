package store

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"sshpfm/internal/vault"
)

// testParams keep the suite fast; vault.DefaultParams costs 64 MiB per
// derivation. The store does not care which parameters it is handed.
var testParams = vault.Params{Time: 1, Memory: 8 * 1024, Threads: 1}

// unlockTest opens a store and installs a master key, which is the state
// every screen of the app runs in.
func unlockTest(t *testing.T) *Store {
	t.Helper()
	s := openTest(t)
	s.Unlock(testKey(t, "correct horse"))
	return s
}

func testKey(t *testing.T, password string) vault.Key {
	t.Helper()
	// A fixed salt keeps derivations comparable across helpers in one test.
	salt := make([]byte, vault.SaltLen)
	k, _, err := vault.Derive(password, salt, testParams)
	if err != nil {
		t.Fatalf("Derive: %v", err)
	}
	return k
}

func withPassword(c Connection, password string) Connection {
	c.Password = &password
	return c
}

func TestUnlockStateReported(t *testing.T) {
	s := openTest(t)
	if s.IsUnlocked() {
		t.Error("IsUnlocked = true before Unlock")
	}
	s.Unlock(testKey(t, "correct horse"))
	if !s.IsUnlocked() {
		t.Error("IsUnlocked = false after Unlock")
	}
	s.Lock()
	if s.IsUnlocked() {
		t.Error("IsUnlocked = true after Lock")
	}
}

func TestPasswordRoundTrip(t *testing.T) {
	ctx := context.Background()
	s := unlockTest(t)

	id, err := s.Create(ctx, withPassword(sample(), "hunter2"))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := s.Get(ctx, id)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !got.HasPassword {
		t.Error("HasPassword = false, want true")
	}
	if got.Password == nil {
		t.Fatal("Password = nil, want the stored secret")
	}
	if *got.Password != "hunter2" {
		t.Errorf("Password = %q, want %q", *got.Password, "hunter2")
	}
}

func TestCreateWithoutPasswordStoresNull(t *testing.T) {
	ctx := context.Background()
	s := unlockTest(t)

	id, err := s.Create(ctx, sample())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := s.Get(ctx, id)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.HasPassword {
		t.Error("HasPassword = true for a profile created without one")
	}
	if got.Password != nil {
		t.Errorf("Password = %q, want nil", *got.Password)
	}
}

// An empty string means "no password", not a password that is the empty
// string; it must not produce a row the UI would report as having one.
func TestCreateWithEmptyPasswordStoresNull(t *testing.T) {
	ctx := context.Background()
	s := unlockTest(t)

	id, err := s.Create(ctx, withPassword(sample(), ""))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := s.Get(ctx, id)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.HasPassword {
		t.Error("HasPassword = true for an empty password")
	}
}

// This is what the edit form relies on: saving with the password field
// left blank must not wipe the stored secret.
func TestUpdateWithNilPasswordKeepsStored(t *testing.T) {
	ctx := context.Background()
	s := unlockTest(t)

	id, err := s.Create(ctx, withPassword(sample(), "hunter2"))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	c := sample()
	c.ID = id
	c.Label = "renamed"
	c.Password = nil
	if err := s.Update(ctx, c); err != nil {
		t.Fatalf("Update: %v", err)
	}

	got, err := s.Get(ctx, id)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Label != "renamed" {
		t.Errorf("Label = %q, want %q", got.Label, "renamed")
	}
	if got.Password == nil || *got.Password != "hunter2" {
		t.Errorf("Password = %v, want %q to survive the update", got.Password, "hunter2")
	}
}

func TestUpdateWithNewPasswordReplacesStored(t *testing.T) {
	ctx := context.Background()
	s := unlockTest(t)

	id, err := s.Create(ctx, withPassword(sample(), "hunter2"))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	c := withPassword(sample(), "swordfish")
	c.ID = id
	if err := s.Update(ctx, c); err != nil {
		t.Fatalf("Update: %v", err)
	}

	got, err := s.Get(ctx, id)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Password == nil || *got.Password != "swordfish" {
		t.Errorf("Password = %v, want %q", got.Password, "swordfish")
	}
}

func TestUpdateWithEmptyPasswordClears(t *testing.T) {
	ctx := context.Background()
	s := unlockTest(t)

	id, err := s.Create(ctx, withPassword(sample(), "hunter2"))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	c := withPassword(sample(), "")
	c.ID = id
	if err := s.Update(ctx, c); err != nil {
		t.Fatalf("Update: %v", err)
	}

	got, err := s.Get(ctx, id)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.HasPassword {
		t.Error("HasPassword = true after clearing")
	}
	if got.Password != nil {
		t.Errorf("Password = %q, want nil after clearing", *got.Password)
	}
}

func TestReadingSecretWhileLockedFails(t *testing.T) {
	ctx := context.Background()
	s := unlockTest(t)

	if _, err := s.Create(ctx, withPassword(sample(), "hunter2")); err != nil {
		t.Fatalf("Create: %v", err)
	}
	s.Lock()

	if _, err := s.List(ctx); !errors.Is(err, ErrLocked) {
		t.Errorf("List while locked = %v, want ErrLocked", err)
	}
}

func TestWritingSecretWhileLockedFails(t *testing.T) {
	ctx := context.Background()
	s := openTest(t)

	if _, err := s.Create(ctx, withPassword(sample(), "hunter2")); !errors.Is(err, ErrLocked) {
		t.Errorf("Create while locked = %v, want ErrLocked", err)
	}
}

// Profiles without secrets stay readable while locked; nothing in them
// needs the key.
func TestReadingWithoutSecretWhileLockedWorks(t *testing.T) {
	ctx := context.Background()
	s := unlockTest(t)

	if _, err := s.Create(ctx, sample()); err != nil {
		t.Fatalf("Create: %v", err)
	}
	s.Lock()

	conns, err := s.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(conns) != 1 {
		t.Fatalf("len(List) = %d, want 1", len(conns))
	}
}

func TestSecretIsNotStoredInTheClear(t *testing.T) {
	ctx := context.Background()
	s := unlockTest(t)

	id, err := s.Create(ctx, withPassword(sample(), "hunter2"))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	var enc []byte
	if err := s.db.QueryRow(`SELECT password_enc FROM connections WHERE id = ?`, id).Scan(&enc); err != nil {
		t.Fatalf("read column: %v", err)
	}
	if len(enc) == 0 {
		t.Fatal("password_enc is empty")
	}
	if string(enc) == "hunter2" {
		t.Error("password_enc holds the plaintext")
	}
}

func TestRedactedDropsPassword(t *testing.T) {
	password := "hunter2"
	c := Connection{ID: 1, Label: "x", Password: &password, HasPassword: true}

	got := c.Redacted()
	if got.Password != nil {
		t.Errorf("Password = %q, want nil", *got.Password)
	}
	if !got.HasPassword {
		t.Error("HasPassword = false, want the flag to survive redaction")
	}
	if c.Password == nil {
		t.Error("Redacted mutated its receiver")
	}
}

func TestVaultRoundTrip(t *testing.T) {
	ctx := context.Background()
	s := openTest(t)

	if _, err := s.Vault(ctx); !errors.Is(err, ErrNoVault) {
		t.Errorf("Vault on a fresh database = %v, want ErrNoVault", err)
	}

	want := VaultRecord{Salt: []byte("0123456789abcdef"), Verifier: []byte("verifier"), Params: testParams}
	if err := s.SetVault(ctx, want); err != nil {
		t.Fatalf("SetVault: %v", err)
	}

	got, err := s.Vault(ctx)
	if err != nil {
		t.Fatalf("Vault: %v", err)
	}
	if string(got.Salt) != string(want.Salt) {
		t.Errorf("Salt = %q, want %q", got.Salt, want.Salt)
	}
	if string(got.Verifier) != string(want.Verifier) {
		t.Errorf("Verifier = %q, want %q", got.Verifier, want.Verifier)
	}
	if got.Params != want.Params {
		t.Errorf("Params = %+v, want %+v", got.Params, want.Params)
	}
}

func TestSetVaultReplacesPrevious(t *testing.T) {
	ctx := context.Background()
	s := openTest(t)

	first := VaultRecord{Salt: []byte("0123456789abcdef"), Verifier: []byte("one"), Params: testParams}
	if err := s.SetVault(ctx, first); err != nil {
		t.Fatalf("SetVault: %v", err)
	}
	second := VaultRecord{Salt: []byte("fedcba9876543210"), Verifier: []byte("two"), Params: testParams}
	if err := s.SetVault(ctx, second); err != nil {
		t.Fatalf("SetVault: %v", err)
	}

	got, err := s.Vault(ctx)
	if err != nil {
		t.Fatalf("Vault: %v", err)
	}
	if string(got.Verifier) != "two" {
		t.Errorf("Verifier = %q, want %q", got.Verifier, "two")
	}

	var rows int
	if err := s.db.QueryRow(`SELECT count(*) FROM vault`).Scan(&rows); err != nil {
		t.Fatalf("count vault: %v", err)
	}
	if rows != 1 {
		t.Errorf("vault rows = %d, want 1", rows)
	}
}

func TestResetVaultDropsPasswordsKeepsConnections(t *testing.T) {
	ctx := context.Background()
	s := unlockTest(t)

	if err := s.SetVault(ctx, VaultRecord{Salt: []byte("0123456789abcdef"), Verifier: []byte("v"), Params: testParams}); err != nil {
		t.Fatalf("SetVault: %v", err)
	}
	id, err := s.Create(ctx, withPassword(sample(), "hunter2"))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := s.ResetVault(ctx); err != nil {
		t.Fatalf("ResetVault: %v", err)
	}

	if _, err := s.Vault(ctx); !errors.Is(err, ErrNoVault) {
		t.Errorf("Vault after reset = %v, want ErrNoVault", err)
	}
	if s.IsUnlocked() {
		t.Error("IsUnlocked = true after reset")
	}

	// The profile survives, and it is readable without a key because it
	// no longer holds a secret.
	got, err := s.Get(ctx, id)
	if err != nil {
		t.Fatalf("Get after reset: %v", err)
	}
	if got.Host != sample().Host {
		t.Errorf("Host = %q, want %q", got.Host, sample().Host)
	}
	if got.HasPassword {
		t.Error("HasPassword = true after reset")
	}
}

// A database written by an earlier version reports user_version 0. Open
// must bring it forward without touching the rows already in it.
func TestMigrationUpgradesLegacyDatabase(t *testing.T) {
	ctx := context.Background()
	dir := filepath.Join(t.TempDir(), "with space")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	path := filepath.Join(dir, "legacy.db")

	legacy, err := sql.Open("sqlite", dsn(path))
	if err != nil {
		t.Fatalf("open legacy: %v", err)
	}
	if _, err := legacy.Exec(migrations[0]); err != nil {
		t.Fatalf("create legacy schema: %v", err)
	}
	if _, err := legacy.Exec(
		`INSERT INTO connections (label, host, ssh_port, username, remote_bind_port, local_port) VALUES ('old', 'legacy.example.com', 22, 'deploy', 5432, 15432)`,
	); err != nil {
		t.Fatalf("seed legacy row: %v", err)
	}
	if err := legacy.Close(); err != nil {
		t.Fatalf("close legacy: %v", err)
	}

	s, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	var version int
	if err := s.db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		t.Fatalf("read user_version: %v", err)
	}
	if version != len(migrations) {
		t.Errorf("user_version = %d, want %d", version, len(migrations))
	}

	conns, err := s.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(conns) != 1 {
		t.Fatalf("len(List) = %d, want 1", len(conns))
	}
	if conns[0].Label != "old" {
		t.Errorf("Label = %q, want %q", conns[0].Label, "old")
	}
	if conns[0].HasPassword {
		t.Error("HasPassword = true for a migrated row")
	}

	// The new column is usable straight away.
	s.Unlock(testKey(t, "correct horse"))
	c := conns[0]
	password := "hunter2"
	c.Password = &password
	if err := s.Update(ctx, c); err != nil {
		t.Fatalf("Update after migration: %v", err)
	}
}

// Running Open twice must not re-run migrations that already applied.
func TestMigrationIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.db")

	for i := range 2 {
		s, err := Open(path)
		if err != nil {
			t.Fatalf("Open %d: %v", i, err)
		}
		s.Close()
	}
}
