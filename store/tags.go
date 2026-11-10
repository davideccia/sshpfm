package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"
)

// ErrTagNotFound is returned when an operation targets a tag id that does
// not exist.
var ErrTagNotFound = errors.New("tag not found")

// MaxTagLabel is the longest Tag label, in characters. It keeps a chip
// short enough to sit beside others in the 300px sidebar.
const MaxTagLabel = 24

// Tag is a named, coloured label attached to Connections.
type Tag struct {
	ID    int64  `json:"id"`
	Label string `json:"label"`

	// Color is a user-chosen "#rrggbb", stored lower-case. The UI only
	// ever paints it as a swatch dot, never behind text, so no value can
	// make a label unreadable.
	Color string `json:"color"`

	// ConnectionCount is read-only and filled by ListTags alone: how many
	// Connections carry the Tag, for the confirmation before deleting it.
	ConnectionCount int `json:"connectionCount,omitempty"`
}

var hexColor = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

// normalize trims the label, lower-cases the colour and reports whether
// the Tag is storable.
func (t *Tag) normalize() error {
	t.Label = strings.TrimSpace(t.Label)
	if t.Label == "" {
		return fmt.Errorf("tag label is required")
	}
	if utf8.RuneCountInString(t.Label) > MaxTagLabel {
		return fmt.Errorf("tag label %q is longer than %d characters", t.Label, MaxTagLabel)
	}
	if !hexColor.MatchString(t.Color) {
		return fmt.Errorf("tag %q: invalid colour %q, want #rrggbb", t.Label, t.Color)
	}
	t.Color = strings.ToLower(t.Color)
	return nil
}

// querier is what both *sql.DB and *sql.Tx offer. Helpers that run inside
// a transaction must use the Tx: the pool holds a single connection, so
// reaching for the DB mid-transaction would block forever.
type querier interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// attachTags fills Tags on each connection, ordered by label.
func attachTags(ctx context.Context, q querier, conns []Connection) error {
	if len(conns) == 0 {
		return nil
	}
	index := make(map[int64]int, len(conns))
	for i := range conns {
		index[conns[i].ID] = i
		if conns[i].Tags == nil {
			conns[i].Tags = []Tag{}
		}
	}

	rows, err := q.QueryContext(ctx, `
		SELECT ct.connection_id, t.id, t.label, t.color
		FROM connection_tags ct JOIN tags t ON t.id = ct.tag_id
		ORDER BY t.label COLLATE NOCASE, t.id`)
	if err != nil {
		return fmt.Errorf("read tags: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var connID int64
		var t Tag
		if err := rows.Scan(&connID, &t.ID, &t.Label, &t.Color); err != nil {
			return fmt.Errorf("read tags: %w", err)
		}
		if i, ok := index[connID]; ok {
			conns[i].Tags = append(conns[i].Tags, t)
		}
	}
	return rows.Err()
}

// resolveTag returns the id of the Tag carrying t's label, creating it with
// t's colour when there is none. An existing Tag keeps its own colour:
// typing a label that already exists means "this Tag", not "recolour it".
func resolveTag(ctx context.Context, q querier, t Tag) (int64, error) {
	if err := t.normalize(); err != nil {
		return 0, err
	}
	var id int64
	err := q.QueryRowContext(ctx, `SELECT id FROM tags WHERE label = ?`, t.Label).Scan(&id)
	if err == nil {
		return id, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, fmt.Errorf("find tag %q: %w", t.Label, err)
	}
	res, err := q.ExecContext(ctx, `INSERT INTO tags (label, color) VALUES (?, ?)`, t.Label, t.Color)
	if err != nil {
		return 0, fmt.Errorf("create tag %q: %w", t.Label, err)
	}
	return res.LastInsertId()
}

// setConnectionTags replaces the connection's Tags with tagIDs plus
// newTags, creating any new Tag that does not exist yet.
func setConnectionTags(ctx context.Context, q querier, connID int64, tagIDs []int64, newTags []Tag) error {
	ids := append([]int64(nil), tagIDs...)
	for _, t := range newTags {
		id, err := resolveTag(ctx, q, t)
		if err != nil {
			return err
		}
		ids = append(ids, id)
	}

	if _, err := q.ExecContext(ctx, `DELETE FROM connection_tags WHERE connection_id = ?`, connID); err != nil {
		return fmt.Errorf("clear tags: %w", err)
	}
	for _, id := range ids {
		var exists bool
		if err := q.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM tags WHERE id = ?)`, id).Scan(&exists); err != nil {
			return fmt.Errorf("attach tag %d: %w", id, err)
		}
		if !exists {
			return fmt.Errorf("attach tag %d: %w", id, ErrTagNotFound)
		}
		// OR IGNORE: a typed label that resolves to a Tag also picked by
		// id is the same Tag, not a conflict.
		if _, err := q.ExecContext(ctx,
			`INSERT OR IGNORE INTO connection_tags (connection_id, tag_id) VALUES (?, ?)`, connID, id,
		); err != nil {
			return fmt.Errorf("attach tag %d: %w", id, err)
		}
	}
	return nil
}

// ListTags returns every Tag, including those no Connection carries, with
// how many Connections use each, ordered by label.
func (s *Store) ListTags(ctx context.Context) ([]Tag, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT t.id, t.label, t.color, COUNT(ct.connection_id)
		FROM tags t LEFT JOIN connection_tags ct ON ct.tag_id = t.id
		GROUP BY t.id
		ORDER BY t.label COLLATE NOCASE, t.id`)
	if err != nil {
		return nil, fmt.Errorf("list tags: %w", err)
	}
	defer rows.Close()

	tags := []Tag{}
	for rows.Next() {
		var t Tag
		if err := rows.Scan(&t.ID, &t.Label, &t.Color, &t.ConnectionCount); err != nil {
			return nil, fmt.Errorf("list tags: %w", err)
		}
		tags = append(tags, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list tags: %w", err)
	}
	return tags, nil
}

// CreateTag adds a Tag that no Connection carries yet, returning its id.
// A label that already exists is refused rather than silently reused: the
// user asked for a new Tag, and getting an old one back would hide that.
func (s *Store) CreateTag(ctx context.Context, t Tag) (int64, error) {
	if err := t.normalize(); err != nil {
		return 0, err
	}
	if err := s.checkLabelFree(ctx, t.Label, 0); err != nil {
		return 0, err
	}
	res, err := s.db.ExecContext(ctx, `INSERT INTO tags (label, color) VALUES (?, ?)`, t.Label, t.Color)
	if err != nil {
		return 0, fmt.Errorf("create tag: %w", err)
	}
	return res.LastInsertId()
}

// UpdateTag renames and recolours a Tag. Renaming onto another Tag's label
// is refused rather than merging the two.
func (s *Store) UpdateTag(ctx context.Context, t Tag) error {
	if t.ID == 0 {
		return fmt.Errorf("id is required")
	}
	if err := t.normalize(); err != nil {
		return err
	}

	if err := s.checkLabelFree(ctx, t.Label, t.ID); err != nil {
		return err
	}

	res, err := s.db.ExecContext(ctx, `UPDATE tags SET label = ?, color = ? WHERE id = ?`, t.Label, t.Color, t.ID)
	if err != nil {
		return fmt.Errorf("update tag %d: %w", t.ID, err)
	}
	return checkTagAffected(res, t.ID, "update")
}

// checkLabelFree refuses a label another Tag already carries, compared
// case-insensitively through the column's NOCASE collation. exceptID is
// the Tag being renamed, or 0 for a new one. The UNIQUE index would catch
// the clash too, but with a message no user should read.
func (s *Store) checkLabelFree(ctx context.Context, label string, exceptID int64) error {
	var clash bool
	if err := s.db.QueryRowContext(ctx,
		`SELECT EXISTS (SELECT 1 FROM tags WHERE label = ? AND id != ?)`, label, exceptID,
	).Scan(&clash); err != nil {
		return fmt.Errorf("check tag label %q: %w", label, err)
	}
	if clash {
		return fmt.Errorf("a tag named %q already exists", label)
	}
	return nil
}

// DeleteTag removes a Tag. The Connections that carried it lose the link
// and nothing else.
func (s *Store) DeleteTag(ctx context.Context, id int64) error {
	if id == 0 {
		return fmt.Errorf("id is required")
	}
	res, err := s.db.ExecContext(ctx, `DELETE FROM tags WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete tag %d: %w", id, err)
	}
	return checkTagAffected(res, id, "delete")
}

func checkTagAffected(res sql.Result, id int64, op string) error {
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("%s tag %d: %w", op, id, err)
	}
	if n == 0 {
		return fmt.Errorf("%s tag %d: %w", op, id, ErrTagNotFound)
	}
	return nil
}
