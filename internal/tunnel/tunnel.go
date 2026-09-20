// Package tunnel runs SSH local port forwards: it binds a port on the
// loopback interface and pipes every connection accepted there through an
// SSH client to a port on the remote side.
package tunnel

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"

	"sshpfm/internal/sshauth"
	"sshpfm/store"
)

// State is the lifecycle stage of a single forward.
type State string

const (
	StateStopped    State = "stopped"
	StateConnecting State = "connecting"
	StateActive     State = "active"
	StateError      State = "error"
)

// keepaliveInterval bounds how long a dead link can masquerade as active.
const keepaliveInterval = 15 * time.Second

// dialTimeout caps the SSH handshake, so an unreachable host does not
// leave the UI stuck on "connecting".
const dialTimeout = 15 * time.Second

// PendingHostKey is the fingerprint the user is being asked to trust.
type PendingHostKey struct {
	Host        string `json:"host"`
	KeyType     string `json:"keyType"`
	Fingerprint string `json:"fingerprint"`
}

// Status is the snapshot of one forward sent to the frontend.
type Status struct {
	ID    int64  `json:"id"`
	State State  `json:"state"`
	Error string `json:"error"`
	// Streams counts connections currently being forwarded.
	Streams int `json:"streams"`
	// Since is when the forward last became active, RFC3339, empty
	// when it is not active.
	Since string `json:"since"`
	// PendingHostKey is set only when State is StateError because the
	// host is absent from known_hosts; it drives the trust prompt.
	PendingHostKey *PendingHostKey `json:"pendingHostKey"`
}

// connector establishes the SSH client for a forward, including resolving
// the agent and the host key policy. The whole step is behind one seam so
// tests can substitute an in-process server without touching the real
// agent or the user's known_hosts.
//
// The returned io.Closer owns the agent socket and is closed with the
// client.
type connector func(ctx context.Context, c store.Connection, knownHosts string) (*ssh.Client, io.Closer, error)

// authMethods orders the ways to prove identity: the agent first, so a
// key the user already has keeps working, then the stored password.
// Servers commonly advertise keyboard-interactive instead of password, so
// both are offered; x/crypto/ssh skips whichever the server does not
// accept.
func authMethods(agent ssh.AuthMethod, password string) []ssh.AuthMethod {
	var methods []ssh.AuthMethod
	if agent != nil {
		methods = append(methods, agent)
	}
	if password != "" {
		methods = append(methods, ssh.Password(password), ssh.KeyboardInteractive(fixedAnswer(password)))
	}
	return methods
}

// fixedAnswer replies to a keyboard-interactive challenge with the stored
// password. PAM asks one hidden question ("Password:"); an echoed prompt
// is never a secret, so it is answered empty rather than leaking the
// password into something the server will display or log.
func fixedAnswer(password string) ssh.KeyboardInteractiveChallenge {
	return func(_, _ string, questions []string, echos []bool) ([]string, error) {
		answers := make([]string, len(questions))
		for i := range questions {
			if i < len(echos) && echos[i] {
				continue
			}
			answers[i] = password
		}
		return answers, nil
	}
}

// defaultConnector authenticates through the SSH agent (1Password first),
// falling back to the profile's stored password, and verifies the host
// against known_hosts.
func defaultConnector(ctx context.Context, c store.Connection, knownHosts string) (*ssh.Client, io.Closer, error) {
	password := ""
	if c.Password != nil {
		password = *c.Password
	}

	// A missing agent is only fatal when there is nothing to fall back
	// on: a password profile has to work on a machine with no agent
	// running, and on Windows where Agent.Dial always errors.
	var agentAuth ssh.AuthMethod
	agentConn := io.Closer(io.NopCloser(nil))
	ag, agentErr := sshauth.FindAgent()
	if agentErr == nil {
		auth, conn, err := ag.Dial()
		if err != nil {
			agentErr = err
		} else {
			agentAuth, agentConn = auth, conn
		}
	}

	methods := authMethods(agentAuth, password)
	if len(methods) == 0 {
		return nil, nil, agentErr
	}

	hostKey, err := sshauth.HostKeyCallback(knownHosts)
	if err != nil {
		agentConn.Close()
		return nil, nil, err
	}

	cfg := &ssh.ClientConfig{
		User:            c.Username,
		Auth:            methods,
		HostKeyCallback: hostKey,
		Timeout:         dialTimeout,
	}
	addr := net.JoinHostPort(c.Host, strconv.Itoa(c.SSHPort))

	// net.Dialer rather than ssh.Dial: it honours ctx, so cancelling a
	// start does not leave a TCP dial running for the full timeout.
	d := net.Dialer{Timeout: dialTimeout}
	raw, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		agentConn.Close()
		return nil, nil, fmt.Errorf("dial %s: %w", addr, err)
	}

	conn, chans, reqs, err := ssh.NewClientConn(raw, addr, cfg)
	if err != nil {
		raw.Close()
		agentConn.Close()
		return nil, nil, fmt.Errorf("ssh %s@%s: %w", c.Username, addr, err)
	}
	return ssh.NewClient(conn, chans, reqs), agentConn, nil
}

// tunnel owns one local forward and everything it allocates: the local
// listener, the SSH client, the agent socket, and every goroutine piping
// bytes between them.
type tunnel struct {
	conn       store.Connection
	knownHosts string
	connect    connector
	// onChange is called after every state transition, never while
	// holding mu.
	onChange func(Status)

	cancel context.CancelFunc
	wg     sync.WaitGroup

	mu         sync.Mutex
	state      State
	err        error
	pendingKey *sshauth.UnknownHostKeyError
	streams    int
	since      time.Time
	listener   net.Listener
	client     *ssh.Client
	agentConn  io.Closer
}

func newTunnel(c store.Connection, knownHosts string, conn connector, onChange func(Status)) *tunnel {
	return &tunnel{
		conn:       c,
		knownHosts: knownHosts,
		connect:    conn,
		onChange:   onChange,
		state:      StateStopped,
	}
}

// start brings the forward up. It returns once the local port is bound
// and the SSH connection is established, so the caller learns about port
// conflicts and auth failures synchronously.
func (t *tunnel) start(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	t.cancel = cancel

	t.setState(StateConnecting, nil)

	// Bind locally first: a port conflict is the most common failure and
	// must surface before we bother the remote host or the agent.
	localAddr := net.JoinHostPort("127.0.0.1", strconv.Itoa(t.conn.LocalPort))
	ln, err := net.Listen("tcp", localAddr)
	if err != nil {
		cancel()
		return t.fail(fmt.Errorf("bind %s: %w", localAddr, err))
	}

	client, agentConn, err := t.connect(ctx, t.conn, t.knownHosts)
	if err != nil {
		ln.Close()
		cancel()
		return t.fail(err)
	}

	t.mu.Lock()
	t.listener = ln
	t.client = client
	t.agentConn = agentConn
	t.mu.Unlock()

	t.wg.Add(2)
	go func() {
		defer t.wg.Done()
		t.acceptLoop(ctx, ln, client)
	}()
	go func() {
		defer t.wg.Done()
		t.keepalive(ctx, client)
	}()

	t.setState(StateActive, nil)
	return nil
}

// acceptLoop forwards every connection arriving on the local listener.
func (t *tunnel) acceptLoop(ctx context.Context, ln net.Listener, client *ssh.Client) {
	remoteAddr := net.JoinHostPort("localhost", strconv.Itoa(t.conn.RemoteBindPort))

	for {
		local, err := ln.Accept()
		if err != nil {
			// stop() closes the listener; that is not a failure.
			if errors.Is(err, net.ErrClosed) || ctx.Err() != nil {
				return
			}
			t.setState(StateError, fmt.Errorf("accept on local port %d: %w", t.conn.LocalPort, err))
			return
		}

		remote, err := client.Dial("tcp", remoteAddr)
		if err != nil {
			local.Close()
			// One refused connection does not mean the tunnel is
			// down, so report it and keep listening.
			t.report(fmt.Errorf("dial %s over the tunnel: %w", remoteAddr, err))
			continue
		}

		t.wg.Add(1)
		go func() {
			defer t.wg.Done()
			t.pipe(local, remote)
		}()
	}
}

// pipe copies bytes both ways until either side finishes, then closes
// both. Closing both is what unblocks the opposite copy.
func (t *tunnel) pipe(local, remote net.Conn) {
	t.addStream(1)
	defer t.addStream(-1)

	var once sync.Once
	closeBoth := func() {
		once.Do(func() {
			local.Close()
			remote.Close()
		})
	}
	defer closeBoth()

	var wg sync.WaitGroup
	wg.Add(2)
	pump := func(dst, src net.Conn) {
		defer wg.Done()
		defer closeBoth()
		io.Copy(dst, src)
	}
	go pump(remote, local)
	go pump(local, remote)
	wg.Wait()
}

// keepalive detects a link that died without closing, which otherwise
// leaves the forward looking active while every new stream hangs.
func (t *tunnel) keepalive(ctx context.Context, client *ssh.Client) {
	ticker := time.NewTicker(keepaliveInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if _, _, err := client.SendRequest("keepalive@openssh.com", true, nil); err != nil {
				if ctx.Err() != nil {
					return
				}
				t.setState(StateError, fmt.Errorf("ssh connection lost: %w", err))
				t.closeResources()
				return
			}
		}
	}
}

// stop tears the forward down and waits for every goroutine it started.
func (t *tunnel) stop() {
	if t.cancel != nil {
		t.cancel()
	}
	t.closeResources()
	t.wg.Wait()
	t.setState(StateStopped, nil)
}

// closeResources releases the listener, SSH client and agent socket.
// Closing the listener unblocks Accept; closing the client kills every
// in-flight stream, which unblocks the io.Copy pairs.
func (t *tunnel) closeResources() {
	t.mu.Lock()
	ln, client, agentConn := t.listener, t.client, t.agentConn
	t.listener, t.client, t.agentConn = nil, nil, nil
	t.mu.Unlock()

	if ln != nil {
		ln.Close()
	}
	if client != nil {
		client.Close()
	}
	if agentConn != nil {
		agentConn.Close()
	}
}

// fail records err as the terminal state and returns it, so callers can
// write `return t.fail(err)`.
func (t *tunnel) fail(err error) error {
	t.setState(StateError, err)
	return err
}

func (t *tunnel) setState(s State, err error) {
	t.mu.Lock()
	t.state = s
	t.err = err
	t.pendingKey = nil
	// Only an unknown host key is recoverable by the user; a changed key
	// deliberately gets no prompt.
	var unknown *sshauth.UnknownHostKeyError
	if err != nil && errors.As(err, &unknown) {
		t.pendingKey = unknown
	}
	switch s {
	case StateActive:
		t.since = time.Now()
	case StateStopped, StateError:
		t.since = time.Time{}
		t.streams = 0
	}
	status := t.statusLocked()
	t.mu.Unlock()

	t.onChange(status)
}

// report surfaces a non-fatal error without changing the lifecycle state.
func (t *tunnel) report(err error) {
	t.mu.Lock()
	t.err = err
	status := t.statusLocked()
	t.mu.Unlock()

	t.onChange(status)
}

func (t *tunnel) addStream(delta int) {
	t.mu.Lock()
	t.streams += delta
	status := t.statusLocked()
	t.mu.Unlock()

	t.onChange(status)
}

func (t *tunnel) status() Status {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.statusLocked()
}

// pendingHostKey returns the untrusted key this forward stopped on, or
// nil. Unlike Status.PendingHostKey it carries the known_hosts line, so
// it is what sshauth.Trust needs.
func (t *tunnel) pendingHostKey() *sshauth.UnknownHostKeyError {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.pendingKey
}

// statusLocked builds a snapshot; the caller must hold mu. The snapshot is
// emitted only after the lock is released, so a slow consumer cannot stall
// the forward.
func (t *tunnel) statusLocked() Status {
	s := Status{
		ID:      t.conn.ID,
		State:   t.state,
		Streams: t.streams,
	}
	if t.err != nil {
		s.Error = t.err.Error()
	}
	if !t.since.IsZero() {
		s.Since = t.since.Format(time.RFC3339)
	}
	if t.pendingKey != nil {
		s.PendingHostKey = &PendingHostKey{
			Host:        t.pendingKey.Host,
			KeyType:     t.pendingKey.KeyType,
			Fingerprint: t.pendingKey.Fingerprint,
		}
	}
	return s
}
