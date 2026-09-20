// Package vault derives an encryption key from a master password and
// seals secrets with it. It is deliberately self-contained: it knows
// nothing about connections, SQL, or the app, so the crypto can be read
// and tested in one sitting.
package vault

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/subtle"
	"errors"
	"fmt"

	"golang.org/x/crypto/argon2"
)

// MinPasswordLen is the shortest master password accepted. The rule lives
// here rather than in the UI so no caller can bypass it.
const MinPasswordLen = 8

// SaltLen is the length of the per-database salt.
const SaltLen = 16

// keyLen and verifierLen split the Argon2id output. Derive asks for their
// sum and hands out one half each.
const (
	keyLen      = 32
	verifierLen = 32
)

// ErrPasswordTooShort is returned by Derive for a master password below
// MinPasswordLen.
var ErrPasswordTooShort = fmt.Errorf("master password must be at least %d characters", MinPasswordLen)

// ErrCorruptSecret is returned by Open when the stored blob is too short
// to be a nonce plus a GCM tag.
var ErrCorruptSecret = errors.New("stored secret is corrupt")

// Params are the Argon2id cost parameters. They are persisted next to the
// verifier so raising them later does not lock existing databases out:
// an unlock always replays the parameters the vault was created with.
type Params struct {
	Time    uint32 // passes over memory
	Memory  uint32 // KiB
	Threads uint8
}

// DefaultParams costs roughly 100ms per derivation on a modern laptop.
// That delay is the rate limit against offline guessing; it is why no
// attempt counter is needed.
var DefaultParams = Params{Time: 1, Memory: 64 * 1024, Threads: 4}

// Key is the encryption half of a derived master key. It exists only in
// memory and is never written anywhere.
type Key struct {
	enc [keyLen]byte
}

// NewSalt returns a fresh random salt for a new vault.
func NewSalt() ([]byte, error) {
	salt := make([]byte, SaltLen)
	if _, err := rand.Read(salt); err != nil {
		return nil, fmt.Errorf("generate salt: %w", err)
	}
	return salt, nil
}

// Derive stretches the master password and splits the result in two: the
// first half encrypts secrets and never leaves memory, the second is the
// verifier stored in the database so a later unlock can check the
// password. Argon2id output bytes are independent, so holding the
// verifier does not reveal the encryption half.
func Derive(password string, salt []byte, p Params) (Key, []byte, error) {
	if len(password) < MinPasswordLen {
		return Key{}, nil, ErrPasswordTooShort
	}
	if len(salt) != SaltLen {
		return Key{}, nil, fmt.Errorf("salt must be %d bytes, got %d", SaltLen, len(salt))
	}

	out := argon2.IDKey([]byte(password), salt, p.Time, p.Memory, p.Threads, keyLen+verifierLen)

	var k Key
	copy(k.enc[:], out[:keyLen])
	return k, out[keyLen:], nil
}

// Matches reports whether a freshly derived verifier equals the stored
// one, in constant time.
func Matches(derived, stored []byte) bool {
	return subtle.ConstantTimeCompare(derived, stored) == 1
}

// Seal encrypts plaintext under AES-256-GCM and returns nonce||ciphertext.
// A fresh nonce per call is what keeps two identical passwords from
// producing identical rows.
func (k Key) Seal(plaintext string) ([]byte, error) {
	gcm, err := k.gcm()
	if err != nil {
		return nil, err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("generate nonce: %w", err)
	}

	// Appending to nonce puts the ciphertext straight after it in one
	// allocation, which is exactly the layout Open expects.
	return gcm.Seal(nonce, nonce, []byte(plaintext), nil), nil
}

// Open reverses Seal. A key derived from the wrong master password fails
// the GCM tag check rather than returning garbage.
func (k Key) Open(blob []byte) (string, error) {
	gcm, err := k.gcm()
	if err != nil {
		return "", err
	}

	if len(blob) < gcm.NonceSize()+gcm.Overhead() {
		return "", ErrCorruptSecret
	}

	nonce, ciphertext := blob[:gcm.NonceSize()], blob[gcm.NonceSize():]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", fmt.Errorf("decrypt secret: %w", err)
	}
	return string(plaintext), nil
}

func (k Key) gcm() (cipher.AEAD, error) {
	block, err := aes.NewCipher(k.enc[:])
	if err != nil {
		return nil, fmt.Errorf("aes cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("gcm: %w", err)
	}
	return gcm, nil
}
