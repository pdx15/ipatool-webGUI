package cmd

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/byteness/keyring"
	"github.com/majd/ipatool/v2/pkg/appstore"
	pkghttp "github.com/majd/ipatool/v2/pkg/http"
)

const (
	guiAccountsFileName      = "accounts.json"
	guiAccountsDirectoryName = "accounts"
	guiLegacyAccountID       = "legacy"
	guiLoginAttemptTTL       = 10 * time.Minute
)

// guiAccountRecord contains only non-secret display and routing metadata.
// App Store credentials stay in the per-profile keychain managed by ipatool.
type guiAccountRecord struct {
	ID         string `json:"id"`
	Email      string `json:"email,omitempty"`
	Name       string `json:"name,omitempty"`
	Storefront string `json:"storefront,omitempty"`
	DSID       string `json:"dsid,omitempty"`
	Legacy     bool   `json:"legacy,omitempty"`
}

type guiAccountsFile struct {
	Version  int                `json:"version"`
	ActiveID string             `json:"activeAccountId,omitempty"`
	Accounts []guiAccountRecord `json:"accounts"`
}

// guiAccountProfile owns all session-bearing dependencies for one Apple ID.
// It is intentionally kept separate from the JSON metadata persisted to disk.
type guiAccountProfile struct {
	guiAccountRecord
	AppStore         appstore.AppStore
	CookieJar        pkghttp.CookieJar
	stateDir         string
	removing         bool // protected by guiAccountManager.mu
	activeOperations int  // protected by guiAccountManager.mu; prevents deleting an in-use profile
	refreshMu        sync.Mutex
}

type guiAccountView struct {
	ID            string `json:"id"`
	Email         string `json:"email,omitempty"`
	Name          string `json:"name,omitempty"`
	Storefront    string `json:"storefront,omitempty"`
	DSID          string `json:"dsid,omitempty"`
	Authenticated bool   `json:"authenticated"`
	Active        bool   `json:"active"`
}

type guiPendingLogin struct {
	ID         string
	Email      string
	Password   string
	Profile    *guiAccountProfile
	NewProfile bool
	CreatedAt  time.Time
}

type guiAccountManager struct {
	mu            sync.RWMutex
	persistMu     sync.Mutex
	stateDir      string
	interactive   bool
	profiles      map[string]*guiAccountProfile
	activeID      string
	pendingLogins map[string]*guiPendingLogin
}

var guiAccounts *guiAccountManager

func newGUIAccountID() (string, error) {
	var data [16]byte
	if _, err := rand.Read(data[:]); err != nil {
		return "", fmt.Errorf("failed to generate account identifier: %w", err)
	}
	return hex.EncodeToString(data[:]), nil
}

func validGUIAccountID(id string) bool {
	decoded, err := hex.DecodeString(id)
	return err == nil && len(decoded) == 16
}

func newGUIAccountProfile(record guiAccountRecord, stateDir string, interactive bool) (*guiAccountProfile, error) {
	if record.ID == guiLegacyAccountID {
		record.Legacy = true

		return &guiAccountProfile{
			guiAccountRecord: record,
			AppStore:         dependencies.AppStore,
			CookieJar:        dependencies.CookieJar,
			stateDir:         stateDir,
		}, nil
	}
	if record.Legacy || !validGUIAccountID(record.ID) {
		return nil, fmt.Errorf("invalid Apple ID account identifier %q", record.ID)
	}

	profileDir := filepath.Join(stateDir, guiAccountsDirectoryName, record.ID)
	if err := os.MkdirAll(profileDir, 0o700); err != nil {
		return nil, fmt.Errorf("failed to create account state directory: %w", err)
	}
	if err := os.Chmod(profileDir, 0o700); err != nil && !errors.Is(err, os.ErrPermission) {
		return nil, fmt.Errorf("failed to secure account state directory: %w", err)
	}

	serviceName := guiKeychainServiceName + "." + record.ID
	cookieJar := newGUICookieJar(profileDir)
	accountKeychain := newGUIKeychainWithService(profileDir, interactive, serviceName)
	store := appstore.NewAppStore(appstore.Args{
		CookieJar:       cookieJar,
		OperatingSystem: dependencies.OS,
		Keychain:        accountKeychain,
		Machine:         dependencies.Machine,
	})

	return &guiAccountProfile{
		guiAccountRecord: record,
		AppStore:         store,
		CookieJar:        cookieJar,
		stateDir:         profileDir,
	}, nil
}

func initializeGUIAccountManager(stateDir string, interactive bool) error {
	manager := &guiAccountManager{
		stateDir:      stateDir,
		interactive:   interactive,
		profiles:      make(map[string]*guiAccountProfile),
		pendingLogins: make(map[string]*guiPendingLogin),
	}

	configPath := filepath.Join(stateDir, guiAccountsFileName)
	data, err := os.ReadFile(configPath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("failed to read saved Apple ID accounts: %w", err)
	}

	if err == nil {
		var config guiAccountsFile
		if unmarshalErr := json.Unmarshal(data, &config); unmarshalErr != nil {
			return fmt.Errorf("failed to parse saved Apple ID accounts: %w", unmarshalErr)
		}
		for _, record := range config.Accounts {
			if strings.TrimSpace(record.ID) == "" {
				continue
			}
			profile, profileErr := newGUIAccountProfile(record, stateDir, interactive)
			if profileErr != nil {
				return profileErr
			}
			manager.profiles[record.ID] = profile
		}
		manager.activeID = config.ActiveID
	} else {
		// Migrate the pre-multi-account session in place. Keeping this profile
		// on ipatool's original keychain and cookie jar avoids forcing a login.
		if info, infoErr := dependencies.AppStore.AccountInfo(); infoErr == nil {
			record := guiAccountRecordFromAccount(guiLegacyAccountID, info.Account, true)
			profile, profileErr := newGUIAccountProfile(record, stateDir, interactive)
			if profileErr != nil {
				return profileErr
			}
			manager.profiles[record.ID] = profile
			manager.activeID = record.ID
		}
	}

	if _, exists := manager.profiles[manager.activeID]; !exists {
		manager.activeID = ""
	}
	if manager.activeID == "" {
		manager.activeID = nextGUIAccountID(manager.profiles)
	}

	guiAccounts = manager
	if err == nil || len(manager.profiles) > 0 {
		if saveErr := manager.persist(); saveErr != nil {
			return saveErr
		}
	}
	return nil
}

func guiAccountRecordFromAccount(id string, account appstore.Account, legacy bool) guiAccountRecord {
	return guiAccountRecord{
		ID:         id,
		Email:      account.Email,
		Name:       account.Name,
		Storefront: account.StoreFront,
		DSID:       account.DirectoryServicesID,
		Legacy:     legacy,
	}
}

func nextGUIAccountID(profiles map[string]*guiAccountProfile) string {
	ids := make([]string, 0, len(profiles))
	for id, profile := range profiles {
		if profile != nil && !profile.removing {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	if len(ids) == 0 {
		return ""
	}
	return ids[0]
}

func (m *guiAccountManager) persist() error {
	m.persistMu.Lock()
	defer m.persistMu.Unlock()

	m.mu.RLock()
	config := guiAccountsFile{
		Version:  1,
		ActiveID: m.activeID,
		Accounts: make([]guiAccountRecord, 0, len(m.profiles)),
	}
	for _, profile := range m.profiles {
		config.Accounts = append(config.Accounts, profile.guiAccountRecord)
	}
	m.mu.RUnlock()
	sort.Slice(config.Accounts, func(i, j int) bool {
		return strings.ToLower(config.Accounts[i].Email) < strings.ToLower(config.Accounts[j].Email)
	})

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to encode saved Apple ID accounts: %w", err)
	}
	path := filepath.Join(m.stateDir, guiAccountsFileName)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("failed to save Apple ID accounts: %w", err)
	}
	if err := os.Chmod(path, 0o600); err != nil && !errors.Is(err, os.ErrPermission) {
		return fmt.Errorf("failed to secure saved Apple ID accounts: %w", err)
	}
	return nil
}

func (m *guiAccountManager) activeProfile() (*guiAccountProfile, bool) {
	if m == nil {
		return nil, false
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	profile, ok := m.profiles[m.activeID]
	return profile, ok && profile != nil && !profile.removing
}

func (m *guiAccountManager) profile(id string) (*guiAccountProfile, bool) {
	if m == nil {
		return nil, false
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	id = strings.TrimSpace(id)
	if id == "" {
		id = m.activeID
	}
	profile, ok := m.profiles[id]
	return profile, ok && profile != nil && !profile.removing
}

func (m *guiAccountManager) acquireProfileOperation(profile *guiAccountProfile) (func(), error) {
	if m == nil || profile == nil {
		return nil, errors.New("Apple ID account profile is unavailable")
	}

	m.mu.Lock()
	current, exists := m.profiles[profile.ID]
	if !exists || current != profile || profile.removing {
		m.mu.Unlock()
		return nil, errors.New("Apple ID account is being removed or is no longer available")
	}
	profile.activeOperations++
	m.mu.Unlock()

	var once sync.Once
	return func() {
		once.Do(func() {
			m.mu.Lock()
			if profile.activeOperations > 0 {
				profile.activeOperations--
			}
			m.mu.Unlock()
		})
	}, nil
}

func acquireGUIAccountOperation(profile *guiAccountProfile) (func(), error) {
	if profile == nil {
		return func() {}, nil
	}
	if guiAccounts == nil {
		return nil, errors.New("Apple ID account manager is not initialized")
	}
	return guiAccounts.acquireProfileOperation(profile)
}

func (m *guiAccountManager) setActive(id string) error {
	id = strings.TrimSpace(id)
	m.mu.Lock()
	profile, ok := m.profiles[id]
	if !ok || profile == nil || profile.removing {
		m.mu.Unlock()
		return fmt.Errorf("Apple ID account %q was not found", id)
	}
	m.activeID = id
	m.mu.Unlock()
	return m.persist()
}

func (m *guiAccountManager) updateAccount(profile *guiAccountProfile, account appstore.Account) error {
	if profile == nil {
		return errors.New("Apple ID account profile is missing")
	}
	m.mu.Lock()
	current, exists := m.profiles[profile.ID]
	if !exists || current != profile || profile.removing {
		m.mu.Unlock()
		return errors.New("Apple ID account profile is no longer available")
	}
	profile.guiAccountRecord = guiAccountRecordFromAccount(profile.ID, account, profile.Legacy)
	m.mu.Unlock()
	return m.persist()
}

func (m *guiAccountManager) authenticatedProfile(id string) (*guiAccountProfile, appstore.Account, func(), error) {
	profile, ok := m.profile(id)
	if !ok {
		return nil, appstore.Account{}, nil, errors.New("no Apple ID account selected; add or select an account first")
	}
	release, err := m.acquireProfileOperation(profile)
	if err != nil {
		return nil, appstore.Account{}, nil, err
	}
	info, err := profile.AppStore.AccountInfo()
	if err != nil {
		release()
		return nil, appstore.Account{}, nil, err
	}
	_ = m.updateAccount(profile, info.Account)
	return profile, info.Account, release, nil
}

func (m *guiAccountManager) views() ([]guiAccountView, string, *guiAccountView) {
	if m == nil {
		return []guiAccountView{}, "", nil
	}
	type profileSnapshot struct {
		profile *guiAccountProfile
		record  guiAccountRecord
	}

	m.mu.RLock()
	activeID := m.activeID
	profiles := make([]profileSnapshot, 0, len(m.profiles))
	for _, profile := range m.profiles {
		if profile != nil && !profile.removing {
			profiles = append(profiles, profileSnapshot{profile: profile, record: profile.guiAccountRecord})
		}
	}
	m.mu.RUnlock()

	views := make([]guiAccountView, 0, len(profiles))
	var active *guiAccountView
	for _, snapshot := range profiles {
		profile := snapshot.profile
		record := snapshot.record
		view := guiAccountView{
			ID:         record.ID,
			Email:      record.Email,
			Name:       record.Name,
			Storefront: record.Storefront,
			DSID:       record.DSID,
			Active:     record.ID == activeID,
		}
		if release, err := m.acquireProfileOperation(profile); err == nil {
			if info, infoErr := profile.AppStore.AccountInfo(); infoErr == nil {
				view.Authenticated = true
				view.Email = info.Account.Email
				view.Name = info.Account.Name
				view.Storefront = info.Account.StoreFront
				view.DSID = info.Account.DirectoryServicesID
				_ = m.updateAccount(profile, info.Account)
			}
			release()
		}
		views = append(views, view)
		if view.Active {
			activeCopy := view
			active = &activeCopy
		}
	}
	sort.Slice(views, func(i, j int) bool {
		if views[i].Active != views[j].Active {
			return views[i].Active
		}
		return strings.ToLower(views[i].Email) < strings.ToLower(views[j].Email)
	})
	if active != nil {
		for i := range views {
			if views[i].ID == active.ID {
				active = &views[i]
				break
			}
		}
	}
	return views, activeID, active
}

func (m *guiAccountManager) findByEmail(email string) *guiAccountProfile {
	normalized := strings.ToLower(strings.TrimSpace(email))
	if normalized == "" {
		return nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, profile := range m.profiles {
		if profile != nil && !profile.removing && strings.EqualFold(strings.TrimSpace(profile.Email), normalized) {
			return profile
		}
	}
	return nil
}

func (m *guiAccountManager) prunePendingLoginsLocked(now time.Time) {
	for id, pending := range m.pendingLogins {
		if now.Sub(pending.CreatedAt) > guiLoginAttemptTTL {
			pending.Password = ""
			delete(m.pendingLogins, id)
		}
	}
}

func (m *guiAccountManager) login(email, password, authCode, attemptID string) (appstore.Account, string, error) {
	email = strings.TrimSpace(email)
	attemptID = strings.TrimSpace(attemptID)

	var pending *guiPendingLogin
	newAttempt := false
	if attemptID != "" {
		m.mu.Lock()
		m.prunePendingLoginsLocked(time.Now())
		pending = m.pendingLogins[attemptID]
		if pending != nil {
			email = pending.Email
			password = pending.Password
		}
		m.mu.Unlock()
		if pending == nil {
			return appstore.Account{}, "", errors.New("login challenge expired; start signing in again")
		}
	} else {
		if email == "" || password == "" {
			return appstore.Account{}, "", errors.New("email and password are required")
		}
		profile := m.findByEmail(email)
		newProfile := profile == nil
		if newProfile {
			id, err := newGUIAccountID()
			if err != nil {
				return appstore.Account{}, "", err
			}
			profile, err = newGUIAccountProfile(guiAccountRecord{ID: id, Email: email}, m.stateDir, m.interactive)
			if err != nil {
				return appstore.Account{}, "", err
			}
		}
		id, err := newGUIAccountID()
		if err != nil {
			return appstore.Account{}, "", err
		}
		pending = &guiPendingLogin{
			ID:         id,
			Email:      email,
			Password:   password,
			Profile:    profile,
			NewProfile: newProfile,
			CreatedAt:  time.Now(),
		}
		m.mu.Lock()
		m.prunePendingLoginsLocked(time.Now())
		m.pendingLogins[id] = pending
		m.mu.Unlock()
		time.AfterFunc(guiLoginAttemptTTL, func() { m.clearPendingLogin(id) })
		attemptID = id
		newAttempt = true
	}

	release := func() {}
	if !pending.NewProfile {
		var acquireErr error
		release, acquireErr = m.acquireProfileOperation(pending.Profile)
		if acquireErr != nil {
			m.clearPendingLogin(attemptID)
			return appstore.Account{}, "", acquireErr
		}
	}
	defer release()

	output, err := pending.Profile.AppStore.Login(appstore.LoginInput{
		Email:    email,
		Password: password,
		AuthCode: authCode,
	})
	if err != nil {
		if errors.Is(err, appstore.ErrAuthCodeRequired) {
			return appstore.Account{}, attemptID, err
		}
		if newAttempt {
			m.clearPendingLogin(attemptID)
			if !m.hasProfile(pending.Profile.ID) && !pending.Profile.Legacy {
				_ = os.RemoveAll(pending.Profile.stateDir)
			}
		}
		return appstore.Account{}, attemptID, err
	}

	if persistErr := m.commitLogin(pending.Profile, output.Account, pending.NewProfile); persistErr != nil {
		m.clearPendingLogin(attemptID)
		return appstore.Account{}, "", persistErr
	}
	m.clearPendingLogin(attemptID)
	return output.Account, "", nil
}

func (m *guiAccountManager) clearPendingLogin(id string) {
	m.mu.Lock()
	if pending := m.pendingLogins[id]; pending != nil {
		pending.Password = ""
		delete(m.pendingLogins, id)
	}
	m.mu.Unlock()
}

func (m *guiAccountManager) hasProfile(id string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, ok := m.profiles[id]
	return ok
}

func (m *guiAccountManager) commitLogin(profile *guiAccountProfile, account appstore.Account, allowNewProfile bool) error {
	if profile == nil {
		return errors.New("Apple ID account profile is missing")
	}
	m.mu.Lock()
	if profile.removing {
		m.mu.Unlock()
		return errors.New("Apple ID account profile is being removed")
	}
	current, exists := m.profiles[profile.ID]
	if exists && current != profile {
		m.mu.Unlock()
		return errors.New("Apple ID account profile was replaced during login")
	}
	if !exists && !allowNewProfile {
		m.mu.Unlock()
		return errors.New("Apple ID account was removed before login completed")
	}
	profile.guiAccountRecord = guiAccountRecordFromAccount(profile.ID, account, profile.Legacy)
	m.profiles[profile.ID] = profile
	m.activeID = profile.ID
	m.mu.Unlock()
	return m.persist()
}

func (m *guiAccountManager) remove(id string) error {
	id = strings.TrimSpace(id)
	if m == nil {
		return errors.New("Apple ID account manager is not initialized")
	}

	m.mu.Lock()
	profile, ok := m.profiles[id]
	if !ok || profile == nil || profile.removing {
		m.mu.Unlock()
		return errors.New("Apple ID account was not found")
	}
	if err := guiAccountHasActiveJobs(profile.ID); err != nil {
		m.mu.Unlock()
		return err
	}
	if profile.activeOperations > 0 {
		m.mu.Unlock()
		return errors.New("this Apple ID is being used by active requests; wait for them to finish before removing the account")
	}
	profile.removing = true
	record := profile.guiAccountRecord
	previousActiveID := m.activeID
	m.mu.Unlock()

	account := appstore.Account{Email: record.Email, DirectoryServicesID: record.DSID}
	if info, err := profile.AppStore.AccountInfo(); err == nil {
		account = info.Account
	}
	if revokeErr := profile.AppStore.Revoke(); revokeErr != nil &&
		!errors.Is(revokeErr, keyring.ErrKeyNotFound) && !errors.Is(revokeErr, os.ErrNotExist) {
		m.mu.Lock()
		if m.profiles[profile.ID] == profile {
			profile.removing = false
		}
		m.mu.Unlock()
		return fmt.Errorf("failed to revoke Apple ID session: %w", revokeErr)
	}

	m.mu.Lock()
	if m.profiles[profile.ID] != profile {
		m.mu.Unlock()
		return errors.New("Apple ID account changed while it was being removed")
	}
	delete(m.profiles, profile.ID)
	if m.activeID == profile.ID {
		m.activeID = nextGUIAccountID(m.profiles)
	}
	m.mu.Unlock()

	if err := m.persist(); err != nil {
		m.mu.Lock()
		m.profiles[profile.ID] = profile
		profile.removing = false
		m.activeID = previousActiveID
		m.mu.Unlock()
		return err
	}
	if !profile.Legacy && profile.stateDir != "" {
		_ = os.RemoveAll(profile.stateDir)
	}
	invalidatePurchasesCache(account)
	return nil
}

func selectedGUIAccountID(rQuery string) string {
	return strings.TrimSpace(rQuery)
}

func activeGUIAccountID() string {
	if guiAccounts == nil {
		return ""
	}
	guiAccounts.mu.RLock()
	defer guiAccounts.mu.RUnlock()
	return guiAccounts.activeID
}
