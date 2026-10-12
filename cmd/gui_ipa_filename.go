package cmd

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"unicode"

	"github.com/majd/ipatool/v2/pkg/appstore"
	"howett.net/plist"
)

const downloadFilenameFallbackWarning = "ipatool-gui-filename-fallback"

type ipaFilenameMetadata struct {
	DisplayName      string
	Version          string
	MinimumOSVersion string
	PlatformName     string
}

// renameDownloadedIPAPackage applies the GUI's descriptive filename to an IPA
// when ipatool chose the path itself (or the user chose an output directory).
// An explicitly supplied output filename is left untouched.
func renameDownloadedIPAPackage(path, requestedOutputPath, fallbackAppName, accountEmail string, platform appstore.Platform) (string, ipaFilenameMetadata, error) {
	if !strings.EqualFold(filepath.Ext(path), ".ipa") {
		return path, ipaFilenameMetadata{}, nil
	}

	destinationDirectory := filepath.Dir(path)
	if requestedOutputPath != "" {
		info, err := os.Stat(requestedOutputPath)
		if err != nil || !info.IsDir() {
			return path, ipaFilenameMetadata{}, nil
		}
		destinationDirectory = requestedOutputPath
	}

	metadata, err := readIPAFilenameMetadata(path)
	if err != nil {
		return path, ipaFilenameMetadata{}, err
	}
	if metadata.DisplayName == "" {
		metadata.DisplayName = fallbackAppName
	}
	metadata.DisplayName = safeFilenamePart(metadata.DisplayName)
	metadata.Version = safeFilenamePart(metadata.Version)
	metadata.MinimumOSVersion = safeFilenamePart(metadata.MinimumOSVersion)

	if metadata.PlatformName == "" {
		metadata.PlatformName = filenamePlatformName(platform)
	}

	fileName := formatIPAFilename(metadata, accountEmail)
	if fileName == "" {
		return path, metadata, errors.New("IPA Info.plist does not contain a usable app name and version")
	}

	destination := filepath.Join(destinationDirectory, fileName)
	if sameFilePath(path, destination) {
		return path, metadata, nil
	}

	destination, err = unusedDestinationPath(destination, path)
	if err != nil {
		return path, metadata, fmt.Errorf("failed to select a destination filename: %w", err)
	}
	if err := os.Rename(path, destination); err != nil {
		return path, metadata, fmt.Errorf("failed to rename downloaded IPA: %w", err)
	}

	return destination, metadata, nil
}

func readIPAFilenameMetadata(path string) (ipaFilenameMetadata, error) {
	reader, err := zip.OpenReader(path)
	if err != nil {
		return ipaFilenameMetadata{}, fmt.Errorf("failed to open downloaded IPA: %w", err)
	}
	defer reader.Close()

	for _, file := range reader.File {
		if !isMainAppInfoPlistPath(file.Name) {
			continue
		}

		source, err := file.Open()
		if err != nil {
			return ipaFilenameMetadata{}, fmt.Errorf("failed to open IPA Info.plist: %w", err)
		}
		data, readErr := io.ReadAll(source)
		closeErr := source.Close()
		if readErr != nil {
			return ipaFilenameMetadata{}, fmt.Errorf("failed to read IPA Info.plist: %w", readErr)
		}
		if closeErr != nil {
			return ipaFilenameMetadata{}, fmt.Errorf("failed to close IPA Info.plist: %w", closeErr)
		}

		plistData := map[string]interface{}{}
		if _, err := plist.Unmarshal(data, &plistData); err != nil {
			return ipaFilenameMetadata{}, fmt.Errorf("failed to parse IPA Info.plist: %w", err)
		}

		displayName := plistString(plistData["CFBundleDisplayName"])
		if displayName == "" {
			displayName = plistString(plistData["CFBundleName"])
		}

		return ipaFilenameMetadata{
			DisplayName:      displayName,
			Version:          plistString(plistData["CFBundleShortVersionString"]),
			MinimumOSVersion: plistString(plistData["MinimumOSVersion"]),
			PlatformName:     plistPlatformName(plistData["CFBundleSupportedPlatforms"]),
		}, nil
	}

	return ipaFilenameMetadata{}, errors.New("downloaded IPA does not contain Payload/<app>.app/Info.plist")
}

func isMainAppInfoPlistPath(name string) bool {
	parts := strings.Split(name, "/")
	return len(parts) == 3 && parts[0] == "Payload" && strings.HasSuffix(parts[1], ".app") && parts[2] == "Info.plist"
}

func plistString(value interface{}) string {
	if value == nil {
		return ""
	}
	valueString := strings.TrimSpace(fmt.Sprintf("%v", value))
	if valueString == "<nil>" {
		return ""
	}
	return valueString
}

func plistPlatformName(value interface{}) string {
	switch platforms := value.(type) {
	case []string:
		for _, platform := range platforms {
			if name := filenamePlatformName(appstore.Platform(platform)); name != "" {
				return name
			}
		}
	case []interface{}:
		for _, platform := range platforms {
			if name := filenamePlatformName(appstore.Platform(plistString(platform))); name != "" {
				return name
			}
		}
	}

	return ""
}

func filenamePlatformName(platform appstore.Platform) string {
	switch strings.ToLower(string(platform)) {
	case "iphone", "ipad", "iphoneos", "ios":
		return "iOS"
	case "appletv", "appletvos", "tvos":
		return "tvOS"
	case "watchos":
		return "watchOS"
	case "visionos", "xros":
		return "visionOS"
	case "macos", "macosx":
		return "macOS"
	default:
		return ""
	}
}

func formatIPAFilename(metadata ipaFilenameMetadata, accountEmail string) string {
	name := safeFilenamePart(metadata.DisplayName)
	version := safeFilenamePart(metadata.Version)
	if name == "" || version == "" {
		return ""
	}

	parts := []string{name, version}
	if minimumOS := safeFilenamePart(metadata.MinimumOSVersion); minimumOS != "" {
		platform := metadata.PlatformName
		if platform == "" {
			platform = "iOS"
		}
		parts = append(parts, platform+minimumOS)
	}

	if username := accountUsername(accountEmail); username != "" {
		parts = append(parts, username)
	}

	return strings.Join(parts, "_") + ".ipa"
}

func accountUsername(email string) string {
	email = strings.TrimSpace(email)
	if separator := strings.IndexByte(email, '@'); separator >= 0 {
		email = email[:separator]
	}
	return safeFilenamePart(email)
}

func safeFilenamePart(value string) string {
	value = strings.TrimSpace(value)
	var result strings.Builder
	for _, character := range value {
		if unicode.IsControl(character) || strings.ContainsRune(`<>:"/\\|?*`, character) {
			result.WriteByte('_')
			continue
		}
		result.WriteRune(character)
	}
	return strings.Trim(result.String(), " .")
}

func sameFilePath(first, second string) bool {
	firstAbsolute, firstErr := filepath.Abs(first)
	secondAbsolute, secondErr := filepath.Abs(second)
	if firstErr == nil && secondErr == nil {
		firstAbsolute = filepath.Clean(firstAbsolute)
		secondAbsolute = filepath.Clean(secondAbsolute)
		if runtime.GOOS == "windows" {
			return strings.EqualFold(firstAbsolute, secondAbsolute)
		}
		return firstAbsolute == secondAbsolute
	}
	return filepath.Clean(first) == filepath.Clean(second)
}

func unusedDestinationPath(destination, source string) (string, error) {
	if sameFilePath(destination, source) {
		return destination, nil
	}

	extension := filepath.Ext(destination)
	base := strings.TrimSuffix(destination, extension)
	candidate := destination
	for suffix := 2; ; suffix++ {
		if sameFilePath(candidate, source) {
			return candidate, nil
		}
		_, err := os.Stat(candidate)
		if err == nil {
			candidate = fmt.Sprintf("%s_%d%s", base, suffix, extension)
			continue
		}
		if os.IsNotExist(err) {
			return candidate, nil
		}
		return "", err
	}
}

func appendDownloadJobWarning(current, warning string) string {
	if current == "" {
		return warning
	}
	if warning == "" || strings.Contains(current, warning) {
		return current
	}
	return current + "\n" + warning
}
