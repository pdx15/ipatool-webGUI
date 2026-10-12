package cmd

import (
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
)

func TestLoadOrCreateGUIKeychainPassphrasePersists(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".ipatool", guiKeychainPassphraseFileName)

	first, err := loadOrCreateGUIKeychainPassphrase(path)
	if err != nil {
		t.Fatalf("failed to create passphrase: %v", err)
	}
	if first == "" {
		t.Fatal("generated passphrase is empty")
	}

	second, err := loadOrCreateGUIKeychainPassphrase(path)
	if err != nil {
		t.Fatalf("failed to load passphrase: %v", err)
	}
	if second != first {
		t.Fatalf("passphrase changed between launches: first %q, second %q", first, second)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("failed to stat passphrase file: %v", err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0600 {
		t.Fatalf("passphrase file permissions = %o, want 600", info.Mode().Perm())
	}
}

func TestLoadOrCreateGUIKeychainPassphraseIsAtomic(t *testing.T) {
	path := filepath.Join(t.TempDir(), guiKeychainPassphraseFileName)
	const workers = 12

	var wait sync.WaitGroup
	results := make(chan string, workers)
	errs := make(chan error, workers)
	for range workers {
		wait.Add(1)
		go func() {
			defer wait.Done()
			passphrase, err := loadOrCreateGUIKeychainPassphrase(path)
			if err != nil {
				errs <- err
					return
			}
			results <- passphrase
		}()
	}
	wait.Wait()
	close(results)
	close(errs)

	for err := range errs {
		t.Errorf("concurrent passphrase creation failed: %v", err)
	}

	var expected string
	for passphrase := range results {
		if expected == "" {
			expected = passphrase
			continue
		}
		if passphrase != expected {
			t.Errorf("concurrent callers received different passphrases: %q and %q", expected, passphrase)
		}
	}
	if expected == "" {
		t.Fatal("no passphrase was returned")
	}
}
