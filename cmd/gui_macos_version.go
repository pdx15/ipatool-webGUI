package cmd

import (
	"fmt"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
)

// macOSSystemTrustVersion is the first macOS release whose Security framework
// exports SecTrustCopyCertificateChain. The Go runtime verifies TLS through
// that function on darwin unconditionally, so an older system aborts on the
// first HTTPS request with "Symbol not found" and a goroutine dump. Report the
// requirement up front instead.
const macOSSystemTrustVersion = 12

// ensureSupportedMacOS refuses to start the GUI on a macOS release that cannot
// verify TLS with the Security framework the Go runtime expects.
func ensureSupportedMacOS() error {
	if runtime.GOOS != "darwin" {
		return nil
	}

	output, err := exec.Command("sw_vers", "-productVersion").Output()
	if err != nil {
		return nil
	}

	version := strings.TrimSpace(string(output))
	if version == "" || macOSSupportsSystemTrust(version) {
		return nil
	}

	return fmt.Errorf(
		"ipatool WebGUI requires macOS %d (Monterey) or newer: this build verifies TLS with SecTrustCopyCertificateChain, which macOS %s does not provide",
		macOSSystemTrustVersion, version,
	)
}

// macOSSupportsSystemTrust reports whether a macOS product version such as
// "10.15.7", "12.6" or "26.1" provides SecTrustCopyCertificateChain. An
// unreadable version is treated as supported so the check never blocks a
// system it cannot describe.
func macOSSupportsSystemTrust(version string) bool {
	major, ok := macOSMajorVersion(version)
	if !ok {
		return true
	}

	return major >= macOSSystemTrustVersion
}

func macOSMajorVersion(version string) (int, bool) {
	head := strings.TrimSpace(version)
	if index := strings.IndexByte(head, '.'); index >= 0 {
		head = head[:index]
	}

	major, err := strconv.Atoi(head)
	if err != nil || major <= 0 {
		return 0, false
	}

	return major, true
}
