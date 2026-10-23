package cmd

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/majd/ipatool/v2/pkg/appstore"
)

func TestNewGUIAccountIDIsUniqueRandomHex(t *testing.T) {
	first, err := newGUIAccountID()
	if err != nil {
		t.Fatalf("newGUIAccountID() failed: %v", err)
	}
	second, err := newGUIAccountID()
	if err != nil {
		t.Fatalf("newGUIAccountID() failed: %v", err)
	}
	if first == second {
		t.Fatal("expected distinct account IDs")
	}
	if len(first) != 32 || len(second) != 32 {
		t.Fatalf("expected 128-bit hex IDs, got lengths %d and %d", len(first), len(second))
	}
	if _, err := hex.DecodeString(first); err != nil {
		t.Fatalf("account ID is not hexadecimal: %q", first)
	}
	for _, invalid := range []string{"", "legacy", "../escape", "not-hex"} {
		if validGUIAccountID(invalid) {
			t.Errorf("validGUIAccountID(%q) = true, want false", invalid)
		}
	}
}

func TestGUIAccountManagerPersistsMetadataWithoutCredentials(t *testing.T) {
	stateDir := t.TempDir()
	account := appstore.Account{
		Email:               "apple@example.com",
		Name:                "Apple User",
		StoreFront:          "143441-1,29",
		DirectoryServicesID: "123456789",
		Password:            "raw-password-must-not-persist",
		PasswordToken:       "password-token-must-not-persist",
	}
	profile := &guiAccountProfile{
		guiAccountRecord: guiAccountRecordFromAccount("profile-1", account, false),
	}
	manager := &guiAccountManager{
		stateDir: stateDir,
		profiles: map[string]*guiAccountProfile{profile.ID: profile},
		activeID: profile.ID,
	}

	if err := manager.persist(); err != nil {
		t.Fatalf("persist() failed: %v", err)
	}

	path := filepath.Join(stateDir, guiAccountsFileName)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("could not read account metadata: %v", err)
	}
	for _, secret := range []string{"raw-password-must-not-persist", "password-token-must-not-persist"} {
		if strings.Contains(string(data), secret) {
			t.Fatalf("account metadata unexpectedly contains a secret: %q", secret)
		}
	}

	var saved guiAccountsFile
	if err := json.Unmarshal(data, &saved); err != nil {
		t.Fatalf("saved account metadata is invalid JSON: %v", err)
	}
	if saved.ActiveID != profile.ID || len(saved.Accounts) != 1 || saved.Accounts[0].Email != profile.Email {
		t.Fatalf("unexpected saved account metadata: %+v", saved)
	}
}

func TestNextGUIAccountIDIsDeterministicAndSkipsRemovingProfiles(t *testing.T) {
	profiles := map[string]*guiAccountProfile{
		"b-account": {guiAccountRecord: guiAccountRecord{ID: "b-account"}},
		"a-removing": {
			guiAccountRecord: guiAccountRecord{ID: "a-removing"},
			removing:         true,
		},
	}
	if got := nextGUIAccountID(profiles); got != "b-account" {
		t.Fatalf("nextGUIAccountID() = %q, want b-account", got)
	}
	if got := nextGUIAccountID(nil); got != "" {
		t.Fatalf("nextGUIAccountID(nil) = %q, want empty", got)
	}
}

func TestRegisterGUIAccountJobRejectsRemovedProfiles(t *testing.T) {
	profile := &guiAccountProfile{guiAccountRecord: guiAccountRecord{ID: "profile-a"}}
	manager := &guiAccountManager{
		profiles: map[string]*guiAccountProfile{profile.ID: profile},
	}
	previous := guiAccounts
	guiAccounts = manager
	t.Cleanup(func() { guiAccounts = previous })

	registered := false
	if err := registerGUIAccountJob(profile, func() { registered = true }); err != nil {
		t.Fatalf("registerGUIAccountJob() failed for an available profile: %v", err)
	}
	if !registered {
		t.Fatal("expected the job registration callback to run")
	}

	registered = false
	profile.removing = true
	if err := registerGUIAccountJob(profile, func() { registered = true }); err == nil {
		t.Fatal("expected registration to fail while the profile is being removed")
	}
	if registered {
		t.Fatal("job was registered for a profile being removed")
	}
}

func TestPurchasesCachesAreIsolatedByAppleID(t *testing.T) {
	first := purchasesCacheForAccount(appstore.Account{
		Email:               "profile-a@example.com",
		DirectoryServicesID: "arena-test-profile-a-93001",
	})
	second := purchasesCacheForAccount(appstore.Account{
		Email:               "profile-b@example.com",
		DirectoryServicesID: "arena-test-profile-b-93002",
	})
	sameAccount := purchasesCacheForAccount(appstore.Account{
		Email:               "renamed@example.com",
		DirectoryServicesID: "arena-test-profile-a-93001",
	})

	if first == second {
		t.Fatal("different Apple IDs must not share a purchases cache")
	}
	if first != sameAccount {
		t.Fatal("the same DSID should reuse its purchases cache even if the email changes")
	}
}

func TestBatchCheckTimeoutKeepsAccountInUseUntilProbeReturns(t *testing.T) {
	entered := make(chan struct{})
	resume := make(chan struct{})
	profile := &guiAccountProfile{
		guiAccountRecord: guiAccountRecord{ID: "batch-timeout-account"},
		AppStore: &fakeAppStore{
			listVersions: func(appstore.ListVersionsInput) (appstore.ListVersionsOutput, error) {
				close(entered)
				<-resume
				return appstore.ListVersionsOutput{}, errors.New("probe finished after timeout")
			},
		},
	}
	manager := &guiAccountManager{
		stateDir: t.TempDir(),
		profiles: map[string]*guiAccountProfile{profile.ID: profile},
		activeID: profile.ID,
	}
	previous := guiAccounts
	guiAccounts = manager
	t.Cleanup(func() { guiAccounts = previous })

	result := runBatchCheckItem(profile, appstore.App{ID: 12345}, appstore.Account{Email: "batch@example.com"}, appstore.PlatformIPhone, false, 20*time.Millisecond)
	if !result.skipped {
		t.Fatalf("expected timed-out probe to be marked skipped, got %+v", result)
	}
	<-entered

	if err := manager.remove(profile.ID); err == nil {
		t.Fatal("account removal should wait for the timed-out probe goroutine")
	}

	close(resume)
	deadline := time.After(time.Second)
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	for {
		manager.mu.RLock()
		active := profile.activeOperations
		manager.mu.RUnlock()
		if active == 0 {
			break
		}
		select {
		case <-ticker.C:
		case <-deadline:
			t.Fatal("timed-out probe did not release its account operation")
		}
	}
}
