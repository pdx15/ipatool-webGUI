package cmd

import (
	"bytes"
	"compress/zlib"
	"crypto/sha1"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/majd/ipatool/v2/pkg/appstore"
)

const testPkgDistribution = `<?xml version="1.0" encoding="utf-8" standalone="yes"?>
<installer-gui-script minSpecVersion="2">
    <pkg-ref id="toolbox.yt.videoplayer">
        <bundle-version>
            <bundle CFBundleShortVersionString="3.0" CFBundleVersion="3.0.0" id="toolbox.yt.videoplayer" path="App for Youtube.app"/>
        </bundle-version>
    </pkg-ref>
    <product id="toolbox.yt.videoplayer" version="3.0"/>
    <title>App for Youtube</title>
    <options customize="never" require-scripts="false" hostArchitectures="arm64,x86_64"/>
    <volume-check>
        <allowed-os-versions>
            <os-version min="14.0"/>
        </allowed-os-versions>
    </volume-check>
    <choices-outline>
        <line choice="default">
            <line choice="toolbox.yt.videoplayer"/>
        </line>
    </choices-outline>
    <choice id="default" title="App for Youtube" versStr="3.0"/>
    <choice id="toolbox.yt.videoplayer" title="App for Youtube" visible="false" customLocation="/Applications">
        <pkg-ref id="toolbox.yt.videoplayer"/>
    </choice>
    <pkg-ref id="toolbox.yt.videoplayer" version="3.0.0" onConclusion="none" installKBytes="37248" updateKBytes="0">#toolbox.yt.videoplayer.pkg</pkg-ref>
</installer-gui-script>`

func TestRenameDownloadedPackageUsesDistributionScript(t *testing.T) {
	directory := t.TempDir()
	source := filepath.Join(directory, "6753067801_3.0.pkg")
	writeTestPackage(t, source, map[string]string{pkgDistributionName: testPkgDistribution})

	destination, metadata, err := renameDownloadedPackage(
		source,
		"",
		"App Store fallback",
		"spdx15@gmail.com",
		appstore.PlatformMacOS,
	)
	if err != nil {
		t.Fatalf("failed to rename downloaded package: %v", err)
	}

	if got, want := filepath.Base(destination), "App for Youtube_3.0_macOS14.0_spdx15.pkg"; got != want {
		t.Fatalf("filename = %q, want %q", got, want)
	}
	if metadata.Version != "3.0" || metadata.MinimumOSVersion != "14.0" || metadata.PlatformName != "macOS" {
		t.Fatalf("unexpected Distribution metadata: %+v", metadata)
	}
	if _, err := os.Stat(destination); err != nil {
		t.Fatalf("renamed package does not exist: %v", err)
	}
}

func TestRenameDownloadedPackageKeepsNameWithoutDistribution(t *testing.T) {
	directory := t.TempDir()
	source := filepath.Join(directory, "6753067801_3.0.pkg")
	writeTestPackage(t, source, map[string]string{"PackageInfo": "<pkg-info/>"})

	destination, _, err := renameDownloadedPackage(source, "", "App for Youtube", "spdx15@gmail.com", appstore.PlatformMacOS)
	if err == nil {
		t.Fatal("expected an error for a package without a Distribution script")
	}
	if destination != source {
		t.Fatalf("destination = %q, want the original path %q", destination, source)
	}
}

func TestRenameDownloadedPackagePreservesExplicitPackageFilename(t *testing.T) {
	directory := t.TempDir()
	source := filepath.Join(directory, "custom.pkg")
	writeTestPackage(t, source, map[string]string{pkgDistributionName: testPkgDistribution})

	destination, metadata, err := renameDownloadedPackage(source, source, "App for Youtube", "spdx15@gmail.com", appstore.PlatformMacOS)
	if err != nil {
		t.Fatalf("explicit output path should be preserved: %v", err)
	}
	if destination != source || metadata.DisplayName != "" {
		t.Fatalf("explicit output path changed: destination=%q metadata=%+v", destination, metadata)
	}
}

func TestParsePkgDistributionReadsTitleVersionAndMinimumOS(t *testing.T) {
	metadata, err := parsePkgDistribution([]byte(testPkgDistribution))
	if err != nil {
		t.Fatalf("failed to parse Distribution script: %v", err)
	}

	if metadata.DisplayName != "App for Youtube" || metadata.Version != "3.0" || metadata.MinimumOSVersion != "14.0" {
		t.Fatalf("unexpected metadata: %+v", metadata)
	}
}

// writeTestPackage writes a xar container the Store reader accepts: a 28 byte
// header, a zlib compressed table of contents with its SHA-1 in the heap and one
// zlib compressed member per entry.
func writeTestPackage(t *testing.T, path string, members map[string]string) {
	t.Helper()

	names := make([]string, 0, len(members))
	for name := range members {
		names = append(names, name)
	}
	sort.Strings(names)

	type storedMember struct {
		name   string
		plain  []byte
		stored []byte
	}

	items := make([]storedMember, 0, len(names))
	for _, name := range names {
		var buffer bytes.Buffer
		writer := zlib.NewWriter(&buffer)
		if _, err := writer.Write([]byte(members[name])); err != nil {
			t.Fatalf("failed to compress test member: %v", err)
		}
		if err := writer.Close(); err != nil {
			t.Fatalf("failed to compress test member: %v", err)
		}

		items = append(items, storedMember{name: name, plain: []byte(members[name]), stored: buffer.Bytes()})
	}

	// The heap holds the table-of-contents checksum first, so member offsets
	// start after it.
	offset := sha1.Size
	var files strings.Builder

	for index, item := range items {
		archived := sha1.Sum(item.stored)
		extracted := sha1.Sum(item.plain)

		fmt.Fprintf(&files,
			`<file id="%d"><data><length>%d</length><offset>%d</offset><size>%d</size>`+
				`<encoding style="application/x-gzip"/>`+
				`<archived-checksum style="sha1">%x</archived-checksum>`+
				`<extracted-checksum style="sha1">%x</extracted-checksum></data>`+
				`<name>%s</name><type>file</type></file>`,
			index+1, len(item.stored), offset, len(item.plain), archived, extracted, item.name)

		offset += len(item.stored)
	}

	toc := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?><xar><toc>`+
		`<checksum style="sha1"><offset>0</offset><size>%d</size></checksum>%s</toc></xar>`,
		sha1.Size, files.String())

	var compressedTOC bytes.Buffer
	tocWriter := zlib.NewWriter(&compressedTOC)
	if _, err := tocWriter.Write([]byte(toc)); err != nil {
		t.Fatalf("failed to compress test table of contents: %v", err)
	}
	if err := tocWriter.Close(); err != nil {
		t.Fatalf("failed to compress test table of contents: %v", err)
	}

	header := make([]byte, 28)
	copy(header, "xar!")
	binary.BigEndian.PutUint16(header[4:6], 28)
	binary.BigEndian.PutUint16(header[6:8], 1)
	binary.BigEndian.PutUint64(header[8:16], uint64(compressedTOC.Len()))
	binary.BigEndian.PutUint64(header[16:24], uint64(len(toc)))
	binary.BigEndian.PutUint32(header[24:28], 1)

	contents := append([]byte{}, header...)
	contents = append(contents, compressedTOC.Bytes()...)

	tocChecksum := sha1.Sum(compressedTOC.Bytes())
	contents = append(contents, tocChecksum[:]...)

	for _, item := range items {
		contents = append(contents, item.stored...)
	}

	if err := os.WriteFile(path, contents, 0o600); err != nil {
		t.Fatalf("failed to write test package: %v", err)
	}
}
