package vault

import (
	"bytes"
	"errors"
	"testing"
)

// testParams keep the suite fast. DefaultParams costs 64 MiB per
// derivation, which a test file full of them would pay dozens of times.
var testParams = Params{Time: 1, Memory: 8 * 1024, Threads: 1}

func testSalt(t *testing.T) []byte {
	t.Helper()
	salt, err := NewSalt()
	if err != nil {
		t.Fatalf("NewSalt: %v", err)
	}
	return salt
}

func derive(t *testing.T, password string, salt []byte) (Key, []byte) {
	t.Helper()
	k, verifier, err := Derive(password, salt, testParams)
	if err != nil {
		t.Fatalf("Derive: %v", err)
	}
	return k, verifier
}

func TestNewSaltIsRandomAndSized(t *testing.T) {
	a, b := testSalt(t), testSalt(t)
	if len(a) != SaltLen {
		t.Errorf("len(salt) = %d, want %d", len(a), SaltLen)
	}
	if bytes.Equal(a, b) {
		t.Error("two salts are identical")
	}
}

func TestDeriveIsDeterministic(t *testing.T) {
	salt := testSalt(t)

	k1, v1 := derive(t, "correct horse", salt)
	k2, v2 := derive(t, "correct horse", salt)

	if k1 != k2 {
		t.Error("same password and salt produced different keys")
	}
	if !bytes.Equal(v1, v2) {
		t.Error("same password and salt produced different verifiers")
	}
}

func TestDeriveDiffersBySalt(t *testing.T) {
	_, v1 := derive(t, "correct horse", testSalt(t))
	_, v2 := derive(t, "correct horse", testSalt(t))

	if bytes.Equal(v1, v2) {
		t.Error("different salts produced the same verifier")
	}
}

func TestDeriveDiffersByPassword(t *testing.T) {
	salt := testSalt(t)

	_, v1 := derive(t, "correct horse", salt)
	_, v2 := derive(t, "correct zebra", salt)

	if bytes.Equal(v1, v2) {
		t.Error("different passwords produced the same verifier")
	}
}

// The verifier is stored in the database; if it repeated the encryption
// half, anyone holding the file would hold the key.
func TestVerifierDiffersFromKey(t *testing.T) {
	k, verifier := derive(t, "correct horse", testSalt(t))

	if bytes.Equal(k.enc[:], verifier) {
		t.Error("verifier equals the encryption key")
	}
}

func TestDeriveRejectsShortPassword(t *testing.T) {
	_, _, err := Derive("short", testSalt(t), testParams)
	if !errors.Is(err, ErrPasswordTooShort) {
		t.Errorf("Derive error = %v, want ErrPasswordTooShort", err)
	}
}

func TestDeriveRejectsWrongSaltLength(t *testing.T) {
	_, _, err := Derive("correct horse", []byte("tooshort"), testParams)
	if err == nil {
		t.Fatal("Derive accepted a short salt")
	}
}

func TestMatches(t *testing.T) {
	salt := testSalt(t)
	_, stored := derive(t, "correct horse", salt)

	_, same := derive(t, "correct horse", salt)
	if !Matches(same, stored) {
		t.Error("Matches = false for the right password")
	}

	_, other := derive(t, "correct zebra", salt)
	if Matches(other, stored) {
		t.Error("Matches = true for the wrong password")
	}
}

func TestSealOpenRoundTrip(t *testing.T) {
	k, _ := derive(t, "correct horse", testSalt(t))

	const want = "hunter2"
	blob, err := k.Seal(want)
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}
	if bytes.Contains(blob, []byte(want)) {
		t.Error("ciphertext contains the plaintext")
	}

	got, err := k.Open(blob)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if got != want {
		t.Errorf("Open = %q, want %q", got, want)
	}
}

func TestSealHandlesEmptyString(t *testing.T) {
	k, _ := derive(t, "correct horse", testSalt(t))

	blob, err := k.Seal("")
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}
	got, err := k.Open(blob)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if got != "" {
		t.Errorf("Open = %q, want empty", got)
	}
}

func TestSealUsesFreshNonce(t *testing.T) {
	k, _ := derive(t, "correct horse", testSalt(t))

	a, err := k.Seal("hunter2")
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}
	b, err := k.Seal("hunter2")
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}
	if bytes.Equal(a, b) {
		t.Error("sealing the same plaintext twice produced identical blobs")
	}
}

func TestOpenWithWrongKeyFails(t *testing.T) {
	salt := testSalt(t)
	right, _ := derive(t, "correct horse", salt)
	wrong, _ := derive(t, "correct zebra", salt)

	blob, err := right.Seal("hunter2")
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}
	if _, err := wrong.Open(blob); err == nil {
		t.Fatal("Open succeeded with the wrong key")
	}
}

func TestOpenRejectsTruncatedBlob(t *testing.T) {
	k, _ := derive(t, "correct horse", testSalt(t))

	if _, err := k.Open([]byte("short")); !errors.Is(err, ErrCorruptSecret) {
		t.Errorf("Open error = %v, want ErrCorruptSecret", err)
	}
}

func TestOpenRejectsTamperedBlob(t *testing.T) {
	k, _ := derive(t, "correct horse", testSalt(t))

	blob, err := k.Seal("hunter2")
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}
	blob[len(blob)-1] ^= 0xff

	if _, err := k.Open(blob); err == nil {
		t.Fatal("Open accepted a tampered blob")
	}
}
