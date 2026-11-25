package content

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"bnema.dev/dumber/internal/domain/entity"
)

func TestPermissionTypesFromStringsMapsLocalNetwork(t *testing.T) {
	t.Parallel()
	got := permissionTypesFromStrings(context.Background(), []string{"geolocation", "local_network", "bogus"})
	assert.Equal(t, []entity.PermissionType{entity.PermissionTypeGeolocation, entity.PermissionTypeLocalNetwork}, got)
}
