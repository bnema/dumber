package runtimeprofile

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWithInstanceDeterministicAndIsolated(t *testing.T) {
	base := Profile{Engine: engineCEF, Shared: SharedPaths{DataDir: "/shared/data"}}
	first, err := base.WithInstance("work", "/tmp")
	require.NoError(t, err)
	again, err := base.WithInstance("work", "/tmp")
	require.NoError(t, err)
	other, err := base.WithInstance("personal", "/tmp")
	require.NoError(t, err)

	require.Equal(t, first.InstanceRoot, again.InstanceRoot)
	require.Equal(t, first.IPC, again.IPC)
	require.Equal(t, first.InstanceAppIDSuffix, again.InstanceAppIDSuffix)
	require.NotEqual(t, first.InstanceRoot, other.InstanceRoot)
	require.NotEqual(t, first.IPC.BrowserLaunchSocket, other.IPC.BrowserLaunchSocket)
	require.Equal(t, base.Shared, first.Shared)
	require.Equal(t, filepath.Join(first.InstanceRoot, "cef"), first.CEFUserDataDir())
}

func TestWithInstanceSeparatesProdAndDevRoots(t *testing.T) {
	prod := Profile{Engine: engineCEF, Mode: ModeProd}
	devA := Profile{Engine: engineCEF, Mode: ModeDev, Shared: SharedPaths{RootDir: "/src/a/.dev/dumber"}}
	devB := Profile{Engine: engineCEF, Mode: ModeDev, Shared: SharedPaths{RootDir: "/src/b/.dev/dumber"}}

	prodInstance, err := prod.WithInstance("work", "/tmp")
	require.NoError(t, err)
	devAInstance, err := devA.WithInstance("work", "/tmp")
	require.NoError(t, err)
	devBInstance, err := devB.WithInstance("work", "/tmp")
	require.NoError(t, err)
	require.NotEqual(t, prodInstance.InstanceRoot, devAInstance.InstanceRoot)
	require.NotEqual(t, devAInstance.InstanceRoot, devBInstance.InstanceRoot)
}

func TestWithInstanceRejectsUnsafeNames(t *testing.T) {
	base := Profile{Engine: engineCEF}
	for _, name := range []string{"", "../work", "has space", ".hidden", "é", "abcdefghijklmnopqrstuvwxyz1234567"} {
		_, err := base.WithInstance(name, "/tmp")
		require.Error(t, err, name)
	}
	_, err := (Profile{Engine: "webkit"}).WithInstance("work", "/tmp")
	require.Error(t, err)
	_, err = base.WithInstance("work", "relative")
	require.Error(t, err)
}
