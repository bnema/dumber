package ui

import (
	"testing"

	"bnema.dev/puregotk/v4/gio"
	"github.com/stretchr/testify/assert"
)

func TestGTKApplicationFlags_AreNonUnique(t *testing.T) {
	flags := gtkApplicationFlags()

	assert.Equal(t, gio.GApplicationNonUniqueValue, flags)
	assert.NotEqual(t, gio.GApplicationFlagsNoneValue, flags)
}
