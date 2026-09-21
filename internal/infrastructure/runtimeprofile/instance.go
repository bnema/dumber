package runtimeprofile

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"regexp"
)

var instanceNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,31}$`)

// WithInstance isolates process coordination for a named CEF instance while
// retaining every shared user-data path from the base profile.
func (p Profile) WithInstance(name, runtimeBase string) (Profile, error) {
	if p.Engine != engineCEF {
		return Profile{}, fmt.Errorf("--instance requires the CEF engine")
	}
	if !instanceNamePattern.MatchString(name) {
		return Profile{}, fmt.Errorf(
			"invalid instance name %q: use 1-32 ASCII letters, digits, underscores or hyphens, starting with a letter or digit",
			name,
		)
	}
	if !filepath.IsAbs(runtimeBase) {
		return Profile{}, fmt.Errorf("instance runtime base must be absolute")
	}

	namespace := string(p.Mode) + "\x00" + name
	if p.Mode == ModeDev {
		namespace += "\x00" + devIPCRootHash(p.Shared.RootDir)
	}
	sum := sha256.Sum256([]byte(namespace))
	key := "i" + hex.EncodeToString(sum[:8])
	root := filepath.Join(runtimeBase, "dumber", "instances", key)
	socket := filepath.Join(root, browserLaunchSocketName)
	if len(socket) >= devIPCSocketPathLimit {
		return Profile{}, fmt.Errorf("instance socket path too long")
	}
	p.InstanceRoot = root
	p.InstanceAppIDSuffix = key
	p.IPC = IPCPaths{RuntimeDir: root, BrowserLaunchSocket: socket}
	return p, nil
}
