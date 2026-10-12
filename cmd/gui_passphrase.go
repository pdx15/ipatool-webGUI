package cmd

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const guiKeychainPassphraseFileName = "webgui-keychain-passphrase"

// ensureGUIKeychainPassphrase supplies a stable, random passphrase to ipatool's
// file-backed keyring so the browser GUI never waits for terminal input. A
// caller-provided --keychain-passphrase always takes precedence. The generated
// value is kept beside ipatool's state files with owner-only file permissions;
// its protection therefore relies on the operating-system user account.
func ensureGUIKeychainPassphrase() error {
	if keychainPassphrase != "" {
		return nil
	}
	if dependencies.OS == nil || dependencies.Machine == nil {
		return errors.New("ipatool dependencies are not initialized")
	}

	stateDirectory, err := prepareStateDirectory(dependencies.OS, dependencies.Machine.HomeDirectory())
	if err != nil {
		return fmt.Errorf("failed to locate ipatool state directory: %w", err)
	}

	passphrase, err := loadOrCreateGUIKeychainPassphrase(filepath.Join(stateDirectory, guiKeychainPassphraseFileName))
	if err != nil {
		return fmt.Errorf("failed to prepare GUI keychain passphrase: %w", err)
	}

	keychainPassphrase = passphrase
	return nil
}

// loadOrCreateGUIKeychainPassphrase reuses the persisted key or creates a
// cryptographically random one on first GUI launch. Installing the temporary
// file with a hard link makes creation atomic so two GUI instances cannot end
// up encrypting the same keychain with different generated passphrases.
func loadOrCreateGUIKeychainPassphrase(path string) (string, error) {
	if passphrase, err := readGUIKeychainPassphrase(path); err == nil {
		return passphrase, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}

	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0700); err != nil {
		return "", fmt.Errorf("failed to create keychain state directory: %w", err)
	}

	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return "", fmt.Errorf("failed to generate random passphrase: %w", err)
	}
	passphrase := base64.RawURLEncoding.EncodeToString(secret)

	temp, err := os.CreateTemp(directory, ".ipatool-passphrase-*")
	if err != nil {
		return "", fmt.Errorf("failed to create temporary passphrase file: %w", err)
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath) //nolint:errcheck

	if err := temp.Chmod(0600); err != nil {
		_ = temp.Close()
		return "", fmt.Errorf("failed to secure temporary passphrase file: %w", err)
	}
	if _, err := io.WriteString(temp, passphrase); err != nil {
		_ = temp.Close()
		return "", fmt.Errorf("failed to write temporary passphrase file: %w", err)
	}
	if err := temp.Sync(); err != nil {
		_ = temp.Close()
		return "", fmt.Errorf("failed to sync temporary passphrase file: %w", err)
	}
	if err := temp.Close(); err != nil {
		return "", fmt.Errorf("failed to close temporary passphrase file: %w", err)
	}

	if err := os.Link(tempPath, path); err == nil {
		return passphrase, nil
	} else if errors.Is(err, os.ErrExist) {
		// Another GUI process created the persistent key first.
		return readGUIKeychainPassphrase(path)
	}

	// Some filesystems do not support hard links. Retain the exclusive-create
	// guarantee there; readers retry briefly if another process is still writing.
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return readGUIKeychainPassphrase(path)
		}
		return "", fmt.Errorf("failed to install persistent passphrase: %w", err)
	}
	if _, err := io.WriteString(file, passphrase); err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return "", fmt.Errorf("failed to write persistent passphrase: %w", err)
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return "", fmt.Errorf("failed to sync persistent passphrase: %w", err)
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(path)
		return "", fmt.Errorf("failed to close persistent passphrase: %w", err)
	}

	return passphrase, nil
}

func readGUIKeychainPassphrase(path string) (string, error) {
	for attempt := 0; attempt < 5; attempt++ {
		data, err := os.ReadFile(path)
		if err != nil {
			return "", err
		}

		passphrase := strings.TrimSpace(string(data))
		if passphrase != "" {
			// Tighten permissions for a file created by an older GUI build or
			// Python fallback. Windows applies its own per-user ACLs; chmod is
			// best-effort there.
			if err := os.Chmod(path, 0600); err != nil && !errors.Is(err, os.ErrPermission) {
				return "", fmt.Errorf("failed to secure persistent passphrase file: %w", err)
			}

			return passphrase, nil
		}

		// An exclusive-create fallback can briefly expose an empty file while
		// another GUI instance is writing it.
		time.Sleep(10 * time.Millisecond)
	}

	return "", errors.New("persistent passphrase file is empty")
}
