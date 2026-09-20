package tunnel

import (
	"fmt"
	"testing"

	"golang.org/x/crypto/ssh"
)

// ssh.AuthMethod values are funcs and so uncomparable; their concrete
// types are what distinguishes one method from another. stubAuth returns
// the same kind of method sshauth.Agent.Dial hands back.
func stubAuth() ssh.AuthMethod { return ssh.PublicKeysCallback(nil) }

func kind(m ssh.AuthMethod) string { return fmt.Sprintf("%T", m) }

func TestAuthMethodsAgentOnly(t *testing.T) {
	got := authMethods(stubAuth(), "")
	if len(got) != 1 {
		t.Fatalf("len(authMethods) = %d, want 1", len(got))
	}
	if kind(got[0]) != kind(stubAuth()) {
		t.Errorf("authMethods[0] = %T, want the agent method", got[0])
	}
}

// The agent has to come first: a key the user already has must keep
// working without the server ever being offered the password.
func TestAuthMethodsAgentThenPassword(t *testing.T) {
	got := authMethods(stubAuth(), "hunter2")

	if len(got) != 3 {
		t.Fatalf("len(authMethods) = %d, want 3 (agent, password, keyboard-interactive)", len(got))
	}
	want := []string{kind(stubAuth()), kind(ssh.Password("")), kind(ssh.KeyboardInteractive(nil))}
	for i, w := range want {
		if kind(got[i]) != w {
			t.Errorf("authMethods[%d] = %T, want %s", i, got[i], w)
		}
	}
}

func TestAuthMethodsPasswordOnly(t *testing.T) {
	got := authMethods(nil, "hunter2")
	if len(got) != 2 {
		t.Fatalf("len(authMethods) = %d, want 2 (password, keyboard-interactive)", len(got))
	}
	if kind(got[0]) != kind(ssh.Password("")) {
		t.Errorf("authMethods[0] = %T, want the password method", got[0])
	}
	if kind(got[1]) != kind(ssh.KeyboardInteractive(nil)) {
		t.Errorf("authMethods[1] = %T, want the keyboard-interactive method", got[1])
	}
}

// Neither an agent nor a password leaves nothing to try, and the caller
// relies on the empty slice to report the agent error instead of dialling.
func TestAuthMethodsWithNothingIsEmpty(t *testing.T) {
	if got := authMethods(nil, ""); len(got) != 0 {
		t.Errorf("len(authMethods) = %d, want 0", len(got))
	}
}

func TestFixedAnswerAnswersHiddenPrompts(t *testing.T) {
	answers, err := fixedAnswer("hunter2")("", "", []string{"Password:"}, []bool{false})
	if err != nil {
		t.Fatalf("challenge: %v", err)
	}
	if len(answers) != 1 {
		t.Fatalf("len(answers) = %d, want 1", len(answers))
	}
	if answers[0] != "hunter2" {
		t.Errorf("answers[0] = %q, want %q", answers[0], "hunter2")
	}
}

// An echoed prompt is displayed by the server, so it is never asking for
// a secret; answering it with the password would leak it.
func TestFixedAnswerLeavesEchoedPromptsEmpty(t *testing.T) {
	answers, err := fixedAnswer("hunter2")("", "", []string{"Username:", "Password:"}, []bool{true, false})
	if err != nil {
		t.Fatalf("challenge: %v", err)
	}
	if answers[0] != "" {
		t.Errorf("answers[0] = %q, want empty for an echoed prompt", answers[0])
	}
	if answers[1] != "hunter2" {
		t.Errorf("answers[1] = %q, want %q", answers[1], "hunter2")
	}
}

// Some servers open with an informational challenge and no questions.
func TestFixedAnswerHandlesNoQuestions(t *testing.T) {
	answers, err := fixedAnswer("hunter2")("", "Welcome", nil, nil)
	if err != nil {
		t.Fatalf("challenge: %v", err)
	}
	if len(answers) != 0 {
		t.Errorf("len(answers) = %d, want 0", len(answers))
	}
}
