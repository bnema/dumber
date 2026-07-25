package ui

import (
	"testing"

	"github.com/bnema/puregotk/v4/gtk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type popupStagingShellSpy struct {
	detachCalls  int
	destroyCalls int
	showCalls    int
	content      *gtk.Widget
}

func (s *popupStagingShellSpy) SetContent(widget *gtk.Widget) { s.content = widget }
func (s *popupStagingShellSpy) DetachContent() *gtk.Widget {
	s.detachCalls++
	widget := s.content
	s.content = nil
	return widget
}
func (s *popupStagingShellSpy) Destroy() { s.destroyCalls++ }

func TestPopupStagingHostDetachRemovesContentThenDestroysShell(t *testing.T) {
	t.Parallel()

	widget := &gtk.Widget{}
	shell := &popupStagingShellSpy{content: widget}
	host := &popupStagingHost{shell: shell, attached: widget}

	require.NoError(t, host.Detach())
	assert.Equal(t, 1, shell.detachCalls)
	assert.Equal(t, 1, shell.destroyCalls)
	assert.Nil(t, shell.content)
	assert.Nil(t, host.attached)
}

func TestPopupStagingHostDestroyIsIdempotentAndNeverShows(t *testing.T) {
	t.Parallel()

	shell := &popupStagingShellSpy{content: &gtk.Widget{}}
	host := &popupStagingHost{shell: shell, attached: shell.content}

	host.Destroy()
	host.Destroy()
	require.NoError(t, host.Detach())

	assert.Equal(t, 1, shell.detachCalls)
	assert.Equal(t, 1, shell.destroyCalls)
	assert.Zero(t, shell.showCalls)
}
