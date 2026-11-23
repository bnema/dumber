package content

import (
	"context"
	"testing"

	"bnema.dev/dumber/internal/application/port"
	"bnema.dev/dumber/internal/application/port/mocks"
	"bnema.dev/dumber/internal/domain/entity"
	"github.com/stretchr/testify/assert"
)

func TestShouldRenderCrashPage(t *testing.T) {
	assert.True(t, shouldRenderCrashPage(port.WebProcessTerminationCrashed))
	assert.True(t, shouldRenderCrashPage(port.WebProcessTerminationExceededMemory))
	assert.False(t, shouldRenderCrashPage(port.WebProcessTerminationByAPI))
	assert.False(t, shouldRenderCrashPage(port.WebProcessTerminationRuntimeReplaced))
	assert.True(t, shouldRenderCrashPage(port.WebProcessTerminationReason(99)))
}

func TestExtractOriginalURIFromCrashPage(t *testing.T) {
	assert.Equal(t, "https://example.com/path?q=1", extractOriginalURIFromCrashPage("dumb://history/crash?url=https%3A%2F%2Fexample.com%2Fpath%3Fq%3D1"))
	assert.Empty(t, extractOriginalURIFromCrashPage("dumb://history/crash"))
	assert.Equal(t, "https://example.com", extractOriginalURIFromCrashPage("https://example.com"))
	assert.Equal(t, "%%%bad", extractOriginalURIFromCrashPage("%%%bad"))
}

func TestBuildCrashPageURI(t *testing.T) {
	assert.Equal(t, "dumb://history/crash", buildCrashPageURI(""))
	assert.Equal(t, "dumb://history/crash?url=https%3A%2F%2Fexample.com%2Ffoo%3Fa%3D1%26b%3D2", buildCrashPageURI("https://example.com/foo?a=1&b=2"))
}

func TestHandleWebProcessTerminated(t *testing.T) {
	const pageURI = "https://example.com/page"
	tests := []struct {
		name         string
		reason       port.WebProcessTerminationReason
		loadsCrashUI bool
	}{
		// The crash page needs a renderer too, which a replaced runtime kills.
		{"runtime replaced shows no crash page", port.WebProcessTerminationRuntimeReplaced, false},
		{"killed shows no crash page", port.WebProcessTerminationByAPI, false},
		{"crash loads the crash page", port.WebProcessTerminationCrashed, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			// No pane view resolver: the notice path must still return safely.
			c := &Coordinator{}
			wv := mocks.NewMockWebView(t)
			if tt.loadsCrashUI {
				wv.EXPECT().LoadURI(ctx, buildCrashPageURI(pageURI)).Return(nil).Once()
			}

			c.handleWebProcessTerminated(ctx, entity.PaneID("pane-1"), wv, tt.reason, "label", pageURI)
		})
	}
}
