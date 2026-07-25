package content

import (
	"bytes"
	"encoding/json"
	"errors"
	"testing"

	"github.com/bnema/dumber/internal/application/dto"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLogBrowsingContextDecisionUsesStructuredSafeFields(t *testing.T) {
	t.Parallel()

	var output bytes.Buffer
	logger := zerolog.New(&output)
	req := dto.NewBrowsingContextRequest{
		Engine: dto.BrowserEngineWebKit, SourceHost: dto.SourceHostFloating,
		TargetURI: "https://example.com/private?token=secret", TargetDisposition: dto.WindowDispositionNewPopup,
	}
	decision := dto.HostDecision{Kind: dto.HostDecisionCreateBrowserWindow, ReasonCode: dto.HostDecisionReasonFloatingBrowserWindow}

	logBrowsingContextDecision(logger, req, decision)
	record := decodeBrowsingContextLog(t, output.Bytes())
	assert.Equal(t, "webkit", record["engine"])
	assert.Equal(t, "floating", record["source_host"])
	assert.Equal(t, "create-browser-window", record["decision"])
	assert.Equal(t, "new-popup", record["target_disposition"])
	assert.Equal(t, "floating-browser-window", record["reason_code"])
	assert.NotContains(t, output.String(), "token")
	assert.NotContains(t, output.String(), "secret")
}

func TestLogBrowsingContextFailureUsesTypedReasonCode(t *testing.T) {
	t.Parallel()

	var output bytes.Buffer
	logger := zerolog.New(&output)
	req := dto.NewBrowsingContextRequest{Engine: dto.BrowserEngineCEF, SourceHost: dto.SourceHostWorkspace, TargetDisposition: dto.WindowDispositionNewTab}
	decision := dto.HostDecision{Kind: dto.HostDecisionCreateNativePopup}

	logBrowsingContextFailure(logger, req, decision, dto.BrowsingContextFailureHostFailed, errors.New("attachment failed"))
	record := decodeBrowsingContextLog(t, output.Bytes())
	assert.Equal(t, "cef", record["engine"])
	assert.Equal(t, "workspace", record["source_host"])
	assert.Equal(t, "create-native-popup", record["decision"])
	assert.Equal(t, "new-tab", record["target_disposition"])
	assert.Equal(t, "host-failed", record["reason_code"])
}

func decodeBrowsingContextLog(t *testing.T, data []byte) map[string]any {
	t.Helper()
	var record map[string]any
	require.NoError(t, json.Unmarshal(data, &record))
	for _, field := range []string{"engine", "source_host", "decision", "target_disposition", "reason_code"} {
		assert.Contains(t, record, field)
	}
	return record
}
