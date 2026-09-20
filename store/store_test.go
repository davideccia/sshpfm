package store

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func openTest(t *testing.T) *Store {
	t.Helper()
	// A directory with a space catches DSN-escaping regressions: the real
	// macOS config dir is "~/Library/Application Support/sshpfm".
	dir := filepath.Join(t.TempDir(), "with space")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	s, err := Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func sample() Connection {
	return Connection{
		Label:          "staging db",
		Host:           "bastion.example.com",
		SSHPort:        22,
		Username:       "deploy",
		RemoteBindPort: 5432,
		LocalPort:      15432,
	}
}

func TestCreateGetRoundTrip(t *testing.T) {
	ctx := context.Background()
	s := openTest(t)

	want := sample()
	id, err := s.Create(ctx, want)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if id == 0 {
		t.Fatal("Create returned id 0")
	}

	got, err := s.Get(ctx, id)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	want.ID = id
	if got != want {
		t.Errorf("Get = %+v, want %+v", got, want)
	}
}

func TestCreateDefaultsSSHPort(t *testing.T) {
	ctx := context.Background()
	s := openTest(t)

	c := sample()
	c.SSHPort = 0
	id, err := s.Create(ctx, c)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	got, err := s.Get(ctx, id)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.SSHPort != 22 {
		t.Errorf("SSHPort = %d, want 22", got.SSHPort)
	}
}

func TestValidation(t *testing.T) {
	ctx := context.Background()
	s := openTest(t)

	tests := []struct {
		name   string
		mutate func(*Connection)
	}{
		{"empty label", func(c *Connection) { c.Label = "" }},
		{"empty host", func(c *Connection) { c.Host = "" }},
		{"empty username", func(c *Connection) { c.Username = "" }},
		{"ssh port too high", func(c *Connection) { c.SSHPort = 70000 }},
		{"ssh port negative", func(c *Connection) { c.SSHPort = -1 }},
		{"remote port zero", func(c *Connection) { c.RemoteBindPort = 0 }},
		{"local port zero", func(c *Connection) { c.LocalPort = 0 }},
		{"local port too high", func(c *Connection) { c.LocalPort = 65536 }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := sample()
			tt.mutate(&c)
			if _, err := s.Create(ctx, c); err == nil {
				t.Fatalf("Create(%+v) = nil error, want validation failure", c)
			}
		})
	}
}

func TestListOrdersByLabel(t *testing.T) {
	ctx := context.Background()
	s := openTest(t)

	got, err := s.List(ctx)
	if err != nil {
		t.Fatalf("List on empty store: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("List on empty store = %d rows, want 0", len(got))
	}

	// Inserted out of order, and with mixed case: the ordering is neither
	// insertion order nor ASCII order, where every uppercase label would
	// sort ahead of every lowercase one.
	for _, label := range []string{"c", "Ab", "b", "aa"} {
		c := sample()
		c.Label = label
		if _, err := s.Create(ctx, c); err != nil {
			t.Fatalf("Create %s: %v", label, err)
		}
	}

	got, err = s.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	want := []string{"aa", "Ab", "b", "c"}
	if len(got) != len(want) {
		t.Fatalf("List = %d rows, want %d", len(got), len(want))
	}
	for i, c := range got {
		if c.Label != want[i] {
			t.Errorf("row %d label = %q, want %q", i, c.Label, want[i])
		}
	}
}

func TestUpdate(t *testing.T) {
	ctx := context.Background()
	s := openTest(t)

	id, err := s.Create(ctx, sample())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	c := sample()
	c.ID = id
	c.Label = "renamed"
	c.LocalPort = 19999
	if err := s.Update(ctx, c); err != nil {
		t.Fatalf("Update: %v", err)
	}

	got, err := s.Get(ctx, id)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got != c {
		t.Errorf("Get = %+v, want %+v", got, c)
	}
}

func TestDelete(t *testing.T) {
	ctx := context.Background()
	s := openTest(t)

	id, err := s.Create(ctx, sample())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := s.Delete(ctx, id); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := s.Get(ctx, id); !errors.Is(err, ErrNotFound) {
		t.Errorf("Get after Delete = %v, want ErrNotFound", err)
	}
}

func TestMissingIDReportsNotFound(t *testing.T) {
	ctx := context.Background()
	s := openTest(t)

	if _, err := s.Get(ctx, 404); !errors.Is(err, ErrNotFound) {
		t.Errorf("Get(404) = %v, want ErrNotFound", err)
	}

	c := sample()
	c.ID = 404
	if err := s.Update(ctx, c); !errors.Is(err, ErrNotFound) {
		t.Errorf("Update(404) = %v, want ErrNotFound", err)
	}

	if err := s.Delete(ctx, 404); !errors.Is(err, ErrNotFound) {
		t.Errorf("Delete(404) = %v, want ErrNotFound", err)
	}
}

func TestZeroIDRejected(t *testing.T) {
	ctx := context.Background()
	s := openTest(t)

	if err := s.Update(ctx, sample()); err == nil {
		t.Error("Update with id 0 = nil error, want failure")
	}
	if err := s.Delete(ctx, 0); err == nil {
		t.Error("Delete with id 0 = nil error, want failure")
	}
}
