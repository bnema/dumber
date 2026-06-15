package webkit

import (
	"context"
	"errors"
	"fmt"
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
		Command:    port.PageScrollCommandDown,
		FallbackDX: 0,
		FallbackDY: 80,
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "destroyed")
}

func TestScrollPage_UsesFallbackDeltasAndRunsJavaScript(t *testing.T) {
	oldBuilder := buildPageScrollFallbackJS
	oldRunner := runPageScrollFallbackJS
	defer func() {
		buildPageScrollFallbackJS = oldBuilder
		runPageScrollFallbackJS = oldRunner
	}()

	var gotDX, gotDY int
	var gotScript string
	var runCount int
	buildPageScrollFallbackJS = func(dx, dy int) string {
		gotDX, gotDY = dx, dy
		return fmt.Sprintf("scroll(%d,%d)", dx, dy)
	}
	runPageScrollFallbackJS = func(_ *WebView, _ context.Context, script string) {
		runCount++
		gotScript = script
	}

	wv := &WebView{
		uri:             "https://example.com",
		runJSErrorStats: make(map[string]runJSErrorStat),
	}

	err := wv.ScrollPage(context.Background(), port.PageScrollRequest{
		Command:    port.PageScrollCommand(99),
		FallbackDX: -12,
		FallbackDY: 80,
	})

	require.NoError(t, err)
	assert.Equal(t, -12, gotDX)
	assert.Equal(t, 80, gotDY)
	assert.Equal(t, "scroll(-12,80)", gotScript)
	assert.Equal(t, 1, runCount)
}

func TestScrollPage_VariousRequestsForwardFallbackDeltas(t *testing.T) {
	oldBuilder := buildPageScrollFallbackJS
	oldRunner := runPageScrollFallbackJS
	defer func() {
		buildPageScrollFallbackJS = oldBuilder
		runPageScrollFallbackJS = oldRunner
	}()

	tests := []struct {
		name string
		req  port.PageScrollRequest
	}{
		{"zero request", port.PageScrollRequest{}},
		{"down", port.PageScrollRequest{Command: port.PageScrollCommandDown, FallbackDY: 80}},
		{"up", port.PageScrollRequest{Command: port.PageScrollCommandUp, FallbackDY: -80}},
		{"left", port.PageScrollRequest{Command: port.PageScrollCommandLeft, FallbackDX: -80}},
		{"right", port.PageScrollRequest{Command: port.PageScrollCommandRight, FallbackDX: 80}},
		{"up fast", port.PageScrollRequest{Command: port.PageScrollCommandUpFast, FallbackDY: -320}},
		{"down fast", port.PageScrollRequest{Command: port.PageScrollCommandDownFast, FallbackDY: 320}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotDX, gotDY int
			buildPageScrollFallbackJS = func(dx, dy int) string {
				gotDX, gotDY = dx, dy
				return "ok"
			}
			runPageScrollFallbackJS = func(_ *WebView, _ context.Context, script string) {
				assert.Equal(t, "ok", script)
			}

			wv := &WebView{
				uri:             "https://example.com",
				runJSErrorStats: make(map[string]runJSErrorStat),
			}
			err := wv.ScrollPage(context.Background(), tt.req)
			require.NoError(t, err)
			assert.Equal(t, tt.req.FallbackDX, gotDX)
			assert.Equal(t, tt.req.FallbackDY, gotDY)
		})
	}
}
