package cmd

import (
	"context"
	"errors"
	"testing"
)

// fakeStoreAMDKeychain records the writes made by the Store session helpers so
// tests can assert that an already provisioned profile stays untouched.
type fakeStoreAMDKeychain struct {
	values map[string][]byte
	sets   int
	getErr error
	setErr error
}

func (f *fakeStoreAMDKeychain) Get(key string) ([]byte, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}

	return f.values[key], nil
}

func (f *fakeStoreAMDKeychain) Set(key string, data []byte) error {
	if f.setErr != nil {
		return f.setErr
	}

	f.sets++
	if f.values == nil {
		f.values = map[string][]byte{}
	}
	f.values[key] = data

	return nil
}

func (f *fakeStoreAMDKeychain) Remove(key string) error {
	delete(f.values, key)

	return nil
}

func TestIsStoreAMDStateMissing(t *testing.T) {
	missing := errors.New("failed to send http request: generate Store AMD proof: store AMD state is not provisioned")
	if !isStoreAMDStateMissing(missing) {
		t.Fatalf("expected the missing Store AMD state error to be recognised")
	}

	for _, err := range []error{nil, errors.New("received error: No Longer Available"), errors.New("store AMD context is nil")} {
		if isStoreAMDStateMissing(err) {
			t.Fatalf("expected %v not to be treated as a missing Store AMD state", err)
		}
	}
}

func TestProfileHasStoreAMDState(t *testing.T) {
	keychain := &fakeStoreAMDKeychain{values: map[string][]byte{guiStoreAMDStateKey: []byte("state")}}
	if !profileHasStoreAMDState(&guiAccountProfile{Keychain: keychain}) {
		t.Fatalf("expected a stored Store AMD state to be detected")
	}

	empty := &fakeStoreAMDKeychain{values: map[string][]byte{}}
	if profileHasStoreAMDState(&guiAccountProfile{Keychain: empty}) {
		t.Fatalf("expected an empty keychain to report no Store AMD state")
	}

	failing := &fakeStoreAMDKeychain{getErr: errors.New("keychain is locked")}
	if profileHasStoreAMDState(&guiAccountProfile{Keychain: failing}) {
		t.Fatalf("expected a keychain read error to report no Store AMD state")
	}

	saved := dependencies.Keychain
	dependencies.Keychain = nil
	t.Cleanup(func() { dependencies.Keychain = saved })

	if profileHasStoreAMDState(&guiAccountProfile{Keychain: nil}) {
		t.Fatalf("expected a profile without a keychain to report no Store AMD state")
	}
}

func TestEnsureStoreAMDStateKeepsProvisionedProfile(t *testing.T) {
	keychain := &fakeStoreAMDKeychain{values: map[string][]byte{guiStoreAMDStateKey: []byte("state")}}
	profile := &guiAccountProfile{Keychain: keychain}

	if err := ensureStoreAMDState(context.Background(), profile); err != nil {
		t.Fatalf("unexpected error for a provisioned profile: %v", err)
	}

	if keychain.sets != 0 {
		t.Fatalf("expected no keychain writes for a provisioned profile, got %d", keychain.sets)
	}

	if !profile.storeAMDReady {
		t.Fatalf("expected the profile to be marked ready")
	}

	// A second call must stay on the cached fast path and never read again.
	keychain.getErr = errors.New("keychain is locked")
	if err := ensureStoreAMDState(context.Background(), profile); err != nil {
		t.Fatalf("unexpected error on the cached fast path: %v", err)
	}
}

func TestProvisionStoreAMDStateRejectsMissingKeychain(t *testing.T) {
	saved := dependencies.Keychain
	dependencies.Keychain = nil
	t.Cleanup(func() { dependencies.Keychain = saved })

	profile := &guiAccountProfile{Email: "someone@example.com"}
	err := provisionStoreAMDState(context.Background(), profile, true)
	if err == nil {
		t.Fatalf("expected an error when no keychain is available")
	}

	if profile.storeAMDReady {
		t.Fatalf("expected the profile to stay unprovisioned")
	}
}

func TestGuiProfileLabel(t *testing.T) {
	if got := guiProfileLabel(nil); got != "the active Apple ID" {
		t.Fatalf("label = %q, want %q", got, "the active Apple ID")
	}

	profile := &guiAccountProfile{guiAccountRecord: guiAccountRecord{Email: " someone@example.com "}}
	if got := guiProfileLabel(profile); got != "someone@example.com" {
		t.Fatalf("label = %q, want %q", got, "someone@example.com")
	}
}
