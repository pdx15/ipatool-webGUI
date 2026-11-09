package cmd

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/majd/ipatool/v2/internal/storeauth"
	pkghttp "github.com/majd/ipatool/v2/pkg/http"
	"github.com/majd/ipatool/v2/pkg/keychain"
)

const (
	// guiStoreAMDStateKey mirrors the keychain entry ipatool v2.8.0 reads the
	// Apple Media Services machine identity from when it signs Store requests.
	guiStoreAMDStateKey = "anisette-v4"
	// guiStoreAMDProvisionTimeout bounds the one-time identity provisioning.
	// It may download Apple's runtime assets before answering, so it is given
	// more room than a regular request.
	guiStoreAMDProvisionTimeout = 10 * time.Minute
)

// Sessions that are not owned by a saved Apple ID profile (the process-wide
// dependencies.AppStore) are provisioned once as well.
var (
	fallbackStoreAMDMu    sync.Mutex
	fallbackStoreAMDReady bool
)

// guiProfileKeychain returns the keychain that holds the profile's session.
// Every saved Apple ID owns a keychain, and the migrated legacy profile keeps
// using ipatool's own keychain.
func guiProfileKeychain(profile *guiAccountProfile) keychain.Keychain {
	if profile != nil && profile.Keychain != nil {
		return profile.Keychain
	}

	return dependencies.Keychain
}

// keychainHasStoreAMDState reports whether the keychain already stores a
// provisioned Store machine identity. A keychain error counts as "missing".
func keychainHasStoreAMDState(store keychain.Keychain) bool {
	if store == nil {
		return false
	}

	state, err := store.Get(guiStoreAMDStateKey)

	return err == nil && len(state) > 0
}

// profileHasStoreAMDState reports whether the profile already stores a
// provisioned Store machine identity.
func profileHasStoreAMDState(profile *guiAccountProfile) bool {
	return keychainHasStoreAMDState(guiProfileKeychain(profile))
}

// isStoreAMDStateMissing reports whether err is the ipatool v2.8.0 failure that
// means the Store machine identity was never provisioned for this session.
func isStoreAMDStateMissing(err error) bool {
	if err == nil {
		return false
	}

	return strings.Contains(strings.ToLower(err.Error()), "store amd state is not provisioned")
}

// ensureStoreAMDState provisions the Apple Media Services machine identity that
// ipatool v2.8.0 signs Store download requests with.
//
// Sessions saved by ipatool 2.7.0 and older never provisioned it: the identity
// is written only while signing in, and the GUI restores saved Apple IDs
// without a new login. Without it every Store request fails with
// "store AMD state is not provisioned", so the GUI provisions it once per
// session the first time a Store request is attempted. Provisioning is a
// machine-level exchange with Apple and does not need the Apple ID password.
func ensureStoreAMDState(ctx context.Context, profile *guiAccountProfile) error {
	if profile == nil {
		return ensureFallbackStoreAMDState(ctx)
	}

	profile.storeAMDMu.Lock()
	ready := profile.storeAMDReady
	profile.storeAMDMu.Unlock()
	if ready {
		return nil
	}

	if keychainHasStoreAMDState(guiProfileKeychain(profile)) {
		profile.storeAMDMu.Lock()
		profile.storeAMDReady = true
		profile.storeAMDMu.Unlock()

		return nil
	}

	return provisionStoreAMDState(ctx, profile, false)
}

// provisionStoreAMDState writes a fresh machine identity into the profile
// keychain. With force set it overrides a stored identity that Apple no longer
// accepts.
func provisionStoreAMDState(ctx context.Context, profile *guiAccountProfile, force bool) error {
	store := guiProfileKeychain(profile)
	if store == nil {
		return errors.New("the Apple Store session is not provisioned and no keychain is available; sign in to this Apple ID again")
	}

	profile.storeAMDMu.Lock()
	defer profile.storeAMDMu.Unlock()

	if !force && (profile.storeAMDReady || keychainHasStoreAMDState(store)) {
		// Another goroutine may have provisioned the identity while waiting.
		profile.storeAMDReady = true

		return nil
	}

	if err := provisionStoreAMDIdentity(ctx, store, profile.CookieJar, guiProfileLabel(profile)); err != nil {
		return err
	}

	profile.storeAMDReady = true

	return nil
}

// ensureFallbackStoreAMDState covers requests made through the process-wide
// App Store client, which has no saved Apple ID profile behind it.
func ensureFallbackStoreAMDState(ctx context.Context) error {
	fallbackStoreAMDMu.Lock()
	defer fallbackStoreAMDMu.Unlock()

	if fallbackStoreAMDReady || keychainHasStoreAMDState(dependencies.Keychain) {
		fallbackStoreAMDReady = true

		return nil
	}

	if dependencies.Keychain == nil {
		return errors.New("the Apple Store session is not provisioned and no keychain is available; sign in to an Apple ID again")
	}

	if err := provisionStoreAMDIdentity(ctx, dependencies.Keychain, dependencies.CookieJar, "the active Apple ID"); err != nil {
		return err
	}

	fallbackStoreAMDReady = true

	return nil
}

// provisionStoreAMDIdentity runs Apple's machine provisioning and stores the
// resulting identity in the keychain under the key ipatool reads it from. The
// exchange is a machine-level one: it needs the network and Apple's runtime
// assets, but not the Apple ID password.
func provisionStoreAMDIdentity(ctx context.Context, store keychain.Keychain, cookieJar pkghttp.CookieJar, label string) error {
	if ctx == nil {
		ctx = context.Background()
	}

	ctx, cancel := context.WithTimeout(ctx, guiStoreAMDProvisionTimeout)
	defer cancel()

	_, state, err := storeauth.AnisetteHeaders(ctx, storeauth.NewHTTPClient(cookieJar), nil)
	if err != nil {
		return fmt.Errorf("prepare the Apple Store session for %s, sign in to this Apple ID again if this keeps failing: %w", label, err)
	}

	if err := store.Set(guiStoreAMDStateKey, state); err != nil {
		return fmt.Errorf("save the Apple Store session for %s: %w", label, err)
	}

	return nil
}

// guiProfileLabel names the profile in errors without leaking account details.
func guiProfileLabel(profile *guiAccountProfile) string {
	if profile == nil {
		return "the active Apple ID"
	}

	if email := strings.TrimSpace(profile.Email); email != "" {
		return email
	}

	return "the active Apple ID"
}
