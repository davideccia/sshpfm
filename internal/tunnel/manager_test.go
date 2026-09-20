package tunnel

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"net"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"

	"sshpfm/internal/sshauth"
	"sshpfm/store"
)

// --- test doubles -----------------------------------------------------

// echoServer is the service the forward is expected to reach: it returns
// every byte it is sent, upper-cased, so the test can tell a real round
// trip from an empty read.
func echoServer(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen echo: %v", err)
	}
	t.Cleanup(func() { ln.Close() })

	// Handler goroutines are deliberately not waited on: they block in
	// Read until their peer closes, so joining them would deadlock
	// cleanup. Closing the listener stops new work, and the test binary
	// exiting reaps the rest.
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer c.Close()
				buf := make([]byte, 1024)
				for {
					n, err := c.Read(buf)
					if n > 0 {
						c.Write([]byte(strings.ToUpper(string(buf[:n]))))
					}
					if err != nil {
						return
					}
				}
			}()
		}
	}()

	return ln.Addr().(*net.TCPAddr).Port
}

// sshServer is a minimal in-process SSH server that honours
// direct-tcpip channels, which is the only thing a local forward needs.
type sshServer struct {
	addr    string
	hostKey ssh.Signer
}

func startSSHServer(t *testing.T) *sshServer {
	t.Helper()

	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate host key: %v", err)
	}
	signer, err := ssh.NewSignerFromKey(priv)
	if err != nil {
		t.Fatalf("signer: %v", err)
	}

	cfg := &ssh.ServerConfig{NoClientAuth: true}
	cfg.AddHostKey(signer)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen ssh: %v", err)
	}

	s := &sshServer{addr: ln.Addr().String(), hostKey: signer}
	t.Cleanup(func() { ln.Close() })

	// As with echoServer, connection goroutines are not joined: they
	// unblock only when their peer goes away.
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

func (s *sshServer) serve(raw net.Conn, cfg *ssh.ServerConfig) {
	conn, chans, reqs, err := ssh.NewServerConn(raw, cfg)
	if err != nil {
		raw.Close()
		return
	}
	defer conn.Close()

	// Answer keepalive@openssh.com and friends.
	go ssh.DiscardRequests(reqs)

	for newChan := range chans {
		if newChan.ChannelType() != "direct-tcpip" {
			newChan.Reject(ssh.UnknownChannelType, "only direct-tcpip is supported")
			continue
		}
		var payload struct {
			DestHost   string
			DestPort   uint32
			OriginHost string
			OriginPort uint32
		}
		if err := ssh.Unmarshal(newChan.ExtraData(), &payload); err != nil {
			newChan.Reject(ssh.ConnectionFailed, "bad payload")
			continue
		}

		target, err := net.Dial("tcp", net.JoinHostPort(payload.DestHost, strconv.Itoa(int(payload.DestPort))))
		if err != nil {
			newChan.Reject(ssh.ConnectionFailed, err.Error())
			continue
		}

		ch, chReqs, err := newChan.Accept()
		if err != nil {
			target.Close()
			continue
		}
		go ssh.DiscardRequests(chReqs)

		go func() {
			defer ch.Close()
			defer target.Close()
			var wg sync.WaitGroup
			wg.Add(2)
			go func() { defer wg.Done(); io.Copy(target, ch); target.Close() }()
			go func() { defer wg.Done(); io.Copy(ch, target); ch.Close() }()
			wg.Wait()
		}()
	}
}

// newTestManager wires a Manager to the in-process SSH server. It
// replaces the whole connector, so no test ever reaches the real SSH
// agent or the user's known_hosts; those are covered by sshauth's tests.
func newTestManager(t *testing.T, srv *sshServer, emit func(Status)) *Manager {
	t.Helper()
	m := NewManager(filepath.Join(t.TempDir(), "known_hosts"), emit, nil)
	m.connect = func(_ context.Context, _ store.Connection, _ string) (*ssh.Client, io.Closer, error) {
		client, err := ssh.Dial("tcp", srv.addr, &ssh.ClientConfig{
			User:            "test",
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

func freePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve port: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	return port
}

func profile(t *testing.T, id int64, localPort, remotePort int) store.Connection {
	t.Helper()
	return store.Connection{
		ID:             id,
		Label:          fmt.Sprintf("tunnel-%d", id),
		Host:           "127.0.0.1",
		SSHPort:        22,
		Username:       "test",
		RemoteBindPort: remotePort,
		LocalPort:      localPort,
	}
}

// roundTrip sends payload through the local forward and returns the reply.
func roundTrip(t *testing.T, localPort int, payload string) string {
	t.Helper()
	c, err := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(localPort)), 5*time.Second)
	if err != nil {
		t.Fatalf("dial local forward: %v", err)
	}
	defer c.Close()
	c.SetDeadline(time.Now().Add(5 * time.Second))

	if _, err := io.WriteString(c, payload); err != nil {
		t.Fatalf("write to forward: %v", err)
	}
	buf := make([]byte, len(payload))
	if _, err := io.ReadFull(c, buf); err != nil {
		t.Fatalf("read from forward: %v", err)
	}
	return string(buf)
}

// --- tests ------------------------------------------------------------

func TestForwardsTrafficEndToEnd(t *testing.T) {
	ctx := context.Background()
	remotePort := echoServer(t)

	srv := startSSHServer(t)
	m := newTestManager(t, srv, nil)

	local := freePort(t)
	if err := m.Start(ctx, profile(t, 1, local, remotePort)); err != nil {
		t.Fatalf("Start: %v", err)
	}

	if got := roundTrip(t, local, "hello"); got != "HELLO" {
		t.Errorf("round trip = %q, want %q", got, "HELLO")
	}
	// A second connection over the same tunnel must work too.
	if got := roundTrip(t, local, "again"); got != "AGAIN" {
		t.Errorf("second round trip = %q, want %q", got, "AGAIN")
	}

	statuses := m.Statuses()
	if len(statuses) != 1 {
		t.Fatalf("Statuses = %d entries, want 1", len(statuses))
	}
	if statuses[0].State != StateActive {
		t.Errorf("State = %q, want %q (error: %s)", statuses[0].State, StateActive, statuses[0].Error)
	}
	if statuses[0].Since == "" {
		t.Error("Since is empty for an active tunnel")
	}
}

func TestStopReleasesLocalPort(t *testing.T) {
	ctx := context.Background()
	remotePort := echoServer(t)

	srv := startSSHServer(t)
	m := newTestManager(t, srv, nil)

	local := freePort(t)
	if err := m.Start(ctx, profile(t, 1, local, remotePort)); err != nil {
		t.Fatalf("Start: %v", err)
	}
	roundTrip(t, local, "warm")

	if err := m.Stop(1); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	// The port must be immediately rebindable; a leaked listener would
	// make this fail with EADDRINUSE.
	ln, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(local)))
	if err != nil {
		t.Fatalf("local port still bound after Stop: %v", err)
	}
	ln.Close()

	if got := m.Statuses(); len(got) != 0 {
		t.Errorf("Statuses after Stop = %v, want empty", got)
	}
	if m.IsRunning(1) {
		t.Error("IsRunning = true after Stop")
	}
}

func TestStopClosesLiveStreams(t *testing.T) {
	ctx := context.Background()
	remotePort := echoServer(t)

	srv := startSSHServer(t)
	m := newTestManager(t, srv, nil)

	local := freePort(t)
	if err := m.Start(ctx, profile(t, 1, local, remotePort)); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Hold a stream open across the Stop. Stop must not block on it.
	c, err := net.Dial("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(local)))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer c.Close()
	io.WriteString(c, "x")
	io.ReadFull(c, make([]byte, 1))

	done := make(chan struct{})
	go func() {
		defer close(done)
		m.Stop(1)
	}()

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("Stop blocked on a live stream")
	}

	// The held connection must now be dead.
	c.SetReadDeadline(time.Now().Add(5 * time.Second))
	if _, err := c.Read(make([]byte, 1)); err == nil {
		t.Error("stream still readable after Stop")
	}
}

func TestStartRejectsBusyLocalPort(t *testing.T) {
	ctx := context.Background()
	srv := startSSHServer(t)
	m := newTestManager(t, srv, nil)

	blocker, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer blocker.Close()
	busy := blocker.Addr().(*net.TCPAddr).Port

	err = m.Start(ctx, profile(t, 1, busy, 9999))
	if err == nil {
		t.Fatal("Start on a busy port = nil error, want failure")
	}
	if !strings.Contains(err.Error(), strconv.Itoa(busy)) {
		t.Errorf("error %q should name the port that failed to bind", err)
	}

	statuses := m.Statuses()
	if len(statuses) != 1 || statuses[0].State != StateError {
		t.Fatalf("Statuses = %+v, want one entry in state error", statuses)
	}
	if statuses[0].Error == "" {
		t.Error("error state carries no message")
	}
}

func TestRestartAfterErrorSucceeds(t *testing.T) {
	ctx := context.Background()
	remotePort := echoServer(t)

	srv := startSSHServer(t)
	m := newTestManager(t, srv, nil)

	blocker, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	local := blocker.Addr().(*net.TCPAddr).Port

	c := profile(t, 1, local, remotePort)
	if err := m.Start(ctx, c); err == nil {
		t.Fatal("Start on a busy port = nil error, want failure")
	}

	// Free the port and retry: the failed attempt must not block the
	// retry, and the retry must reuse the same id cleanly.
	blocker.Close()
	if err := m.Start(ctx, c); err != nil {
		t.Fatalf("Start after the failure: %v", err)
	}
	if got := roundTrip(t, local, "retry"); got != "RETRY" {
		t.Errorf("round trip after restart = %q, want %q", got, "RETRY")
	}

	statuses := m.Statuses()
	if len(statuses) != 1 {
		t.Fatalf("Statuses = %d entries, want 1", len(statuses))
	}
	if statuses[0].State != StateActive {
		t.Errorf("State = %q, want %q", statuses[0].State, StateActive)
	}
	if statuses[0].Error != "" {
		t.Errorf("the stale bind error is still shown: %q", statuses[0].Error)
	}
}

func TestStartTwiceIsRejected(t *testing.T) {
	ctx := context.Background()
	remotePort := echoServer(t)

	srv := startSSHServer(t)
	m := newTestManager(t, srv, nil)

	c := profile(t, 1, freePort(t), remotePort)
	if err := m.Start(ctx, c); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := m.Start(ctx, c); !errors.Is(err, ErrAlreadyRunning) {
		t.Errorf("second Start = %v, want ErrAlreadyRunning", err)
	}
}

func TestStopUnknownTunnelIsNoOp(t *testing.T) {
	srv := startSSHServer(t)
	m := newTestManager(t, srv, nil)

	if err := m.Stop(42); err != nil {
		t.Errorf("Stop on an unknown id = %v, want nil", err)
	}
}

func TestStatusEventsTrackStreamCount(t *testing.T) {
	ctx := context.Background()
	remotePort := echoServer(t)

	var mu sync.Mutex
	var states []State
	maxStreams := 0
	emit := func(s Status) {
		mu.Lock()
		defer mu.Unlock()
		if len(states) == 0 || states[len(states)-1] != s.State {
			states = append(states, s.State)
		}
		if s.Streams > maxStreams {
			maxStreams = s.Streams
		}
	}

	srv := startSSHServer(t)
	m := newTestManager(t, srv, emit)

	local := freePort(t)
	if err := m.Start(ctx, profile(t, 1, local, remotePort)); err != nil {
		t.Fatalf("Start: %v", err)
	}
	roundTrip(t, local, "ping")
	if err := m.Stop(1); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if maxStreams < 1 {
		t.Error("no status event ever reported a live stream")
	}
	want := []State{StateConnecting, StateActive, StateStopped}
	if len(states) < len(want) {
		t.Fatalf("state transitions = %v, want to contain %v", states, want)
	}
	if states[0] != StateConnecting {
		t.Errorf("first state = %q, want %q", states[0], StateConnecting)
	}
	if states[len(states)-1] != StateStopped {
		t.Errorf("last state = %q, want %q", states[len(states)-1], StateStopped)
	}
}

func TestStopAllTearsDownEveryTunnel(t *testing.T) {
	ctx := context.Background()
	remotePort := echoServer(t)

	srv := startSSHServer(t)
	m := newTestManager(t, srv, nil)

	ports := []int{freePort(t), freePort(t), freePort(t)}
	for i, p := range ports {
		if err := m.Start(ctx, profile(t, int64(i+1), p, remotePort)); err != nil {
			t.Fatalf("Start %d: %v", i, err)
		}
	}

	m.StopAll()

	if got := m.Statuses(); len(got) != 0 {
		t.Errorf("Statuses after StopAll = %v, want empty", got)
	}
	for _, p := range ports {
		ln, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(p)))
		if err != nil {
			t.Errorf("port %d still bound after StopAll: %v", p, err)
			continue
		}
		ln.Close()
	}
}

// newTOFUManager keeps the real known_hosts policy but skips the agent,
// so the trust-on-first-use flow is exercised exactly as in production.
func newTOFUManager(t *testing.T, srv *sshServer, knownHosts string) *Manager {
	t.Helper()
	m := NewManager(knownHosts, nil, nil)
	m.connect = func(_ context.Context, c store.Connection, kh string) (*ssh.Client, io.Closer, error) {
		hostKey, err := sshauth.HostKeyCallback(kh)
		if err != nil {
			return nil, nil, err
		}
		client, err := ssh.Dial("tcp", srv.addr, &ssh.ClientConfig{
			User:            c.Username,
			HostKeyCallback: hostKey,
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

// TestUnknownHostKeyFlow is the guard on the whole trust-on-first-use
// path: the typed error has to survive the SSH handshake's error
// wrapping, reach the status as a fingerprint the UI can show, and be
// resolvable by TrustHostKey.
func TestUnknownHostKeyFlow(t *testing.T) {
	ctx := context.Background()
	remotePort := echoServer(t)
	knownHosts := filepath.Join(t.TempDir(), "known_hosts")

	srv := startSSHServer(t)
	m := newTOFUManager(t, srv, knownHosts)

	local := freePort(t)
	err := m.Start(ctx, profile(t, 1, local, remotePort))
	if err == nil {
		t.Fatal("Start against an untrusted host = nil error, want failure")
	}

	var unknown *sshauth.UnknownHostKeyError
	if !errors.As(err, &unknown) {
		t.Fatalf("Start error = %v (%T), want it to unwrap to *sshauth.UnknownHostKeyError", err, err)
	}

	statuses := m.Statuses()
	if len(statuses) != 1 {
		t.Fatalf("Statuses = %d entries, want 1", len(statuses))
	}
	pending := statuses[0].PendingHostKey
	if pending == nil {
		t.Fatal("status carries no PendingHostKey, so the UI cannot prompt")
	}
	want := ssh.FingerprintSHA256(srv.hostKey.PublicKey())
	if pending.Fingerprint != want {
		t.Errorf("Fingerprint = %q, want %q", pending.Fingerprint, want)
	}

	if err := m.TrustHostKey(ctx, 1); err != nil {
		t.Fatalf("TrustHostKey: %v", err)
	}

	if got := roundTrip(t, local, "trusted"); got != "TRUSTED" {
		t.Errorf("round trip after trusting = %q, want %q", got, "TRUSTED")
	}
	if statuses = m.Statuses(); len(statuses) != 1 || statuses[0].State != StateActive {
		t.Fatalf("Statuses after trusting = %+v, want one active entry", statuses)
	}
	if statuses[0].PendingHostKey != nil {
		t.Error("PendingHostKey is still set after trusting, so the dialog would reappear")
	}
}

func TestTrustHostKeyWithoutPendingKey(t *testing.T) {
	ctx := context.Background()
	remotePort := echoServer(t)

	srv := startSSHServer(t)
	m := newTestManager(t, srv, nil)

	if err := m.TrustHostKey(ctx, 1); !errors.Is(err, ErrNotRunning) {
		t.Errorf("TrustHostKey on an unknown id = %v, want ErrNotRunning", err)
	}

	if err := m.Start(ctx, profile(t, 1, freePort(t), remotePort)); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := m.TrustHostKey(ctx, 1); err == nil {
		t.Error("TrustHostKey on a healthy tunnel = nil error, want failure")
	}
}

func TestStartRequiresID(t *testing.T) {
	srv := startSSHServer(t)
	m := newTestManager(t, srv, nil)

	if err := m.Start(context.Background(), store.Connection{Label: "no id"}); err == nil {
		t.Error("Start without an id = nil error, want failure")
	}
}
