package sshauth

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

// UnknownHostKeyError reports a host absent from known_hosts. It is
// recoverable: show the fingerprint to the user and, if they accept it,
// pass the error to Trust.
type UnknownHostKeyError struct {
	Host        string // "host:port" as dialled
	KeyType     string // e.g. "ssh-ed25519"
	Fingerprint string // "SHA256:…"

	line string // the known_hosts line Trust appends
}

func (e *UnknownHostKeyError) Error() string {
	return fmt.Sprintf("host %s is not in known_hosts (%s %s)", e.Host, e.KeyType, e.Fingerprint)
}

// ChangedHostKeyError reports that the server presented a key different
// from the one recorded in known_hosts. It is never recoverable in-app:
// either the server was rebuilt or the connection is being intercepted,
// and only the user editing known_hosts can resolve it.
type ChangedHostKeyError struct {
	Host        string
	KeyType     string
	Fingerprint string
	KnownHosts  string // path of the file holding the conflicting entry
}

func (e *ChangedHostKeyError) Error() string {
	return fmt.Sprintf(
		"host key for %s changed: server offered %s %s, which does not match %s. "+
			"Remove the stale entry if you rebuilt the host; otherwise the connection may be intercepted",
		e.Host, e.KeyType, e.Fingerprint, e.KnownHosts)
}

// DefaultKnownHosts returns the path of the user's known_hosts file.
func DefaultKnownHosts() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir: %w", err)
	}
	return filepath.Join(home, ".ssh", "known_hosts"), nil
}

// HostKeyCallback builds a host key policy backed by the known_hosts file
// at path, creating the file when it does not exist yet.
//
// Unknown hosts yield *UnknownHostKeyError and mismatches yield
// *ChangedHostKeyError, so callers can tell "needs a decision" apart from
// "refuse".
func HostKeyCallback(path string) (ssh.HostKeyCallback, error) {
	if err := ensureKnownHosts(path); err != nil {
		return nil, err
	}

	inner, err := knownhosts.New(path)
	if err != nil {
		return nil, fmt.Errorf("read known_hosts %s: %w", path, err)
	}

	return func(hostname string, remote net.Addr, key ssh.PublicKey) error {
		err := inner(hostname, remote, key)
		if err == nil {
			return nil
		}

		var keyErr *knownhosts.KeyError
		if !errors.As(err, &keyErr) {
			return err
		}
		if len(keyErr.Want) > 0 {
			return &ChangedHostKeyError{
				Host:        hostname,
				KeyType:     key.Type(),
				Fingerprint: ssh.FingerprintSHA256(key),
				KnownHosts:  path,
			}
		}
		return &UnknownHostKeyError{
			Host:        hostname,
			KeyType:     key.Type(),
			Fingerprint: ssh.FingerprintSHA256(key),
			line:        knownhosts.Line([]string{knownhosts.Normalize(hostname)}, key),
		}
	}, nil
}

// Trust records the key carried by e in the known_hosts file at path, so
// the next connection to that host verifies. Call it only after the user
// has confirmed the fingerprint.
func Trust(path string, e *UnknownHostKeyError) error {
	if e == nil || e.line == "" {
		return errors.New("no pending host key to trust")
	}
	if err := ensureKnownHosts(path); err != nil {
		return err
	}

	// O_RDWR, not O_WRONLY: ensureTrailingNewline reads the last byte back.
	f, err := os.OpenFile(path, os.O_RDWR|os.O_APPEND, 0o600)
	if err != nil {
		return fmt.Errorf("open known_hosts %s: %w", path, err)
	}
	defer f.Close()

	if err := ensureTrailingNewline(f); err != nil {
		return err
	}
	if _, err := f.WriteString(strings.TrimRight(e.line, "\n") + "\n"); err != nil {
		return fmt.Errorf("append to known_hosts %s: %w", path, err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("close known_hosts %s: %w", path, err)
	}
	return nil
}

// ensureTrailingNewline appends a newline when the file does not already
// end with one, so a hand-edited known_hosts cannot swallow the new entry.
func ensureTrailingNewline(f *os.File) error {
	info, err := f.Stat()
	if err != nil {
		return fmt.Errorf("stat known_hosts: %w", err)
	}
	if info.Size() == 0 {
		return nil
	}

	var last [1]byte
	if _, err := f.ReadAt(last[:], info.Size()-1); err != nil {
		return fmt.Errorf("read known_hosts tail: %w", err)
	}
	if last[0] == '\n' {
		return nil
	}
	if _, err := f.WriteString("\n"); err != nil {
		return fmt.Errorf("append to known_hosts: %w", err)
	}
	return nil
}

// ensureKnownHosts creates the file and its parent directory when absent.
// knownhosts.New fails outright on a missing file, which would block the
// very first connection on a fresh machine.
func ensureKnownHosts(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create ssh dir: %w", err)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("create known_hosts %s: %w", path, err)
	}
	return f.Close()
}
