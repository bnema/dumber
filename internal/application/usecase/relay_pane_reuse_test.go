package usecase

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/bnema/dumber/internal/domain/entity"
	domainurl "github.com/bnema/dumber/internal/domain/url"
)

func TestRelayPaneReuse_PrunesExpiredAndMissingPanes(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	reg := RelayPaneReuse{Now: func() time.Time { return now }}
	reg.Record("https://a.example/", "pane-a")
	reg.Record("https://b.example/", "pane-b")

	exists := func(id entity.PaneID) bool { return id != "pane-b" }
	_, ok := reg.Lookup("https://b.example/", exists)
	assert.False(t, ok)
	assert.Equal(t, 1, reg.Len())

	id, ok := reg.Lookup("https://a.example/", exists)
	assert.True(t, ok)
	assert.Equal(t, entity.PaneID("pane-a"), id)

	now = now.Add(domainurl.RelayReuseWindow + time.Second)
	_, ok = reg.Lookup("https://a.example/", exists)
	assert.False(t, ok)
	assert.Equal(t, 0, reg.Len())
}
