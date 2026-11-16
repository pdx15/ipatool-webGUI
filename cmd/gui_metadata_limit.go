package cmd

import (
	"context"
	"math/rand/v2"
	"os"
	"strconv"
	"strings"
	"time"
)

// Version metadata lookups are full signed Store commerce requests, and each of
// them starts its own emulator proof. Apple starts rejecting bursts with HTTP
// 403 at roughly five concurrent requests, while parallel lookups gain almost
// nothing (13-17 s per lookup whether one or six run at once). Serialise them
// behind a small slot pool instead of firing the whole page at Apple.
const defaultMetadataConcurrency = 2

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
