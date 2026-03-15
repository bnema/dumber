package content

import (
	"testing"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/stretchr/testify/assert"
)

func TestShouldRenderCrashPage(t *testing.T) {
	assert.True(t, shouldRenderCrashPage(port.WebProcessTerminationCrashed))
	assert.True(t, shouldRenderCrashPage(port.WebProcessTerminationExceededMemory))
	assert.False(t, shouldRenderCrashPage(port.WebProcessTerminationByAPI))
	assert.True(t, shouldRenderCrashPage(port.WebProcessTerminationReason(99)))
}

func TestExtractOriginalURIFromCrashPage(t *testing.T) {
	assert.Equal(t, "https://example.com/path?q=1", extractOriginalURIFromCrashPage("dumb://home/crash?url=https%3A%2F%2Fexample.com%2Fpath%3Fq%3D1"))
	assert.Empty(t, extractOriginalURIFromCrashPage("dumb://home/crash"))
	assert.Equal(t, "https://example.com", extractOriginalURIFromCrashPage("https://example.com"))
	assert.Equal(t, "%%%bad", extractOriginalURIFromCrashPage("%%%bad"))
}

func TestBuildCrashPageURI(t *testing.T) {
	assert.Equal(t, "dumb://home/crash", buildCrashPageURI(""))
	assert.Equal(t, "dumb://home/crash?url=https%3A%2F%2Fexample.com%2Ffoo%3Fa%3D1%26b%3D2", buildCrashPageURI("https://example.com/foo?a=1&b=2"))
}
