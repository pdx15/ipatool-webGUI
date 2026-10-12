package cmd

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"

	"github.com/majd/ipatool/v2/pkg/appstore"
	"howett.net/plist"
)

func TestRenameDownloadedIPAPackageUsesEmbeddedInfoPlist(t *testing.T) {
	directory := t.TempDir()
	source := filepath.Join(directory, "pinterest_429047995_14.39.ipa")
	writeTestIPA(t, source)

	destination, metadata, err := renameDownloadedIPAPackage(
		source,
		"",
		"App Store fallback",
		"spdx15@gmail.com",
		appstore.PlatformIPhone,
	)
	if err != nil {
		t.Fatalf("failed to rename downloaded IPA: %v", err)
	}

	if got, want := filepath.Base(destination), "Pinterest_14.39_iOS16.0_spdx15.ipa"; got != want {
		t.Fatalf("filename = %q, want %q", got, want)
	}
	if metadata.DisplayName != "Pinterest" || metadata.Version != "14.39" || metadata.MinimumOSVersion != "16.0" {
		t.Fatalf("unexpected Info.plist metadata: %+v", metadata)
	}
	if _, err := os.Stat(destination); err != nil {
		t.Fatalf("renamed IPA does not exist: %v", err)
	}
	if _, err := os.Stat(source); !os.IsNotExist(err) {
		t.Fatalf("original IPA still exists after rename: %v", err)
	}
}

func TestRenameDownloadedIPAPackagePreservesExplicitFilename(t *testing.T) {
	directory := t.TempDir()
	source := filepath.Join(directory, "custom.ipa")
	writeTestIPA(t, source)

	destination, metadata, err := renameDownloadedIPAPackage(
		source,
		source,
		"Pinterest",
		"spdx15@gmail.com",
		appstore.PlatformIPhone,
	)
	if err != nil {
		t.Fatalf("explicit output path should be preserved: %v", err)
	}
	if destination != source || metadata.DisplayName != "" {
		t.Fatalf("explicit output path changed: destination=%q metadata=%+v", destination, metadata)
	}
}

func writeTestIPA(t *testing.T, path string) {
	t.Helper()

	plistData, err := plist.Marshal(map[string]interface{}{
		"CFBundleDisplayName":        "Pinterest",
		"CFBundleName":               "Pinterest",
		"CFBundleShortVersionString": "14.39",
		"MinimumOSVersion":           "16.0",
		"CFBundleSupportedPlatforms": []string{"iPhoneOS"},
	}, plist.BinaryFormat)
	if err != nil {
		t.Fatalf("failed to create test Info.plist: %v", err)
	}

	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("failed to create test IPA: %v", err)
	}
	writer := zip.NewWriter(file)
	entry, err := writer.Create("Payload/Pinterest.app/Info.plist")
	if err != nil {
		_ = file.Close()
		t.Fatalf("failed to create Info.plist in test IPA: %v", err)
	}
	if _, err := entry.Write(plistData); err != nil {
		_ = file.Close()
		t.Fatalf("failed to write test Info.plist: %v", err)
	}
	if err := writer.Close(); err != nil {
		_ = file.Close()
		t.Fatalf("failed to close test IPA archive: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("failed to close test IPA file: %v", err)
	}
}
