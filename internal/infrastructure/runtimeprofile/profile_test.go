package runtimeprofile

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestResolve_ProdPreservesSharedLayout(t *testing.T) {
	profile, err := Resolve(ResolveInput{
		Env:    func(string) string { return "" },
		Engine: "webkit",
		CWD:    func() (string, error) { return "/repo", nil },
		Base: BasePaths{
			ConfigHome: "/home/u/.config/dumber",
			DataHome:   "/home/u/.local/share/dumber",
			StateHome:  "/home/u/.local/state/dumber",
			CacheHome:  "/home/u/.cache/dumber",
		},
	})
	require.NoError(t, err)
	require.Equal(t, ModeProd, profile.Mode)
	require.Equal(t, "/home/u/.config/dumber", profile.Shared.ConfigDir)
	require.Equal(t, "/home/u/.local/share/dumber", profile.Shared.DataDir)
	require.Equal(t, "/home/u/.local/state/dumber", profile.Shared.StateDir)
	require.Equal(t, "/home/u/.cache/dumber", profile.Shared.CacheDir)
	require.Equal(t, "/home/u/.local/state/dumber/logs", profile.Shared.LogDir)
	require.Equal(t, "/home/u/.local/share/dumber/cef_user_data", profile.CEFUserDataDir())
	require.Equal(t, "/home/u/.local/share/dumber", profile.WebKitDataDir())
	require.Equal(t, "/home/u/.local/state/dumber/webkit-cache", profile.WebKitCacheDir())
}

func TestResolve_DevCEF_SeparatesSharedEngineAndIPC(t *testing.T) {
	profile, err := Resolve(ResolveInput{
		Env: func(key string) string {
			if key == "ENV" {
				return "dev"
			}
			return ""
		},
		Engine: "cef",
		CWD:    func() (string, error) { return "/repo", nil },
	})
	require.NoError(t, err)
	require.Equal(t, ModeDev, profile.Mode)
	require.Equal(t, "/repo/.dev/dumber/config", profile.Shared.ConfigDir)
	require.Equal(t, "/repo/.dev/dumber/data", profile.Shared.DataDir)
	require.Equal(t, "/repo/.dev/dumber/engines/cef/data", profile.CEFUserDataDir())
	require.Equal(t, "/repo/.dev/dumber/runtime/cef/browser-launch.sock", profile.IPC.BrowserLaunchSocket)
}

func TestResolve_DevWebKitAndCEF_HaveDifferentTechnicalNamespaces(t *testing.T) {
	devEnv := func(key string) string {
		if key == "ENV" {
			return "dev"
		}
		return ""
	}
	repoCWD := func() (string, error) { return "/repo", nil }

	cefProfile, err := Resolve(ResolveInput{Env: devEnv, Engine: "cef", CWD: repoCWD})
	require.NoError(t, err)
	wkProfile, err := Resolve(ResolveInput{Env: devEnv, Engine: "webkit", CWD: repoCWD})
	require.NoError(t, err)

	require.NotEqual(t, cefProfile.EnginePaths.RootDir, wkProfile.EnginePaths.RootDir)
	require.NotEqual(t, cefProfile.IPC.RuntimeDir, wkProfile.IPC.RuntimeDir)
	require.NotEqual(t, cefProfile.IPC.BrowserLaunchSocket, wkProfile.IPC.BrowserLaunchSocket)
}

func TestResolve_DefaultsEmptyEngineToWebKit(t *testing.T) {
	profile, err := Resolve(ResolveInput{
		Env:  func(string) string { return "" },
		CWD:  func() (string, error) { return "/repo", nil },
		Base: BasePaths{StateHome: "/state", DataHome: "/data"},
	})
	require.NoError(t, err)
	require.Equal(t, "webkit", profile.Engine)
	require.Equal(t, "/state/runtime/webkit/browser-launch.sock", profile.IPC.BrowserLaunchSocket)
}

func TestResolve_NormalizesEngineWhitespaceAndCase(t *testing.T) {
	profile, err := Resolve(ResolveInput{
		Env:    func(string) string { return "" },
		Engine: "  CEF ",
		CWD:    func() (string, error) { return "/repo", nil },
		Base:   BasePaths{StateHome: "/state", DataHome: "/data"},
	})
	require.NoError(t, err)
	require.Equal(t, "cef", profile.Engine)
	require.Equal(t, "/state/runtime/cef/browser-launch.sock", profile.IPC.BrowserLaunchSocket)
}

func TestResolve_DefaultsUnknownEngineToWebKit(t *testing.T) {
	profile, err := Resolve(ResolveInput{
		Env:    func(string) string { return "" },
		Engine: "wekbit",
		CWD:    func() (string, error) { return "/repo", nil },
		Base:   BasePaths{StateHome: "/state", DataHome: "/data"},
	})
	require.NoError(t, err)
	require.Equal(t, "webkit", profile.Engine)
	require.Equal(t, "/state/runtime/webkit/browser-launch.sock", profile.IPC.BrowserLaunchSocket)
}

func TestResolve_DevFailsWhenCWDUnavailable(t *testing.T) {
	want := errors.New("boom")
	_, err := Resolve(ResolveInput{
		Env: func(key string) string {
			if key == "ENV" {
				return "dev"
			}
			return ""
		},
		Engine: "cef",
		CWD:    func() (string, error) { return "", want },
	})
	require.ErrorIs(t, err, want)
}
