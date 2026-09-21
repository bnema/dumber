package runtimeprofile

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"path/filepath"
)

// WithBrowserProfile selects an isolated persistent CEF profile while keeping
// Dumber application data (configuration, history and favorites) shared.
func (p Profile) WithBrowserProfile(name, runtimeBase string) (Profile, error) {
	if p.Engine != engineCEF {
		return Profile{}, fmt.Errorf("--profile requires the CEF engine")
	}
	if !instanceNamePattern.MatchString(name) {
		return Profile{}, fmt.Errorf(
			"invalid profile name %q: use 1-32 ASCII letters, digits, underscores or hyphens, starting with a letter or digit",
			name,
		)
	}
	if !filepath.IsAbs(runtimeBase) {
		return Profile{}, fmt.Errorf("profile runtime base must be absolute")
	}
	p.InstanceRoot = filepath.Join(p.EnginePaths.RootDir, "profiles", name)
	p.InstanceAppIDSuffix = "profile-" + name
	p.IPC.RuntimeDir = filepath.Join(runtimeBase, "dumber", "profiles", name)
	p.IPC.BrowserLaunchSocket = filepath.Join(p.IPC.RuntimeDir, browserLaunchSocketName)
	if len(p.IPC.BrowserLaunchSocket) >= devIPCSocketPathLimit {
		return Profile{}, fmt.Errorf("profile socket path too long")
	}
	return p, nil
}

// WithEphemeralBrowserProfile selects a fresh temporary CEF profile. The
// caller owns cleanup after the host exits.
func (p Profile) WithEphemeralBrowserProfile(runtimeBase string) (Profile, error) {
	if p.Engine != engineCEF {
		return Profile{}, fmt.Errorf("--ephemeral requires the CEF engine")
	}
	if !filepath.IsAbs(runtimeBase) {
		return Profile{}, fmt.Errorf("profile runtime base must be absolute")
	}
	var nonce [8]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return Profile{}, fmt.Errorf("generate ephemeral profile identity: %w", err)
	}
	key := "ephemeral-" + hex.EncodeToString(nonce[:])
	p.InstanceRoot = filepath.Join(runtimeBase, "dumber", key)
	p.InstanceAppIDSuffix = key
	p.IPC.RuntimeDir = p.InstanceRoot
	p.IPC.BrowserLaunchSocket = filepath.Join(p.IPC.RuntimeDir, browserLaunchSocketName)
	if len(p.IPC.BrowserLaunchSocket) >= devIPCSocketPathLimit {
		return Profile{}, fmt.Errorf("ephemeral profile socket path too long")
	}
	return p, nil
}
