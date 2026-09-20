package tunnel

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"io"
	"net"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"

	"sshpfm/store"
)

// authMode picks which credential the test server will accept. The
// existing startSSHServer sets NoClientAuth, which makes the client stop
// at "none" and never offer a password, so the password path needs a
// server of its own.
type authMode int

const (
	authPassword authMode = iota
	authKeyboardInteractive
)

// startAuthSSHServer is startSSHServer with client authentication turned
// on. It reuses the same serve loop, so it forwards direct-tcpip exactly
// like the real thing.
func startAuthSSHServer(t *testing.T, mode authMode, want string) *sshServer {
	t.Helper()

	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate host key: %v", err)
	}
	signer, err := ssh.NewSignerFromKey(priv)
	if err != nil {
		t.Fatalf("signer: %v", err)
	}

	cfg := &ssh.ServerConfig{}
	switch mode {
	case authPassword:
		cfg.PasswordCallback = func(_ ssh.ConnMetadata, got []byte) (*ssh.Permissions, error) {
			if string(got) != want {
				return nil, errors.New("wrong password")
			}
			return nil, nil
		}
	case authKeyboardInteractive:
		cfg.KeyboardInteractiveCallback = func(_ ssh.ConnMetadata, challenge ssh.KeyboardInteractiveChallenge) (*ssh.Permissions, error) {
			// One hidden question, the shape PAM uses in practice.
			answers, err := challenge("", "", []string{"Password:"}, []bool{false})
			if err != nil {
				return nil, err
			}
			if len(answers) != 1 || answers[0] != want {
				return nil, errors.New("wrong password")
			}
			return nil, nil
		}
	}
	cfg.AddHostKey(signer)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen ssh: %v", err)
	}

	s := &sshServer{addr: ln.Addr().String(), hostKey: signer}
	t.Cleanup(func() { ln.Close() })

	go func() {
		for {
			raw, err := ln.Accept()
			if err != nil {
				return
			}
			go s.serve(raw, cfg)
		}
	}()

	return s
}

// newPasswordManager exercises the real authMethods against the test
// server: only the agent lookup and the host-key policy are stubbed out,
// so the credential assembly under test is production code.
func newPasswordManager(t *testing.T, srv *sshServer) *Manager {
	t.Helper()
	m := NewManager("", nil)
	m.connect = func(_ context.Context, c store.Connection, _ string) (*ssh.Client, io.Closer, error) {
		password := ""
		if c.Password != nil {
			password = *c.Password
		}
		client, err := ssh.Dial("tcp", srv.addr, &ssh.ClientConfig{
			User:            c.Username,
			Auth:            authMethods(nil, password),
			HostKeyCallback: ssh.FixedHostKey(srv.hostKey.PublicKey()),
			Timeout:         5 * time.Second,
		})
		if err != nil {
			return nil, nil, err
		}
		return client, io.NopCloser(nil), nil
	}
	t.Cleanup(m.StopAll)
	return m
}

func passwordProfile(t *testing.T, id int64, localPort, remotePort int, password string) store.Connection {
	t.Helper()
	c := profile(t, id, localPort, remotePort)
	c.Password = &password
	c.HasPassword = true
	return c
}

func TestPasswordAuthConnects(t *testing.T) {
	ctx := context.Background()
	remotePort := echoServer(t)

	srv := startAuthSSHServer(t, authPassword, "hunter2")
	m := newPasswordManager(t, srv)

	local := freePort(t)
	if err := m.Start(ctx, passwordProfile(t, 1, local, remotePort, "hunter2")); err != nil {
		t.Fatalf("Start: %v", err)
	}

	if got := roundTrip(t, local, "hello"); got != "HELLO" {
		t.Errorf("roundTrip = %q, want %q", got, "HELLO")
	}
}

// Servers with PAM commonly advertise keyboard-interactive and not
// password; without this method the same stored secret would fail.
func TestKeyboardInteractiveAuthConnects(t *testing.T) {
	ctx := context.Background()
	remotePort := echoServer(t)

	srv := startAuthSSHServer(t, authKeyboardInteractive, "hunter2")
	m := newPasswordManager(t, srv)

	local := freePort(t)
	if err := m.Start(ctx, passwordProfile(t, 1, local, remotePort, "hunter2")); err != nil {
		t.Fatalf("Start: %v", err)
	}

	if got := roundTrip(t, local, "hello"); got != "HELLO" {
		t.Errorf("roundTrip = %q, want %q", got, "HELLO")
	}
}

func TestWrongPasswordFails(t *testing.T) {
	ctx := context.Background()
	remotePort := echoServer(t)

	srv := startAuthSSHServer(t, authPassword, "hunter2")
	m := newPasswordManager(t, srv)

	local := freePort(t)
	if err := m.Start(ctx, passwordProfile(t, 1, local, remotePort, "swordfish")); err == nil {
		t.Fatal("Start with the wrong password = nil error, want failure")
	}

	statuses := m.Statuses()
	if len(statuses) != 1 {
		t.Fatalf("Statuses = %d entries, want 1", len(statuses))
	}
	if statuses[0].State != StateError {
		t.Errorf("State = %q, want %q", statuses[0].State, StateError)
	}
}

// A profile with no password must not offer one, or a server that only
// accepts keys would see a pointless failed attempt.
func TestProfileWithoutPasswordOffersNothing(t *testing.T) {
	ctx := context.Background()
	remotePort := echoServer(t)

	srv := startAuthSSHServer(t, authPassword, "hunter2")
	m := newPasswordManager(t, srv)

	local := freePort(t)
	if err := m.Start(ctx, profile(t, 1, local, remotePort)); err == nil {
		t.Fatal("Start without credentials = nil error, want failure")
	}
}
