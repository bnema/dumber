package runtimeprofile

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"strings"
)

const (
	browserLaunchSocketName = "browser-launch.sock"
	devIPCSocketPathLimit   = 104
	engineWebKit            = "webkit"
	engineCEF               = "cef"
)

// Mode identifies the execution sandbox mode.
type Mode string

const (
	ModeProd Mode = "prod"
	ModeDev  Mode = "dev"
)

// BasePaths holds the already-resolved dumber XDG homes used in prod.
type BasePaths struct {
	ConfigHome string
	DataHome   string
	StateHome  string
	CacheHome  string
}

// ResolveInput contains the external inputs needed to resolve a runtime profile.
type ResolveInput struct {
	Env    func(string) string
	CWD    func() (string, error)
	Engine string
	Base   BasePaths
}

// SharedPaths are shared by the whole runtime sandbox.
type SharedPaths struct {
	// RootDir is only populated when the shared sandbox has a single root
	// directory (for example ENV=dev under .dev/dumber). It is intentionally
	// empty in prod, where shared paths come from distinct XDG homes.
	RootDir   string
	ConfigDir string
	DataDir   string
	StateDir  string
	CacheDir  string
	LogDir    string
}

// EnginePaths are engine-specific technical paths.
type EnginePaths struct {
	Name       string
	RootDir    string
	RuntimeDir string
	LogDir     string
}

// IPCPaths are runtime coordination paths.
type IPCPaths struct {
	RuntimeDir          string
	BrowserLaunchSocket string
}

// Profile is the fully resolved runtime profile for one mode+engine namespace.
type Profile struct {
	Mode        Mode
	Engine      string
	Shared      SharedPaths
	EnginePaths EnginePaths
	IPC         IPCPaths
}

// Resolve builds a runtime profile from pure inputs.
func Resolve(input ResolveInput) (Profile, error) {
	engine := normalizeEngine(input.Engine)
	if envValue(input.Env, "ENV") == string(ModeDev) {
		cwd, err := cwdValue(input.CWD)
		if err != nil {
			return Profile{}, fmt.Errorf("resolve cwd: %w", err)
		}
		root := filepath.Join(cwd, ".dev", "dumber")
		engineRoot := filepath.Join(root, "engines", engine)
		ipcRoot, err := devIPCRoot(input.Env, root, engine)
		if err != nil {
			return Profile{}, err
		}
		return Profile{
			Mode:   ModeDev,
			Engine: engine,
			Shared: SharedPaths{
				RootDir:   root,
				ConfigDir: filepath.Join(root, "config"),
				DataDir:   filepath.Join(root, "data"),
				StateDir:  filepath.Join(root, "state"),
				CacheDir:  filepath.Join(root, "cache"),
				LogDir:    filepath.Join(root, "logs"),
			},
			EnginePaths: EnginePaths{
				Name:       engine,
				RootDir:    engineRoot,
				RuntimeDir: filepath.Join(engineRoot, "runtime"),
				LogDir:     filepath.Join(engineRoot, "logs"),
			},
			IPC: IPCPaths{
				RuntimeDir:          ipcRoot,
				BrowserLaunchSocket: filepath.Join(ipcRoot, browserLaunchSocketName),
			},
		}, nil
	}

	shared := SharedPaths{
		ConfigDir: input.Base.ConfigHome,
		DataDir:   input.Base.DataHome,
		StateDir:  input.Base.StateHome,
		CacheDir:  input.Base.CacheHome,
		LogDir:    filepath.Join(input.Base.StateHome, "logs"),
	}
	engineRoot := filepath.Join(shared.StateDir, "engines", engine)
	ipcRoot := filepath.Join(shared.StateDir, "runtime", engine)
	return Profile{
		Mode:   ModeProd,
		Engine: engine,
		Shared: shared,
		EnginePaths: EnginePaths{
			Name:       engine,
			RootDir:    engineRoot,
			RuntimeDir: filepath.Join(engineRoot, "runtime"),
			LogDir:     filepath.Join(engineRoot, "logs"),
		},
		IPC: IPCPaths{
			RuntimeDir:          ipcRoot,
			BrowserLaunchSocket: filepath.Join(ipcRoot, browserLaunchSocketName),
		},
	}, nil
}

func devIPCRoot(env func(string) string, root, engine string) (string, error) {
	rootHash := devIPCRootHash(root)
	for _, base := range []string{envValue(env, "XDG_RUNTIME_DIR"), envValue(env, "TMPDIR")} {
		if base == "" {
			continue
		}
		ipcRoot := devIPCRootFromBase(base, rootHash, engine)
		if len(filepath.Join(ipcRoot, browserLaunchSocketName)) < devIPCSocketPathLimit {
			return ipcRoot, nil
		}
	}

	ipcRoot := filepath.Join(root, "runtime", engine)
	socketPath := filepath.Join(ipcRoot, browserLaunchSocketName)
	if len(socketPath) < devIPCSocketPathLimit {
		return ipcRoot, nil
	}
	return "", fmt.Errorf(
		"browser launch socket path too long: %d bytes; set XDG_RUNTIME_DIR or TMPDIR to a shorter runtime directory",
		len(socketPath),
	)
}

func devIPCRootFromBase(base, rootHash, engine string) string {
	return filepath.Join(base, "dumber", "dev-"+rootHash, engine)
}

func devIPCRootHash(root string) string {
	sum := sha256.Sum256([]byte(root))
	return hex.EncodeToString(sum[:])[:12]
}

// normalizeEngine trims and lowercases the runtime engine name.
// Supported engines are "webkit" and "cef"; any other value falls back to
// the default "webkit" runtime namespace.
func normalizeEngine(engine string) string {
	switch normalized := strings.ToLower(strings.TrimSpace(engine)); normalized {
	case "", engineWebKit:
		return engineWebKit
	case engineCEF:
		return engineCEF
	default:
		return engineWebKit
	}
}

func envValue(get func(string) string, key string) string {
	if get == nil {
		return ""
	}
	return get(key)
}

func cwdValue(get func() (string, error)) (string, error) {
	if get == nil {
		return "", fmt.Errorf("cwd getter is nil")
	}
	return get()
}

// CEFUserDataDir returns the resolved CEF root cache/user-data directory.
func (p Profile) CEFUserDataDir() string {
	if p.Mode == ModeDev {
		return filepath.Join(p.EnginePaths.RootDir, "data")
	}
	return filepath.Join(p.Shared.DataDir, "cef_user_data")
}

// CEFLogFile returns the resolved default CEF runtime log path.
func (p Profile) CEFLogFile() string {
	return filepath.Join(p.EnginePaths.LogDir, "cef_runtime.log")
}

// WebKitDataDir returns the resolved WebKit website data directory.
func (p Profile) WebKitDataDir() string {
	if p.Mode == ModeDev {
		return filepath.Join(p.EnginePaths.RootDir, "data")
	}
	return p.Shared.DataDir
}

// WebKitCacheDir returns the resolved WebKit cache directory.
func (p Profile) WebKitCacheDir() string {
	if p.Mode == ModeDev {
		return filepath.Join(p.EnginePaths.RootDir, "cache")
	}
	// Keep the current prod layout under state for non-user-managed technical
	// engine cache data; this intentionally preserves existing WebKit behavior.
	return filepath.Join(p.Shared.StateDir, "webkit-cache")
}
