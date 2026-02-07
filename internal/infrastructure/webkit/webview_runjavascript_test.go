package webkit

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
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
