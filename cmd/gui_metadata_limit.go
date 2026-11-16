package cmd

import (
	"context"
	"math/rand/v2"
	"os"
	"strconv"
	"strings"
	"time"
)

// Version metadata lookups are signed Store commerce requests, and each of them
// starts its own emulator proof. Apple starts rejecting bursts with HTTP 403 at
// roughly five concurrent requests, so the pool stays just below that: package
// range reads no longer touch the emulator, which makes the slots cheap and
// parallel lookups worth keeping.
const defaultMetadataConcurrency = 4

var metadataSlots = make(chan struct{}, metadataConcurrency())

// metadataConcurrency reports how many Store metadata lookups may run at once.
// The value can be raised for experiments with IPATOOL_GUI_METADATA_CONCURRENCY.
func metadataConcurrency() int {
	raw, ok := os.LookupEnv("IPATOOL_GUI_METADATA_CONCURRENCY")
	if ok {
		if parsed, err := strconv.Atoi(strings.TrimSpace(raw)); err == nil && parsed > 0 {
			return parsed
		}
	}

	return defaultMetadataConcurrency
}

// acquireMetadataSlot waits for a free Store lookup slot. The returned release
// must be called once the lookup finishes; it is nil when ctx already ended.
func acquireMetadataSlot(ctx context.Context) (func(), error) {
	if ctx == nil {
		ctx = context.Background()
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	select {
	case metadataSlots <- struct{}{}:
		return func() { <-metadataSlots }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// metadataRetryDelay spaces out the single retry for a rejected burst so that
// retries cannot merge into another burst.
func metadataRetryDelay() time.Duration {
	return time.Second + rand.N(750*time.Millisecond)
}
