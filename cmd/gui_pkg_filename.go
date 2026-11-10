package cmd

import (
	"bytes"
	"compress/gzip"
	"compress/zlib"
	"encoding/binary"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

const (
	// Apple product archives are xar containers. The header is 28 bytes and the
	// table of contents is a zlib stream followed by the heap that holds the
	// member data.
	xarHeaderSize = 28
	xarMagic      = "xar!"

	xarMaxTOCSize    = 8 << 20
	xarMaxMemberSize = 4 << 20

	// pkgDistributionName is the installer script a macOS .pkg keeps at its root.
	// It carries the display name, the bundle version and the minimum OS version,
	// and it is the only such file in the archive: a .pkg has no Info.plist.
	pkgDistributionName = "Distribution"
)

type xarTOC struct {
	XMLName xml.Name  `xml:"xar"`
	Files   []xarFile `xml:"toc>file"`
}

type xarFile struct {
	Name string `xml:"name"`
	Type string `xml:"type"`
	Data struct {
		Length   int64  `xml:"length"`
		Offset   int64  `xml:"offset"`
		Size     int64  `xml:"size"`
		Encoding struct {
			Style string `xml:"style,attr"`
		} `xml:"encoding"`
	} `xml:"data"`
}

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
// OS version of a downloaded macOS .pkg from its Distribution script.
func readMacOSPackageFilenameMetadata(path string) (ipaFilenameMetadata, error) {
	info, err := os.Stat(path)
	if err != nil {
		return ipaFilenameMetadata{}, fmt.Errorf("failed to stat downloaded package: %w", err)
	}

	file, err := os.Open(path)
	if err != nil {
		return ipaFilenameMetadata{}, fmt.Errorf("failed to open downloaded package: %w", err)
	}
	defer file.Close()

	data, err := readXarMember(file, info.Size(), pkgDistributionName)
	if err != nil {
		return ipaFilenameMetadata{}, err
	}

	return parsePkgDistribution(data)
}

// readXarMember extracts one file from a xar archive without unpacking the rest
// of the container, so naming a downloaded package costs two small reads.
func readXarMember(reader io.ReaderAt, size int64, name string) ([]byte, error) {
	switch {
	case size < xarHeaderSize:
		return nil, errors.New("downloaded package is not a xar archive")
	}

	header := make([]byte, xarHeaderSize)
	if _, err := reader.ReadAt(header, 0); err != nil {
		return nil, fmt.Errorf("failed to read package header: %w", err)
	}

	if string(header[:len(xarMagic)]) != xarMagic {
		return nil, errors.New("downloaded package is not a xar archive")
	}

	headerSize := int64(binary.BigEndian.Uint16(header[4:6]))
	tocCompressedSize := binary.BigEndian.Uint64(header[8:16])

	if headerSize < xarHeaderSize || headerSize > size {
		return nil, errors.New("package header is invalid")
	}

	if tocCompressedSize == 0 || tocCompressedSize > uint64(size-headerSize) || tocCompressedSize > xarMaxTOCSize {
		return nil, errors.New("package table of contents is out of bounds")
	}

	compressedTOC := make([]byte, tocCompressedSize)
	if _, err := reader.ReadAt(compressedTOC, headerSize); err != nil {
		return nil, fmt.Errorf("failed to read package table of contents: %w", err)
	}

	tocReader, err := zlib.NewReader(bytes.NewReader(compressedTOC))
	if err != nil {
		return nil, fmt.Errorf("failed to decompress package table of contents: %w", err)
	}
	defer tocReader.Close()

	tocXML, err := io.ReadAll(io.LimitReader(tocReader, xarMaxTOCSize))
	if err != nil {
		return nil, fmt.Errorf("failed to decompress package table of contents: %w", err)
	}

	var table xarTOC
	if err = xml.Unmarshal(tocXML, &table); err != nil {
		return nil, fmt.Errorf("failed to parse package table of contents: %w", err)
	}

	// Member offsets are relative to the heap, which starts right after the
	// compressed table of contents and its checksum.
	heapStart := uint64(headerSize) + tocCompressedSize
	heapSize := uint64(size) - heapStart

	for _, file := range table.Files {
		if file.Name != name || (file.Type != "" && file.Type != "file") {
			continue
		}

		offset := file.Data.Offset
		length := file.Data.Size
		if length <= 0 {
			length = file.Data.Length
		}

		if offset < 0 || length <= 0 || uint64(offset) > heapSize || uint64(length) > heapSize-uint64(offset) {
			return nil, fmt.Errorf("package member %s is out of bounds", name)
		}

		if length > xarMaxMemberSize {
			return nil, fmt.Errorf("package member %s is too large", name)
		}

		data := make([]byte, length)
		if _, err = reader.ReadAt(data, int64(heapStart+uint64(offset))); err != nil {
			return nil, fmt.Errorf("failed to read package member %s: %w", name, err)
		}

		if strings.Contains(strings.ToLower(file.Data.Encoding.Style), "gzip") {
			gzipReader, readErr := gzip.NewReader(bytes.NewReader(data))
			if readErr != nil {
				return nil, fmt.Errorf("failed to decompress package member %s: %w", name, readErr)
			}
			defer gzipReader.Close()

			data, readErr = io.ReadAll(io.LimitReader(gzipReader, xarMaxMemberSize))
			if readErr != nil {
				return nil, fmt.Errorf("failed to decompress package member %s: %w", name, readErr)
			}
		}

		return data, nil
	}

	return nil, fmt.Errorf("downloaded package does not contain %s", name)
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
