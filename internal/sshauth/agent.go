// Package sshauth resolves the credentials sshpfm uses to open SSH
// connections: the signing agent to authenticate with, and the host key
// policy to verify the server against.
package sshauth

import (
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

// SocketEnvVar overrides agent discovery entirely when set.
const SocketEnvVar = "SSHPFM_AGENT_SOCK"

// Agent describes the socket sshpfm authenticates through.
type Agent struct {
	Path string // filesystem path of the agent socket
	// Source names where Path came from: "SSHPFM_AGENT_SOCK",
	// "1Password" or "SSH_AUTH_SOCK". Surfaced in logs so it is obvious
	// which agent signed a connection.
	Source string
}

// FindAgent locates the SSH agent socket to authenticate through,
// preferring the 1Password agent when it is running.
//
// SSH_AUTH_SOCK is checked last on purpose: on macOS it points at the
// launchd-provided agent even when 1Password is installed, so honouring it
// first would bypass 1Password entirely.
func FindAgent() (Agent, error) {
	if p := os.Getenv(SocketEnvVar); p != "" {
		return Agent{Path: p, Source: SocketEnvVar}, nil
	}

	tried := []string{}
	if p := onePasswordSocket(); p != "" {
		if socketExists(p) {
			return Agent{Path: p, Source: "1Password"}, nil
		}
		tried = append(tried, "1Password ("+p+")")
	}

	if p := os.Getenv("SSH_AUTH_SOCK"); p != "" {
		if socketExists(p) {
			return Agent{Path: p, Source: "SSH_AUTH_SOCK"}, nil
		}
		tried = append(tried, "SSH_AUTH_SOCK ("+p+")")
	} else {
		tried = append(tried, "SSH_AUTH_SOCK (unset)")
	}

	return Agent{}, fmt.Errorf("no ssh agent found; tried %s. Start the 1Password SSH agent or ssh-agent, or set %s",
		strings.Join(tried, ", "), SocketEnvVar)
}

// onePasswordSocket returns the well-known 1Password agent socket path for
// this OS, or "" when 1Password does not expose one we can dial.
func onePasswordSocket() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	switch runtime.GOOS {
	case "darwin":
		return filepath.Join(home, "Library", "Group Containers",
			"2BUA8C4S2C.com.1password", "t", "agent.sock")
	case "linux":
		return filepath.Join(home, ".1password", "agent.sock")
	default:
		// On Windows the agent is a named pipe, which needs a different
		// dialer than net.Dial("unix", …). Not supported yet.
		return ""
	}
}

func socketExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// Dial connects to the agent socket and returns a public-key auth method
// backed by it. The returned io.Closer owns the socket connection and must
// be closed once the SSH connection it authenticated is finished with.
func (a Agent) Dial() (ssh.AuthMethod, io.Closer, error) {
	if runtime.GOOS == "windows" {
		return nil, nil, fmt.Errorf("ssh agent support is not implemented on windows yet")
	}
	conn, err := net.Dial("unix", a.Path)
	if err != nil {
		return nil, nil, fmt.Errorf("dial %s agent at %s: %w", a.Source, a.Path, err)
	}
	client := agent.NewClient(conn)

	// Signers are fetched lazily, per authentication attempt, so keys
	// unlocked in 1Password after the app started are still picked up.
	return ssh.PublicKeysCallback(client.Signers), conn, nil
}
