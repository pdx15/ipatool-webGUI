package cmd

import (
	"bytes"
	"compress/gzip"
	"compress/zlib"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
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
	writeTestPackage(t, source, testPkgDistribution, true)

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
	writeTestPackage(t, source, "", false)

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
	writeTestPackage(t, source, testPkgDistribution, true)

	destination, metadata, err := renameDownloadedPackage(source, source, "App for Youtube", "spdx15@gmail.com", appstore.PlatformMacOS)
	if err != nil {
		t.Fatalf("explicit output path should be preserved: %v", err)
	}
	if destination != source || metadata.DisplayName != "" {
		t.Fatalf("explicit output path changed: destination=%q metadata=%+v", destination, metadata)
	}
}

// writeTestPackage writes a minimal xar container: a 28 byte header, a zlib
// compressed table of contents and, when distribution is not empty, a gzip
// compressed Distribution member in the heap.
func writeTestPackage(t *testing.T, path, distribution string, withDistribution bool) {
	t.Helper()

	var heap, tocFile string

	if withDistribution {
		var member bytes.Buffer
		gzipWriter := gzip.NewWriter(&member)
		if _, err := gzipWriter.Write([]byte(distribution)); err != nil {
			t.Fatalf("failed to compress test Distribution: %v", err)
		}
		if err := gzipWriter.Close(); err != nil {
			t.Fatalf("failed to compress test Distribution: %v", err)
		}

		heap = member.String()
		tocFile = fmt.Sprintf(`<file id="1"><data><length>%d</length><offset>0</offset><size>%d</size><encoding style="application/x-gzip"/></data><name>Distribution</name><type>file</type></file>`, len(distribution), member.Len())
	}

	toc := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?><xar><toc><checksum style="sha1"><offset>0</offset><size>20</size></checksum>%s</toc></xar>`, tocFile)

	var compressedTOC bytes.Buffer
	zlibWriter := zlib.NewWriter(&compressedTOC)
	if _, err := zlibWriter.Write([]byte(toc)); err != nil {
		t.Fatalf("failed to compress test table of contents: %v", err)
	}
	if err := zlibWriter.Close(); err != nil {
		t.Fatalf("failed to compress test table of contents: %v", err)
	}

	header := make([]byte, xarHeaderSize)
	copy(header, xarMagic)
	binary.BigEndian.PutUint16(header[4:6], xarHeaderSize)
	binary.BigEndian.PutUint16(header[6:8], 1)
	binary.BigEndian.PutUint64(header[8:16], uint64(compressedTOC.Len()))
	binary.BigEndian.PutUint64(header[16:24], uint64(len(toc)))
	binary.BigEndian.PutUint32(header[24:28], 1)

	contents := append(header, compressedTOC.Bytes()...)
	contents = append(contents, heap...)

	if err := os.WriteFile(path, contents, 0o600); err != nil {
		t.Fatalf("failed to write test package: %v", err)
	}
}
