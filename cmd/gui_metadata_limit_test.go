package cmd

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestMetadataSlotsCapConcurrency verifies that Store metadata lookups never
// exceed the slot pool: bursts of five or more concurrent requests are what
// Apple rejects with HTTP 403.
func TestMetadataSlotsCapConcurrency(t *testing.T) {
	limit := cap(metadataSlots)
	if limit <= 0 {
		t.Fatalf("metadataSlots capacity = %d, want > 0", limit)
	}

	const workers = 12
	var (
		inFlight    atomic.Int64
		maxInFlight atomic.Int64
		wg          sync.WaitGroup
	)

	for i := 0; i < workers; i++ {
		wg.Add(1)

		go func() {
			defer wg.Done()

			release, err := acquireMetadataSlot(context.Background())
			if err != nil {
				t.Errorf("acquireMetadataSlot() error = %v", err)
				return
			}
			defer release()

			current := inFlight.Add(1)
			for {
				observed := maxInFlight.Load()
				if current <= observed || maxInFlight.CompareAndSwap(observed, current) {
					break
				}
			}

			time.Sleep(10 * time.Millisecond)
			inFlight.Add(-1)
		}()
	}

	wg.Wait()

	if got := int(maxInFlight.Load()); got > limit {
		t.Fatalf("concurrent lookups peaked at %d, want at most %d", got, limit)
	}
	if got := int(maxInFlight.Load()); got < 2 && limit > 1 {
		t.Fatalf("concurrent lookups peaked at %d, want the pool to be used in parallel", got)
	}
	if got := len(metadataSlots); got != 0 {
		t.Fatalf("metadataSlots holds %d slots after every release, want 0", got)
	}
}

// TestAcquireMetadataSlotHonoursCancellation verifies that a cancelled request
// does not stay queued behind other lookups.
func TestAcquireMetadataSlotHonoursCancellation(t *testing.T) {
	limit := cap(metadataSlots)

	releases := make([]func(), 0, limit)
	for i := 0; i < limit; i++ {
		release, err := acquireMetadataSlot(context.Background())
		if err != nil {
			t.Fatalf("acquireMetadataSlot() error = %v", err)
		}
		releases = append(releases, release)
	}

	defer func() {
		for _, release := range releases {
			release()
		}
	}()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	started := time.Now()
	release, err := acquireMetadataSlot(ctx)
	if release != nil {
		release()
		t.Fatal("acquireMetadataSlot() returned a release for a cancelled context")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("acquireMetadataSlot() error = %v, want context.Canceled", err)
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("acquireMetadataSlot() blocked for %s on a cancelled context", elapsed)
	}
}

// TestMetadataConcurrencyFromEnv documents the tuning knob and its fallback.
func TestMetadataConcurrencyFromEnv(t *testing.T) {
	t.Setenv("IPATOOL_GUI_METADATA_CONCURRENCY", "3")
	if got := metadataConcurrency(); got != 3 {
		t.Fatalf("metadataConcurrency() = %d, want 3", got)
	}

	for _, value := range []string{"", "0", "-1", "two"} {
		t.Setenv("IPATOOL_GUI_METADATA_CONCURRENCY", value)
		if got := metadataConcurrency(); got != defaultMetadataConcurrency {
			t.Fatalf("metadataConcurrency() with %q = %d, want %d", value, got, defaultMetadataConcurrency)
		}
	}
}

// TestIsTransientAppleError covers the burst failures the Version History page
// runs into: 403 rejections under load and gateway errors.
func TestIsTransientAppleError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "nil", err: nil, want: false},
		{name: "403 burst", err: errors.New("send Store commerce request: unexpected response from Apple (HTTP 403): empty or non-plist body"), want: true},
		{name: "502", err: errors.New("unexpected response from Apple (HTTP 502)"), want: true},
		{name: "503", err: errors.New("unexpected response from Apple (HTTP 503)"), want: true},
		{name: "504", err: errors.New("unexpected response from Apple (HTTP 504)"), want: true},
		{name: "401", err: errors.New("unexpected response from Apple (HTTP 401)"), want: false},
		{name: "cancelled", err: context.Canceled, want: false},
	}

	for _, test := range tests {
		if got := isTransientAppleError(test.err); got != test.want {
			t.Errorf("%s: isTransientAppleError() = %v, want %v", test.name, got, test.want)
		}
	}
}

// TestMetadataRetryDelayIsSpaced keeps retries from merging into another burst.
func TestMetadataRetryDelayIsSpaced(t *testing.T) {
	for i := 0; i < 32; i++ {
		delay := metadataRetryDelay()
		if delay < time.Second || delay > 2*time.Second {
			t.Fatalf("metadataRetryDelay() = %s, want between 1s and 2s", delay)
		}
	}
}
