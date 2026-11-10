package cmd

import (
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/blacktop/go-macho/pkg/xar"
)

const (
	// pkgDistributionName is the installer script a macOS .pkg keeps at its root.
	// It carries the display name, the bundle version and the minimum OS version,
	// and it is the only such file in the archive: a .pkg has no Info.plist.
	pkgDistributionName = "Distribution"

	xarDistributionMaxSize = 4 << 20
)

type pkgDistributionScript struct {
	XMLName xml.Name `xml:"installer-gui-script"`
	Title   string   `xml:"title"`
	Product struct {
		Version string `xml:"version,attr"`
	} `xml:"product"`
	PkgRefs []struct {
		Version string `xml:"version,attr"`
		Bundles []struct {
			ShortVersion string `xml:"CFBundleShortVersionString,attr"`
		} `xml:"bundle-version>bundle"`
	} `xml:"pkg-ref"`
	Choices []struct {
		VersionString string `xml:"versStr,attr"`
	} `xml:"choice"`
	VolumeCheck struct {
		AllowedOSVersions struct {
			Versions []struct {
				Min string `xml:"min,attr"`
			} `xml:"os-version"`
		} `xml:"allowed-os-versions"`
	} `xml:"volume-check"`
}

// readMacOSPackageFilenameMetadata reads the display name, version and minimum
// OS version of a downloaded macOS .pkg from its Distribution script. The
// container is read with the same xar reader the Store metadata uses, so a
// package Apple accepts is parsed the same way here.
func readMacOSPackageFilenameMetadata(path string) (ipaFilenameMetadata, error) {
	archive, err := xar.Open(path)
	if err != nil {
		return ipaFilenameMetadata{}, fmt.Errorf("failed to open downloaded package: %w", err)
	}
	defer archive.Close()

	for _, file := range archive.Files {
		if file.Name != pkgDistributionName {
			continue
		}

		return readPkgDistribution(file)
	}

	return ipaFilenameMetadata{}, fmt.Errorf("downloaded package does not contain %s", pkgDistributionName)
}

func readPkgDistribution(file *xar.File) (ipaFilenameMetadata, error) {
	source, err := file.Open()
	if err != nil {
		return ipaFilenameMetadata{}, fmt.Errorf("failed to open package %s: %w", pkgDistributionName, err)
	}

	data, readErr := io.ReadAll(io.LimitReader(source, xarDistributionMaxSize))
	closeErr := source.Close()

	if err := errors.Join(readErr, closeErr); err != nil {
		return ipaFilenameMetadata{}, fmt.Errorf("failed to read package %s: %w", pkgDistributionName, err)
	}

	return parsePkgDistribution(data)
}

func parsePkgDistribution(data []byte) (ipaFilenameMetadata, error) {
	var script pkgDistributionScript
	if err := xml.Unmarshal(data, &script); err != nil {
		return ipaFilenameMetadata{}, fmt.Errorf("failed to parse package Distribution script: %w", err)
	}

	metadata := ipaFilenameMetadata{
		DisplayName:      strings.TrimSpace(script.Title),
		Version:          distributionVersion(script),
		MinimumOSVersion: distributionMinimumOSVersion(script),
		PlatformName:     "macOS",
	}

	if metadata.DisplayName == "" && metadata.Version == "" {
		return ipaFilenameMetadata{}, errors.New("package Distribution script contains no app name and version")
	}

	return metadata, nil
}

func distributionVersion(script pkgDistributionScript) string {
	if version := strings.TrimSpace(script.Product.Version); version != "" {
		return version
	}

	for _, ref := range script.PkgRefs {
		for _, bundle := range ref.Bundles {
			if version := strings.TrimSpace(bundle.ShortVersion); version != "" {
				return version
			}
		}
	}

	for _, ref := range script.PkgRefs {
		if version := strings.TrimSpace(ref.Version); version != "" {
			return version
		}
	}

	for _, choice := range script.Choices {
		if version := strings.TrimSpace(choice.VersionString); version != "" {
			return version
		}
	}

	return ""
}

func distributionMinimumOSVersion(script pkgDistributionScript) string {
	for _, version := range script.VolumeCheck.AllowedOSVersions.Versions {
		if minimum := strings.TrimSpace(version.Min); minimum != "" {
			return minimum
		}
	}

	return ""
}
