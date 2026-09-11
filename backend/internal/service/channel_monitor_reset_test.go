package service

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

type resetChannelMonitorRepoStub struct {
	ChannelMonitorRepository
	id  int64
	err error
}

func (s *resetChannelMonitorRepoStub) ResetData(_ context.Context, id int64) error {
	s.id = id
	return s.err
}

func TestChannelMonitorServiceResetData(t *testing.T) {
	repo := &resetChannelMonitorRepoStub{}
	svc := NewChannelMonitorService(repo, nil)

	require.NoError(t, svc.ResetData(context.Background(), 42))
	require.Equal(t, int64(42), repo.id)
}

func TestChannelMonitorServiceResetDataWrapsRepositoryError(t *testing.T) {
	want := errors.New("reset failed")
	repo := &resetChannelMonitorRepoStub{err: want}
	svc := NewChannelMonitorService(repo, nil)

	err := svc.ResetData(context.Background(), 42)
	require.ErrorIs(t, err, want)
	require.Contains(t, err.Error(), "reset channel monitor data")
}
