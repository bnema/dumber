package runtimeprofile

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func testCEFProfile(t *testing.T) Profile {
	t.Helper()
	root := t.TempDir()
	return Profile{
		Mode:   ModeProd,
		Engine: engineCEF,
		EnginePaths: EnginePaths{
			RootDir: filepath.Join(root, "state", "engines", "cef"),
		},
		Shared: SharedPaths{DataDir: filepath.Join(root, "data")},
	}
}

func TestWithBrowserProfileSeparatesCEFStorageAndIPC(t *testing.T) {
	base := testCEFProfile(t)
	runtimeBase := "/tmp/dumber-profile-test"
	profile, err := base.WithBrowserProfile("work", runtimeBase)
	require.NoError(t, err)
	require.Equal(t, filepath.Join(base.EnginePaths.RootDir, "profiles", "work", "cef"), profile.CEFUserDataDir())
	require.Equal(t, filepath.Join(runtimeBase, "dumber", "profiles", "work", browserLaunchSocketName), profile.IPC.BrowserLaunchSocket)
	require.Equal(t, base.Shared, profile.Shared)
}

func TestWithEphemeralBrowserProfileCreatesUniqueRoots(t *testing.T) {
	base := testCEFProfile(t)
	runtimeBase := "/tmp/dumber-ephemeral-test"
	first, err := base.WithEphemeralBrowserProfile(runtimeBase)
	require.NoError(t, err)
	second, err := base.WithEphemeralBrowserProfile(runtimeBase)
	require.NoError(t, err)
	require.NotEqual(t, first.InstanceRoot, second.InstanceRoot)
	require.Equal(t, filepath.Join(first.InstanceRoot, "cef"), first.CEFUserDataDir())
}
