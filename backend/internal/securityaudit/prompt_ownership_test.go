package securityaudit

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type pausedActiveConfigStore struct {
	ConfigStore
	entered chan struct{}
	resume  <-chan struct{}
}

func (s *pausedActiveConfigStore) Active() (ActiveConfig, bool) {
	close(s.entered)
	<-s.resume
	return s.ConfigStore.Active()
}

func TestPromptServiceEnqueueOwnsRequestAfterReturn(t *testing.T) {
	for _, viaCoordinator := range []bool{false, true} {
		t.Run(fmt.Sprintf("coordinator=%t", viaCoordinator), func(t *testing.T) {
			config := &fakeConfigStore{cfg: asyncConfig(), active: true}
			entered, resume := make(chan struct{}), make(chan struct{})
			workerConfig := &pausedActiveConfigStore{ConfigStore: config, entered: entered, resume: resume}
			repo, payload := &fakeJobRepository{}, &fakePayloadStore{}
			svc := &PromptService{
				config: config, background: context.Background(), enqueueSlots: make(chan struct{}, 1),
				enqueuer: NewEnqueuer(workerConfig, repo, payload),
			}
			var resumeOnce sync.Once
			resumeWorker := func() { resumeOnce.Do(func() { close(resume) }) }
			t.Cleanup(func() {
				resumeWorker()
				svc.enqueueWG.Wait()
			})

			groupID := int64(42)
			req := asyncRequest()
			req.GroupID = &groupID
			if viaCoordinator {
				decision := NewCoordinator(nil, svc).Check(context.Background(), req)
				require.True(t, decision.AllowNextStage)
			} else {
				require.NoError(t, svc.Enqueue(context.Background(), req))
			}
			select {
			case <-entered:
			case <-time.After(5 * time.Second):
				t.Fatal("background enqueue did not start")
			}

			// The handler may reuse its memory as soon as Enqueue returns. Keep the
			// worker paused before parsing so a missing ownership copy is observable.
			for i := range req.Body {
				req.Body[i] = 'x'
			}
			groupID = 99
			resumeWorker()
			svc.enqueueWG.Wait()

			require.NotNil(t, repo.createdSnapshot.GroupID)
			require.Equal(t, int64(42), *repo.createdSnapshot.GroupID)
			require.Equal(t, "payload canary text", payload.values[1])
			require.Empty(t, svc.enqueueSlots)
		})
	}
}

type requestRecordingLegacyEngine struct {
	body    string
	groupID int64
}

func (e *requestRecordingLegacyEngine) Check(_ context.Context, req Request) (*LegacyDecision, error) {
	e.body = string(req.Body)
	e.groupID = *req.GroupID
	return &LegacyDecision{Allowed: true}, nil
}

type mutatingBlockingPromptEngine struct{ fakePromptEngine }

func (e *mutatingBlockingPromptEngine) Evaluate(_ context.Context, req Request) (*PromptDecision, error) {
	req.Body[0] = 'x'
	*req.GroupID = 99
	return &PromptDecision{Kind: DecisionAllow, AllowNextStage: true}, nil
}

func TestCoordinatorBlockingKeepsPromptRequestIsolated(t *testing.T) {
	groupID := int64(42)
	req := asyncRequest()
	req.GroupID = &groupID
	originalBody := string(req.Body)
	legacy := &requestRecordingLegacyEngine{}
	prompt := &mutatingBlockingPromptEngine{fakePromptEngine{mode: ModeBlocking}}

	decision := NewCoordinator(legacy, prompt).Check(context.Background(), req)

	require.True(t, decision.AllowNextStage)
	require.Equal(t, originalBody, string(req.Body))
	require.Equal(t, int64(42), groupID)
	require.Equal(t, originalBody, legacy.body)
	require.Equal(t, int64(42), legacy.groupID)
}

// This engine models the ownership copy at the async boundary without database,
// parsing, or goroutine overhead, so the benchmark measures request copying only.
type retainingAsyncPromptEngine struct {
	fakePromptEngine
	retained Request
}

func (e *retainingAsyncPromptEngine) Enqueue(_ context.Context, req Request) error {
	e.retained = req.Clone()
	return nil
}

func BenchmarkCoordinatorAsyncRequestCopy(b *testing.B) {
	for _, size := range []int{1 << 20, 8 << 20, 16 << 20} {
		b.Run(fmt.Sprintf("%dMiB", size>>20), func(b *testing.B) {
			req := Request{Body: make([]byte, size)}
			prompt := &retainingAsyncPromptEngine{fakePromptEngine: fakePromptEngine{mode: ModeAsync}}
			coordinator := NewCoordinator(nil, prompt)
			ctx := context.Background()
			b.ReportAllocs()
			b.SetBytes(int64(size))
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				coordinator.Check(ctx, req)
			}
		})
	}
}
