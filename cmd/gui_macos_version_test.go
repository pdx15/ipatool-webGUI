package cmd

import "testing"

func TestMacOSSupportsSystemTrust(t *testing.T) {
	tests := map[string]bool{
		"10.15.7": false,
		"11.7.11": false,
		"12.0":    true,
		"12.6.1":  true,
		"13.5":    true,
		"26.1":    true,
		"":        true,
		"unknown": true,
	}

	for version, want := range tests {
		if got := macOSSupportsSystemTrust(version); got != want {
			t.Errorf("macOSSupportsSystemTrust(%q) = %v, want %v", version, got, want)
		}
	}
}
