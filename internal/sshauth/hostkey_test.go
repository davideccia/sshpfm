package sshauth

import (
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"
)

func newKey(t *testing.T) ssh.PublicKey {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	_ = priv
	signer, err := ssh.NewPublicKey(pub)
	if err != nil {
		t.Fatalf("wrap key: %v", err)
	}
	return signer
}

// remoteAddr is the net.Addr the ssh package hands the callback.
func remoteAddr(t *testing.T) net.Addr {
	t.Helper()
	a, err := net.ResolveTCPAddr("tcp", "203.0.113.7:22")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	return a
}

func TestHostKeyCallbackCreatesMissingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "known_hosts")

	if _, err := HostKeyCallback(path); err != nil {
		t.Fatalf("HostKeyCallback: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("known_hosts not created: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("known_hosts mode = %o, want 600", perm)
	}
}

func TestUnknownHostThenTrust(t *testing.T) {
	path := filepath.Join(t.TempDir(), "known_hosts")
	key := newKey(t)
	const host = "bastion.example.com:2222"

	cb, err := HostKeyCallback(path)
	if err != nil {
		t.Fatalf("HostKeyCallback: %v", err)
	}

	err = cb(host, remoteAddr(t), key)
	var unknown *UnknownHostKeyError
	if !errors.As(err, &unknown) {
		t.Fatalf("first check = %v, want *UnknownHostKeyError", err)
	}
	if unknown.Host != host {
		t.Errorf("Host = %q, want %q", unknown.Host, host)
	}
	if want := ssh.FingerprintSHA256(key); unknown.Fingerprint != want {
		t.Errorf("Fingerprint = %q, want %q", unknown.Fingerprint, want)
	}
	if !strings.HasPrefix(unknown.Fingerprint, "SHA256:") {
		t.Errorf("Fingerprint = %q, want a SHA256: prefix", unknown.Fingerprint)
	}
	if unknown.KeyType != key.Type() {
		t.Errorf("KeyType = %q, want %q", unknown.KeyType, key.Type())
	}

	if err := Trust(path, unknown); err != nil {
		t.Fatalf("Trust: %v", err)
	}

	// A fresh callback reads the updated file and must now accept the key.
	cb, err = HostKeyCallback(path)
	if err != nil {
		t.Fatalf("HostKeyCallback after Trust: %v", err)
	}
	if err := cb(host, remoteAddr(t), key); err != nil {
		t.Errorf("check after Trust = %v, want nil", err)
	}
}

func TestChangedHostKeyIsRejected(t *testing.T) {
	path := filepath.Join(t.TempDir(), "known_hosts")
	original := newKey(t)
	impostor := newKey(t)
	const host = "bastion.example.com:22"

	cb, err := HostKeyCallback(path)
	if err != nil {
		t.Fatalf("HostKeyCallback: %v", err)
	}
	var unknown *UnknownHostKeyError
	if !errors.As(cb(host, remoteAddr(t), original), &unknown) {
		t.Fatal("expected the first connection to report an unknown host")
	}
	if err := Trust(path, unknown); err != nil {
		t.Fatalf("Trust: %v", err)
	}

	cb, err = HostKeyCallback(path)
	if err != nil {
		t.Fatalf("HostKeyCallback: %v", err)
	}

	err = cb(host, remoteAddr(t), impostor)
	var changed *ChangedHostKeyError
	if !errors.As(err, &changed) {
		t.Fatalf("mismatched key = %v, want *ChangedHostKeyError", err)
	}
	if changed.Fingerprint != ssh.FingerprintSHA256(impostor) {
		t.Error("ChangedHostKeyError should carry the fingerprint the server offered")
	}

	// A changed key must never be offered as a trust prompt.
	var alsoUnknown *UnknownHostKeyError
	if errors.As(err, &alsoUnknown) {
		t.Error("a changed host key must not be reported as an unknown host")
	}
}

func TestTrustAppendsAfterFileWithoutTrailingNewline(t *testing.T) {
	path := filepath.Join(t.TempDir(), "known_hosts")
	existing := newKey(t)
	// A hand-edited file whose last line has no newline: a naive append
	// would concatenate onto it and corrupt both entries.
	firstLine := "other.example.com " + existing.Type() + " " +
		strings.TrimSpace(string(ssh.MarshalAuthorizedKey(existing))[len(existing.Type())+1:])
	if err := os.WriteFile(path, []byte(firstLine), 0o600); err != nil {
		t.Fatalf("seed known_hosts: %v", err)
	}

	cb, err := HostKeyCallback(path)
	if err != nil {
		t.Fatalf("HostKeyCallback: %v", err)
	}
	key := newKey(t)
	var unknown *UnknownHostKeyError
	if !errors.As(cb("new.example.com:22", remoteAddr(t), key), &unknown) {
		t.Fatal("expected an unknown host")
	}
	if err := Trust(path, unknown); err != nil {
		t.Fatalf("Trust: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read known_hosts: %v", err)
	}
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("known_hosts has %d lines, want 2:\n%s", len(lines), data)
	}

	cb, err = HostKeyCallback(path)
	if err != nil {
		t.Fatalf("HostKeyCallback after Trust: %v", err)
	}
	if err := cb("new.example.com:22", remoteAddr(t), key); err != nil {
		t.Errorf("check after Trust = %v, want nil", err)
	}
}

func TestTrustWithoutPendingKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "known_hosts")

	if err := Trust(path, nil); err == nil {
		t.Error("Trust(nil) = nil error, want failure")
	}
	if err := Trust(path, &UnknownHostKeyError{Host: "h"}); err == nil {
		t.Error("Trust with an empty line = nil error, want failure")
	}
}
