package main

import (
	"errors"

	"sshpfm/internal/vault"
	"sshpfm/store"
)

// Vault states reported to the frontend, which shows the matching screen.
const (
	vaultUnset    = "unset"    // no master password has ever been set
	vaultLocked   = "locked"   // a vault exists and is waiting to be opened
	vaultUnlocked = "unlocked" // the key is loaded; the app is usable
)

// errLocked is what every data binding returns while the vault is shut.
// The UI gates those screens behind the unlock form, so seeing this is a
// bug rather than something the user did.
var errLocked = errors.New("unlock sshpfm with your master password first")

// unlocked guards every binding that touches connection data.
func (a *App) unlocked() error {
	if err := a.ready(); err != nil {
		return err
	}
	if !a.store.IsUnlocked() {
		return errLocked
	}
	return nil
}

// VaultState reports which screen the frontend should show.
func (a *App) VaultState() (string, error) {
	if err := a.ready(); err != nil {
		return "", err
	}
	if a.store.IsUnlocked() {
		return vaultUnlocked, nil
	}
	if _, err := a.store.Vault(a.ctx); errors.Is(err, store.ErrNoVault) {
		return vaultUnset, nil
	} else if err != nil {
		return "", err
	}
	return vaultLocked, nil
}

// CreateVault sets the master password for the first time and opens the
// vault. It refuses to overwrite an existing one: replacing the key would
// orphan every secret encrypted under the old one.
func (a *App) CreateVault(master string) error {
	if err := a.ready(); err != nil {
		return err
	}
	if _, err := a.store.Vault(a.ctx); err == nil {
		return errors.New("a master password is already set")
	} else if !errors.Is(err, store.ErrNoVault) {
		return err
	}

	salt, err := vault.NewSalt()
	if err != nil {
		return err
	}
	key, verifier, err := vault.Derive(master, salt, vault.DefaultParams)
	if err != nil {
		return err
	}

	record := store.VaultRecord{Salt: salt, Verifier: verifier, Params: vault.DefaultParams}
	if err := a.store.SetVault(a.ctx, record); err != nil {
		return err
	}

	a.store.Unlock(key)
	return nil
}

// UnlockVault checks the master password and loads the key.
//
// The Argon2id derivation costs about 100ms, which is the rate limit on
// guessing; no attempt counter is needed on top of it.
func (a *App) UnlockVault(master string) error {
	if err := a.ready(); err != nil {
		return err
	}

	record, err := a.store.Vault(a.ctx)
	if err != nil {
		return err
	}

	// The stored parameters are replayed rather than DefaultParams, so a
	// vault created under an older cost still opens.
	key, verifier, err := vault.Derive(master, record.Salt, record.Params)
	if err != nil {
		return err
	}
	if !vault.Matches(verifier, record.Verifier) {
		return errors.New("wrong master password")
	}

	a.store.Unlock(key)
	return nil
}

// ResetVault forgets the master password and every secret stored under
// it. It takes no argument on purpose: it exists precisely for the case
// where the master password is gone.
//
// The connection profiles survive. Their passwords are unrecoverable
// either way, and making the user retype hosts and ports would be a
// second loss for no gain.
func (a *App) ResetVault() error {
	if err := a.ready(); err != nil {
		return err
	}
	// Running tunnels hold a client built from the old credentials and the
	// UI is about to return to the setup screen, so they cannot be managed
	// from anywhere.
	a.tunnels.StopAll()
	return a.store.ResetVault(a.ctx)
}
