package cmd

import (
	"testing"
	"time"
)

func TestVersionMetadataEmulatorTotals(t *testing.T) {
	results := map[string]interface{}{
		"1": map[string]interface{}{
			"emulatorOpens":    int64(1),
			"emulatorOpenMs":   int64(3200),
			"emulatorRequests": map[string]int64{"request POST buy.itunes.apple.com/WebObjects/MZBuy.woa/wa/buyProduct": 1},
		},
		"2": map[string]interface{}{
			"emulatorOpens":    int64(2),
			"emulatorOpenMs":   int64(6100),
			"emulatorRequests": map[string]int64{"request POST buy.itunes.apple.com/WebObjects/MZBuy.woa/wa/buyProduct": 2},
		},
		// A cached entry carries no counters.
		"3": map[string]interface{}{"cached": true},
		// A failed lookup is a plain error message.
		"4": map[string]interface{}{"success": false, "message": "not authenticated"},
	}

	opens, openMs, requests := versionMetadataEmulatorTotals(results)
	if opens != 3 {
		t.Errorf("opens = %d, want 3", opens)
	}

	if openMs != 9300 {
		t.Errorf("open ms = %d, want 9300", openMs)
	}

	if got := requests["request POST buy.itunes.apple.com/WebObjects/MZBuy.woa/wa/buyProduct"]; got != 3 {
		t.Errorf("buyProduct requests = %d, want 3", got)
	}
}

func TestFormatVersionMetadataDate(t *testing.T) {
	if got := formatVersionMetadataDate(time.Time{}); got != "" {
		t.Errorf("zero date = %q, want empty", got)
	}

	if got := cleanVersionMetadataDate("0001-01-01"); got != "" {
		t.Errorf("cached zero date = %q, want empty", got)
	}

	if got := cleanVersionMetadataDate("2026-10-20"); got != "2026-10-20" {
		t.Errorf("cached date = %q, want 2026-10-20", got)
	}
}
