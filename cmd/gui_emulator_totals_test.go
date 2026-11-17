package main

import "testing"

func TestVersionMetadataEmulatorTotals(t *testing.T) {
	results := map[string]interface{}{
		"1": map[string]interface{}{"emulatorOpens": int64(1), "emulatorOpenMs": int64(3200)},
		"2": map[string]interface{}{"emulatorOpens": int64(2), "emulatorOpenMs": int64(6100)},
		// A cached entry carries no counters.
		"3": map[string]interface{}{"cached": true},
		// A failed lookup is a plain error message.
		"4": map[string]interface{}{"success": false, "message": "not authenticated"},
	}

	opens, openMs := versionMetadataEmulatorTotals(results)
	if opens != 3 {
		t.Errorf("opens = %d, want 3", opens)
	}

	if openMs != 9300 {
		t.Errorf("open ms = %d, want 9300", openMs)
	}
}
