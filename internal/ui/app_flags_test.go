package ui

import (
	"testing"

	"github.com/jwijenbergh/puregotk/v4/gio"
	"github.com/stretchr/testify/assert"
)

func TestGTKApplicationFlags_AreNonUnique(t *testing.T) {
	flags := gtkApplicationFlags()

	assert.Equal(t, gio.GApplicationNonUniqueValue, flags)
	assert.NotEqual(t, gio.GApplicationFlagsNoneValue, flags)
}
