package service

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestLocalImageStorageRoundTrip(t *testing.T) {
	root := t.TempDir()
	storage, err := NewLocalImageStorage(root)
	require.NoError(t, err)

	data := []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a}
	ref, err := storage.SaveObject(context.Background(), "library/7/2026/07/24/demo.png", "image/png", data)
	require.NoError(t, err)
	require.Equal(t, config.ImageStorageProviderLocal, ref.Provider)
	require.Equal(t, "local", ref.Bucket)
	require.FileExists(t, filepath.Join(root, filepath.FromSlash(ref.ObjectKey)))

	access, err := storage.SignURL(context.Background(), ref, 0)
	require.NoError(t, err)
	require.True(t, IsLocalObjectAccess(access))

	reader, err := storage.Read(context.Background(), ref)
	require.NoError(t, err)
	got := make([]byte, len(data))
	_, err = reader.Read(got)
	require.NoError(t, err)
	require.NoError(t, reader.Close())
	require.Equal(t, data, got)

	require.NoError(t, storage.Delete(context.Background(), ref))
	_, err = os.Stat(filepath.Join(root, filepath.FromSlash(ref.ObjectKey)))
	require.True(t, os.IsNotExist(err))
}

func TestImageDurableStorageWriteUsesDedicatedOSSNotAsync(t *testing.T) {
	asyncCalls := 0
	durableCalls := 0
	asyncStub := &imageLibraryFlowStorage{data: []byte("async")}
	durableStub := &imageLibraryFlowStorage{data: []byte("durable")}

	asyncSettings := NewImageStorageSettingService(nil, nil, nil, func(context.Context, *config.ImageStorageConfig) (ImageStorage, error) {
		asyncCalls++
		return asyncStub, nil
	}, config.ImageStorageConfig{
		Enabled: true, Provider: ImageStorageProviderCustomS3, Bucket: "async-bucket",
		AccessKeyID: "a", SecretAccessKey: "b", Endpoint: "https://s3.example.test", Region: "auto",
	})

	svc := NewImageDurableStorageService(
		config.ImageDurableStorageConfig{
			Backend: config.ImageDurableBackendOSS,
			OSS: config.ImageStorageConfig{
				Enabled: true, Provider: ImageStorageProviderCustomS3, Bucket: "plaza-bucket",
				AccessKeyID: "a", SecretAccessKey: "b", Endpoint: "https://s3.example.test", Region: "auto",
				Prefix: "library/",
			},
		},
		t.TempDir(),
		func(_ context.Context, cfg *config.ImageStorageConfig) (ImageStorage, error) {
			durableCalls++
			require.Equal(t, "plaza-bucket", cfg.Bucket)
			require.NotEqual(t, "async-bucket", cfg.Bucket)
			return durableStub, nil
		},
		asyncSettings,
	)

	write, err := svc.WriteStorage(context.Background())
	require.NoError(t, err)
	_, err = write.SaveObject(context.Background(), "library/1/x.png", "image/png", []byte("png-bytes-here!!!!!!"))
	require.NoError(t, err)
	require.Equal(t, 1, durableCalls)
	require.Equal(t, 0, asyncCalls)
	require.Equal(t, 1, durableStub.saveCalls)
	require.Zero(t, asyncStub.saveCalls)
}

func TestImageDurableStorageDefaultLocalBackend(t *testing.T) {
	root := t.TempDir()
	svc := NewImageDurableStorageService(
		config.ImageDurableStorageConfig{Backend: ""},
		root,
		nil,
		nil,
	)
	write, err := svc.WriteStorage(context.Background())
	require.NoError(t, err)
	local, ok := write.(*LocalImageStorage)
	require.True(t, ok)
	require.Contains(t, local.RootDir(), "image_durable")
}
