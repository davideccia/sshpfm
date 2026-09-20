package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/wailsapp/wails/v2/pkg/options"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"sshpfm/internal/sshauth"
	"sshpfm/internal/tunnel"
	"sshpfm/store"
)

// statusEvent is the frontend event carrying tunnel state changes.
const statusEvent = "tunnel:status"

// App struct
type App struct {
	ctx     context.Context
	store   *store.Store
	tunnels *tunnel.Manager

	// initErr records a startup failure instead of killing the process,
	// so the window still opens and can explain what went wrong.
	initErr error
}

// NewApp creates a new App application struct
func NewApp() *App {
	return &App{}
}

// startup is called when the app starts. The context is saved
// so we can call the runtime methods
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx

	if err := a.init(ctx); err != nil {
		a.initErr = err
		log.Printf("sshpfm: startup failed: %v", err)
	}
}

func (a *App) init(ctx context.Context) error {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return fmt.Errorf("resolve user config dir: %w", err)
	}

	appDir := filepath.Join(configDir, "sshpfm")
	if err := os.MkdirAll(appDir, 0o700); err != nil {
		return fmt.Errorf("create app config dir: %w", err)
	}

	s, err := store.Open(filepath.Join(appDir, "sshpfm.db"))
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	a.store = s

	knownHosts, err := sshauth.DefaultKnownHosts()
	if err != nil {
		return err
	}
	a.tunnels = tunnel.NewManager(knownHosts, func(s tunnel.Status) {
		wailsruntime.EventsEmit(ctx, statusEvent, s)
	})

	if ag, err := sshauth.FindAgent(); err != nil {
		// Not fatal: the user may start the agent later. It only bites
		// when they try to connect.
		log.Printf("sshpfm: no ssh agent yet: %v", err)
	} else {
		log.Printf("sshpfm: using %s agent at %s", ag.Source, ag.Path)
	}

	return nil
}

// onSecondInstance runs when the user launches sshpfm while it is already
// running. The second process exits on its own; all this has to do is bring
// the existing window back to the front.
func (a *App) onSecondInstance(options.SecondInstanceData) {
	if a.ctx == nil {
		return
	}
	wailsruntime.WindowUnminimise(a.ctx)
	wailsruntime.Show(a.ctx)
}

// shutdown closes every tunnel and the database before the app exits.
func (a *App) shutdown(context.Context) {
	if a.tunnels != nil {
		a.tunnels.StopAll()
	}
	if a.store != nil {
		// Drop the master key before anything else, so it does not sit in
		// memory through the rest of the teardown.
		a.store.Lock()
		if err := a.store.Close(); err != nil {
			log.Printf("sshpfm: close store: %v", err)
		}
	}
}

// ready reports the startup failure, if any, so every binding fails with
// a clear message rather than a nil-pointer panic.
func (a *App) ready() error {
	if a.initErr != nil {
		return fmt.Errorf("sshpfm could not start: %w", a.initErr)
	}
	return nil
}

// StartupError returns the startup failure message, or "" when the app
// initialised cleanly.
func (a *App) StartupError() string {
	if a.initErr == nil {
		return ""
	}
	return a.initErr.Error()
}

// ListConnections returns all stored SSH port-forwarding profiles.
func (a *App) ListConnections() ([]store.Connection, error) {
	if err := a.unlocked(); err != nil {
		return nil, err
	}
	conns, err := a.store.List(a.ctx)
	if err != nil {
		return nil, err
	}
	// Stored passwords have no use in the frontend, and shipping them
	// across the bridge would park every secret in the renderer.
	for i, c := range conns {
		conns[i] = c.Redacted()
	}
	return conns, nil
}

// CreateConnection validates and persists a new connection profile.
func (a *App) CreateConnection(c store.Connection) (int64, error) {
	if err := a.unlocked(); err != nil {
		return 0, err
	}
	return a.store.Create(a.ctx, c)
}

// UpdateConnection persists changes to an existing connection profile.
// Editing a profile whose tunnel is up is refused: the running forward
// would keep using the old settings, which is worse than saying no.
func (a *App) UpdateConnection(c store.Connection) error {
	if err := a.unlocked(); err != nil {
		return err
	}
	if a.tunnels.IsRunning(c.ID) {
		return errors.New("stop the tunnel before editing this connection")
	}
	return a.store.Update(a.ctx, c)
}

// DeleteConnection removes a connection profile by id, stopping its
// tunnel first.
func (a *App) DeleteConnection(id int64) error {
	if err := a.unlocked(); err != nil {
		return err
	}
	if err := a.tunnels.Stop(id); err != nil {
		return err
	}
	return a.store.Delete(a.ctx, id)
}

// StartTunnel opens the local port forward for the given connection.
func (a *App) StartTunnel(id int64) error {
	if err := a.unlocked(); err != nil {
		return err
	}
	c, err := a.store.Get(a.ctx, id)
	if err != nil {
		return err
	}
	return a.tunnels.Start(a.ctx, c)
}

// StopTunnel closes the local port forward for the given connection.
func (a *App) StopTunnel(id int64) error {
	if err := a.unlocked(); err != nil {
		return err
	}
	return a.tunnels.Stop(id)
}

// TunnelStatuses returns the current state of every tracked tunnel.
func (a *App) TunnelStatuses() ([]tunnel.Status, error) {
	if err := a.unlocked(); err != nil {
		return nil, err
	}
	return a.tunnels.Statuses(), nil
}

// OpenInBrowser opens the local end of a running tunnel in the user's
// default browser. It refuses when the tunnel is down, because the
// browser would only show a connection-refused page.
func (a *App) OpenInBrowser(id int64) error {
	if err := a.unlocked(); err != nil {
		return err
	}
	if !a.tunnels.IsRunning(id) {
		return errors.New("start the tunnel before opening it in the browser")
	}
	c, err := a.store.Get(a.ctx, id)
	if err != nil {
		return err
	}

	// http, not https: a forward terminates at a plain local socket, and
	// the overwhelming majority of forwarded services speak http there.
	url := fmt.Sprintf("http://localhost:%d", c.LocalPort)
	wailsruntime.BrowserOpenURL(a.ctx, url)
	return nil
}

// TrustHostKey records the host key the given tunnel stopped on and
// retries the connection. Call it only after the user has confirmed the
// fingerprint shown in the status.
func (a *App) TrustHostKey(id int64) error {
	if err := a.unlocked(); err != nil {
		return err
	}
	return a.tunnels.TrustHostKey(a.ctx, id)
}
