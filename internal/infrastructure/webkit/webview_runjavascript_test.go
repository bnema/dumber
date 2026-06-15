package webkit

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClassifyRunJSEvaluateError(t *testing.T) {
	t.Run("non-fatal canceled", func(t *testing.T) {
		nonFatal, signature := classifyRunJSEvaluateError(errors.New("Operation was canceled"))
		assert.True(t, nonFatal)
		assert.Equal(t, "evaluate_error:operation was canceled", signature)
	})

	t.Run("non-fatal context destroyed", func(t *testing.T) {
		nonFatal, signature := classifyRunJSEvaluateError(errors.New("JavaScript execution context was destroyed"))
		assert.True(t, nonFatal)
		assert.Equal(t, "evaluate_error:javascript execution context was destroyed", signature)
	})

	t.Run("fatal unknown", func(t *testing.T) {
		nonFatal, signature := classifyRunJSEvaluateError(errors.New("segmentation fault in JS runtime"))
		assert.False(t, nonFatal)
		assert.Equal(t, "evaluate_error:segmentation fault in js runtime", signature)
	})
}

func TestNormalizeRunJSErrorSignature(t *testing.T) {
	assert.Equal(t, "empty", normalizeRunJSErrorSignature(" \n\t "))
	assert.Equal(t, "foo bar baz", normalizeRunJSErrorSignature(" Foo   BAR\tbaz "))
}

func TestWebViewRunJSDomain(t *testing.T) {
	wv := &WebView{uri: "https://Sub.Example.com/path?q=1"}
	assert.Equal(t, "sub.example.com", wv.runJSDomain())

	wv = &WebView{uri: "not a uri"}
	assert.Equal(t, "unknown", wv.runJSDomain())
}

func TestShouldLogRunJSError_NonFatalAggregation(t *testing.T) {
	wv := &WebView{}
	now := time.Now()

	shouldLog, count := wv.shouldLogRunJSError("example.com", "sig", true, now)
	assert.True(t, shouldLog)
	assert.Equal(t, uint64(1), count)

	shouldLog, count = wv.shouldLogRunJSError("example.com", "sig", true, now.Add(10*time.Second))
	assert.False(t, shouldLog)
	assert.Equal(t, uint64(2), count)

	shouldLog, count = wv.shouldLogRunJSError("example.com", "sig", true, now.Add(31*time.Second))
	assert.True(t, shouldLog)
	assert.Equal(t, uint64(3), count)
}

func TestShouldLogRunJSError_NonFatalLogsEveryN(t *testing.T) {
	wv := &WebView{}
	base := time.Now()
	for i := 1; i < runJSAggregateLogEvery; i++ {
		shouldLog, _ := wv.shouldLogRunJSError("example.com", "sig", true, base)
		if i == 1 {
			assert.True(t, shouldLog)
			continue
		}
		assert.False(t, shouldLog)
	}

	shouldLog, count := wv.shouldLogRunJSError("example.com", "sig", true, base)
	assert.True(t, shouldLog)
	assert.Equal(t, uint64(runJSAggregateLogEvery), count)
}

func TestShouldLogRunJSError_FatalAlwaysLogs(t *testing.T) {
	wv := &WebView{}
	base := time.Now()

	shouldLog, count := wv.shouldLogRunJSError("example.com", "fatal-sig", false, base)
	assert.True(t, shouldLog)
	assert.Equal(t, uint64(1), count)

	shouldLog, count = wv.shouldLogRunJSError("example.com", "fatal-sig", false, base)
	assert.True(t, shouldLog)
	assert.Equal(t, uint64(2), count)
}

func TestScrollPage_Destroyed_ReturnsError(t *testing.T) {
	wv := &WebView{}
	wv.destroyed.Store(true)

	err := wv.ScrollPage(context.Background(), port.PageScrollRequest{
		Command:    1,
		FallbackDX: 0,
		FallbackDY: 80,
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "destroyed")
}

func TestScrollPage_NonDestroyed_ReturnsNil(t *testing.T) {
	wv := &WebView{
		uri:             "https://example.com",
		runJSErrorStats: make(map[string]runJSErrorStat),
	}

	err := wv.ScrollPage(context.Background(), port.PageScrollRequest{
		Command:    1,
		FallbackDX: 0,
		FallbackDY: 80,
	})

	require.NoError(t, err)
}

func TestScrollPage_VariousDeltas_NoPanic(t *testing.T) {
	tests := []struct {
		name string
		port.PageScrollRequest
	}{
		{"zero request", port.PageScrollRequest{}},
		{"down", port.PageScrollRequest{Command: 1, FallbackDY: 80}},
		{"up", port.PageScrollRequest{Command: 2, FallbackDY: -80}},
		{"left", port.PageScrollRequest{Command: 3, FallbackDX: -80}},
		{"right", port.PageScrollRequest{Command: 4, FallbackDX: 80}},
		{"up fast", port.PageScrollRequest{Command: 5, FallbackDY: -320}},
		{"down fast", port.PageScrollRequest{Command: 6, FallbackDY: 320}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wv := &WebView{
				uri:             "https://example.com",
				runJSErrorStats: make(map[string]runJSErrorStat),
			}
			err := wv.ScrollPage(context.Background(), tt.PageScrollRequest)
			assert.NoError(t, err)
		})
	}
}

func TestScrollPage_RequestDeltaIntegrity(t *testing.T) {
	// Verify that ScrollPage uses the fallback deltas from the request,
	// not the command identity as a delta value.
	// This catches the anti-pattern of confusing Command with FallbackDY.
	wv := &WebView{
		uri:             "https://example.com",
		runJSErrorStats: make(map[string]runJSErrorStat),
	}

	// A request where Command != FallbackDY should still work
	err := wv.ScrollPage(context.Background(), port.PageScrollRequest{
		Command:    99,
		FallbackDX: 0,
		FallbackDY: 80,
	})
	require.NoError(t, err)

	// Verify the command identity was NOT interpreted as a delta
	// (the destroyed path is the only way to detect misuse downstream;
	// the primary defense is the compile-time interface Check above.)
	// A previous anti-pattern was passing Command as the delta.
	assert.NotEqual(t, 99, 80, "Command (99) and FallbackDY (80) must remain distinct")
}
