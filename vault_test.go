package main

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"sshpfm/internal/tunnel"
	"sshpfm/store"
)

// newTestApp builds an App far enough to exercise the vault bindings.
// Those paths never emit events, so a plain context stands in for the
// Wails one, and the manager is only needed because ResetVault stops
// every tunnel first.
func newTestApp(t *testing.T) *App {
	t.Helper()

	s, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { s.Close() })

	return &App{
		ctx:     context.Background(),
		store:   s,
		tunnels: tunnel.NewManager(filepath.Join(t.TempDir(), "known_hosts"), nil, nil),
	}
}

func vaultState(t *testing.T, a *App) string {
	t.Helper()
	got, err := a.VaultState()
	if err != nil {
		t.Fatalf("VaultState: %v", err)
	}
	return got
}

func TestVaultLifecycle(t *testing.T) {
	a := newTestApp(t)

	if got := vaultState(t, a); got != vaultUnset {
		t.Fatalf("VaultState on a fresh database = %q, want %q", got, vaultUnset)
	}

	if err := a.CreateVault("correct horse"); err != nil {
		t.Fatalf("CreateVault: %v", err)
	}
	if got := vaultState(t, a); got != vaultUnlocked {
		t.Errorf("VaultState after CreateVault = %q, want %q", got, vaultUnlocked)
	}

	// Locking stands in for a restart: the metadata is on disk, the key
	// is not.
	a.store.Lock()
	if got := vaultState(t, a); got != vaultLocked {
		t.Errorf("VaultState after locking = %q, want %q", got, vaultLocked)
	}

	if err := a.UnlockVault("correct horse"); err != nil {
		t.Fatalf("UnlockVault: %v", err)
	}
	if got := vaultState(t, a); got != vaultUnlocked {
		t.Errorf("VaultState after UnlockVault = %q, want %q", got, vaultUnlocked)
	}
}

func TestUnlockWithWrongPasswordFails(t *testing.T) {
	a := newTestApp(t)

	if err := a.CreateVault("correct horse"); err != nil {
		t.Fatalf("CreateVault: %v", err)
	}
	a.store.Lock()

	if err := a.UnlockVault("correct zebra"); err == nil {
		t.Fatal("UnlockVault with the wrong password = nil error, want failure")
	}
	if got := vaultState(t, a); got != vaultLocked {
		t.Errorf("VaultState after a failed unlock = %q, want %q", got, vaultLocked)
	}
}

func TestCreateVaultRejectsShortPassword(t *testing.T) {
	a := newTestApp(t)

	if err := a.CreateVault("short"); err == nil {
		t.Fatal("CreateVault with a short password = nil error, want failure")
	}
	// A rejected attempt must not leave a half-built vault behind.
	if got := vaultState(t, a); got != vaultUnset {
		t.Errorf("VaultState = %q, want %q", got, vaultUnset)
	}
}

// Overwriting the vault would orphan every secret encrypted under the
// old key, so it is refused rather than silently destructive.
func TestCreateVaultRefusesToReplace(t *testing.T) {
	a := newTestApp(t)

	if err := a.CreateVault("correct horse"); err != nil {
		t.Fatalf("CreateVault: %v", err)
	}
	if err := a.CreateVault("correct zebra"); err == nil {
		t.Fatal("CreateVault over an existing vault = nil error, want failure")
	}

	// The original password still opens it.
	a.store.Lock()
	if err := a.UnlockVault("correct horse"); err != nil {
		t.Errorf("UnlockVault with the original password: %v", err)
	}
}

func TestDataBindingsRefuseWhileLocked(t *testing.T) {
	a := newTestApp(t)

	if err := a.CreateVault("correct horse"); err != nil {
		t.Fatalf("CreateVault: %v", err)
	}
	a.store.Lock()

	if _, err := a.ListConnections(); !errors.Is(err, errLocked) {
		t.Errorf("ListConnections while locked = %v, want errLocked", err)
	}
	if err := a.StartTunnel(1); !errors.Is(err, errLocked) {
		t.Errorf("StartTunnel while locked = %v, want errLocked", err)
	}
	if _, err := a.TunnelStatuses(); !errors.Is(err, errLocked) {
		t.Errorf("TunnelStatuses while locked = %v, want errLocked", err)
	}
}

func TestResetVaultClearsSecretsAndKeepsConnections(t *testing.T) {
	a := newTestApp(t)

	if err := a.CreateVault("correct horse"); err != nil {
		t.Fatalf("CreateVault: %v", err)
	}

	password := "hunter2"
	id, err := a.CreateConnection(store.Connection{
		Label: "staging db", Host: "bastion.example.com", SSHPort: 22,
		Username: "deploy", RemoteBindPort: 5432, LocalPort: 15432,
		Password: &password,
	})
	if err != nil {
		t.Fatalf("CreateConnection: %v", err)
	}

	if err := a.ResetVault(); err != nil {
		t.Fatalf("ResetVault: %v", err)
	}
	if got := vaultState(t, a); got != vaultUnset {
		t.Errorf("VaultState after reset = %q, want %q", got, vaultUnset)
	}

	// The profile survives the reset, minus its secret.
	if err := a.CreateVault("brand new one"); err != nil {
		t.Fatalf("CreateVault after reset: %v", err)
	}
	conns, err := a.ListConnections()
	if err != nil {
		t.Fatalf("ListConnections: %v", err)
	}
	if len(conns) != 1 {
		t.Fatalf("len(ListConnections) = %d, want 1", len(conns))
	}
	if conns[0].ID != id {
		t.Errorf("id = %d, want %d", conns[0].ID, id)
	}
	if conns[0].HasPassword {
		t.Error("HasPassword = true after reset")
	}
}

// The frontend must never receive a stored password, whatever it asks for.
func TestListConnectionsRedactsPasswords(t *testing.T) {
	a := newTestApp(t)

	if err := a.CreateVault("correct horse"); err != nil {
		t.Fatalf("CreateVault: %v", err)
	}
	password := "hunter2"
	if _, err := a.CreateConnection(store.Connection{
		Label: "staging db", Host: "bastion.example.com", SSHPort: 22,
		Username: "deploy", RemoteBindPort: 5432, LocalPort: 15432,
		Password: &password,
	}); err != nil {
		t.Fatalf("CreateConnection: %v", err)
	}

	conns, err := a.ListConnections()
	if err != nil {
		t.Fatalf("ListConnections: %v", err)
	}
	if conns[0].Password != nil {
		t.Errorf("Password = %q, want nil across the bridge", *conns[0].Password)
	}
	if !conns[0].HasPassword {
		t.Error("HasPassword = false, so the UI cannot tell a password is set")
	}
}
