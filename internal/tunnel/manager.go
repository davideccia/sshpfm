package tunnel

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"

	"sshpfm/internal/sshauth"
	"sshpfm/store"
)

// ErrNotRunning is returned when an operation targets a forward that is
// not currently up.
var ErrNotRunning = errors.New("tunnel is not running")

// ErrAlreadyRunning is returned when a forward is started twice.
var ErrAlreadyRunning = errors.New("tunnel is already running")

// Manager tracks the running forwards, keyed by connection id.
//
// The zero value is not usable; call NewManager.
type Manager struct {
	knownHosts string
	emit       func(Status)
	onLog      func(int64, LogLine)
	connect    connector

	mu      sync.Mutex
	tunnels map[int64]*tunnel
}

// NewManager returns a Manager verifying hosts against the known_hosts
// file at knownHosts. emit is called on every status change, onLog on
// every log line; both may be nil.
func NewManager(knownHosts string, emit func(Status), onLog func(int64, LogLine)) *Manager {
	if emit == nil {
		emit = func(Status) {}
	}
	if onLog == nil {
		onLog = func(int64, LogLine) {}
	}
	return &Manager{
		knownHosts: knownHosts,
		emit:       emit,
		onLog:      onLog,
		connect:    defaultConnector,
		tunnels:    map[int64]*tunnel{},
	}
}

// Start brings up the forward described by c. It blocks until the local
// port is bound and the SSH connection is up, so the caller sees bind and
// auth failures directly rather than only through a status event.
func (m *Manager) Start(ctx context.Context, c store.Connection) error {
	if c.ID == 0 {
		return errors.New("connection id is required")
	}

	m.mu.Lock()
	existing, ok := m.tunnels[c.ID]
	if ok && existing.status().State != StateError {
		m.mu.Unlock()
		return fmt.Errorf("%s: %w", c.Label, ErrAlreadyRunning)
	}
	if ok {
		delete(m.tunnels, c.ID)
	}
	m.mu.Unlock()

	// Drain the previous attempt before rebinding: a tunnel that failed
	// after going active still has goroutines winding down, and its
	// listener must be gone before we ask for the same local port.
	if ok {
		existing.stop()
	}

	m.mu.Lock()
	t := newTunnel(c, m.knownHosts, m.connect, m.emit, m.onLog)
	m.tunnels[c.ID] = t
	m.mu.Unlock()

	if err := t.start(ctx); err != nil {
		// Keep the failed tunnel registered: it holds the pending host
		// key that TrustHostKey needs, and its error status stays
		// visible in the UI.
		return err
	}
	return nil
}

// Stop tears down the forward for id. Stopping a forward that is not
// running is not an error, so the UI can always offer Stop.
func (m *Manager) Stop(id int64) error {
	m.mu.Lock()
	t, ok := m.tunnels[id]
	delete(m.tunnels, id)
	m.mu.Unlock()

	if !ok {
		return nil
	}
	// Called without m.mu held: stop waits for the piping goroutines,
	// which emit status updates as they finish.
	t.stop()
	return nil
}

// StopAll tears down every forward. It is called on app shutdown.
func (m *Manager) StopAll() {
	m.mu.Lock()
	all := make([]*tunnel, 0, len(m.tunnels))
	for _, t := range m.tunnels {
		all = append(all, t)
	}
	m.tunnels = map[int64]*tunnel{}
	m.mu.Unlock()

	var wg sync.WaitGroup
	wg.Add(len(all))
	for _, t := range all {
		go func() {
			defer wg.Done()
			t.stop()
		}()
	}
	wg.Wait()
}

// IsRunning reports whether the forward for id is connecting or active.
func (m *Manager) IsRunning(id int64) bool {
	m.mu.Lock()
	t, ok := m.tunnels[id]
	m.mu.Unlock()

	if !ok {
		return false
	}
	switch t.status().State {
	case StateConnecting, StateActive:
		return true
	default:
		return false
	}
}

// Logs returns the in-memory log history for one forward, or nil when the
// forward is unknown.
func (m *Manager) Logs(id int64) []LogLine {
	m.mu.Lock()
	t, ok := m.tunnels[id]
	m.mu.Unlock()

	if !ok {
		return nil
	}
	return t.logSnapshot()
}

// Statuses returns a snapshot of every tracked forward, ordered by id.
func (m *Manager) Statuses() []Status {
	m.mu.Lock()
	all := make([]*tunnel, 0, len(m.tunnels))
	for _, t := range m.tunnels {
		all = append(all, t)
	}
	m.mu.Unlock()

	out := make([]Status, 0, len(all))
	for _, t := range all {
		out = append(out, t.status())
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// TrustHostKey records the host key the forward for id stopped on, then
// starts it again. It fails when that forward is not waiting on a host
// key decision, so a stale prompt cannot trust the wrong key.
func (m *Manager) TrustHostKey(ctx context.Context, id int64) error {
	m.mu.Lock()
	t, ok := m.tunnels[id]
	m.mu.Unlock()

	if !ok {
		return fmt.Errorf("trust host key: %w", ErrNotRunning)
	}
	pending := t.pendingHostKey()
	if pending == nil {
		return errors.New("trust host key: no host key is awaiting confirmation")
	}
	if err := sshauth.Trust(m.knownHosts, pending); err != nil {
		return err
	}

	conn := t.conn
	if err := m.Stop(id); err != nil {
		return err
	}
	return m.Start(ctx, conn)
}
