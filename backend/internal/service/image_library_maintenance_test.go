package service

import (
	"context"
	"errors"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestResolveLegacyImagePathRejectsTraversalAndAbsolutePaths(t *testing.T) {
	root := t.TempDir()
	for _, candidate := range []string{"../outside.png", "images/../../outside.png", filepath.Join(root, "absolute.png"), "", "."} {
		_, err := resolveLegacyImagePath(root, candidate)
		require.Error(t, err, candidate)
		require.True(t, errors.Is(err, errLegacyImageQuarantined), candidate)
	}
}

func TestImageLibraryLeaseCancelsWorkWhenOwnershipIsLost(t *testing.T) {
	var heartbeats atomic.Int32
	lease := startImageLibraryLeaseWithInterval(context.Background(), time.Millisecond, func(context.Context) (bool, error) {
		heartbeats.Add(1)
		return false, nil
	})

	select {
	case <-lease.Context().Done():
	case <-time.After(time.Second):
		t.Fatal("lease context was not canceled")
	}
	require.ErrorIs(t, lease.Stop(), ErrImageLibraryLeaseLost)
	require.Positive(t, heartbeats.Load())
}

func TestImageLibraryLeaseStopBeforeHeartbeatIsClean(t *testing.T) {
	lease := startImageLibraryLeaseWithInterval(context.Background(), time.Hour, func(context.Context) (bool, error) {
		return true, nil
	})
	require.NoError(t, lease.Stop())
}

func TestImageLibraryMaintenanceStopWaitsForLoopExitAndIsIdempotent(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	service := &ImageLibraryMaintenanceService{stop: cancel, done: done}
	go func() {
		<-ctx.Done()
		close(done)
	}()

	require.NotPanics(t, service.Stop)
	require.True(t, service.stopped)
	require.NotPanics(t, service.Stop)
}

func TestResolveLegacyImagePathAllowsContainedObject(t *testing.T) {
	root := t.TempDir()
	got, err := resolveLegacyImagePath(root, "image_plaza/42/result.png")
	require.NoError(t, err)
	require.Equal(t, filepath.Join(root, "image_plaza", "42", "result.png"), got)
}
