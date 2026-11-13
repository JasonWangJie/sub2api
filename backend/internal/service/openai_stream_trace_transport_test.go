package service

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httptrace"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type streamTraceHTTPUpstream struct {
	HTTPUpstream
	client *http.Client
}

func (u *streamTraceHTTPUpstream) Do(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	return u.client.Do(req)
}

func TestOpenAIStreamTraceTransportKeepsExistingTraceAndDetachedContext(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		require.Equal(t, `{"stream":true}`, string(body))
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: [DONE]\n\n")
	}))
	defer server.Close()
	trace := newOpenAIStreamTrace(time.Now())
	parentWrite := make(chan struct{}, 1)
	ctx := httptrace.WithClientTrace(withOpenAIStreamTrace(context.Background(), trace), &httptrace.ClientTrace{
		WroteRequest: func(httptrace.WroteRequestInfo) { parentWrite <- struct{}{} },
	})
	ctx, release := detachUpstreamContext(ctx)
	defer release()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, server.URL, strings.NewReader(`{"stream":true}`))
	require.NoError(t, err)
	svc := &OpenAIGatewayService{httpUpstream: &streamTraceHTTPUpstream{client: server.Client()}}
	resp, err := svc.doOpenAIUpstream(req, "", &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeAPIKey})
	require.NoError(t, err)
	defer resp.Body.Close()
	_, err = io.Copy(io.Discard, resp.Body)
	require.NoError(t, err)
	select {
	case <-parentWrite:
	default:
		t.Fatal("existing httptrace callback was lost")
	}
	snapshot := trace.Snapshot()
	require.Equal(t, int64(1), snapshot.TransportCount)
	require.NotNil(t, snapshot.RequestWriteMs)
	require.NotNil(t, snapshot.ResponseHeadersMs)
	require.GreaterOrEqual(t, *snapshot.ResponseHeadersMs, *snapshot.RequestWriteMs)
	require.Equal(t, 200, snapshot.HTTPStatus)
	require.Nil(t, snapshot.FirstDataMs, "headers alone must not imply received SSE data")
}

func TestOpenAIStreamTraceDisabledAndSampling(t *testing.T) {
	ctx := context.Background()
	account := &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeAPIKey}
	svc := &OpenAIGatewayService{cfg: &config.Config{}}
	unchanged, finish := svc.beginOpenAIStreamTrace(ctx, nil, account, []byte(`{"stream":true}`), time.Now())
	finish()
	require.Equal(t, ctx, unchanged)
	svc.cfg.Gateway.OpenAIStreamTraceEnabled = true
	svc.cfg.Gateway.OpenAIStreamTraceSampleEvery = 20
	openAIStreamTraceSequence.Store(0)
	traced := 0
	for i := 0; i < 40; i++ {
		sampled, finish := svc.beginOpenAIStreamTrace(ctx, nil, account, []byte(`{"stream":true}`), time.Now())
		if openAIStreamTraceFromContext(sampled) != nil {
			traced++
		}
		finish()
	}
	require.Equal(t, 2, traced)
	svc.cfg.Gateway.OpenAIStreamTraceSampleEvery = 1
	nonstream, finish := svc.beginOpenAIStreamTrace(ctx, nil, account, []byte(`{"stream":false}`), time.Now())
	finish()
	require.Nil(t, openAIStreamTraceFromContext(nonstream))
}
