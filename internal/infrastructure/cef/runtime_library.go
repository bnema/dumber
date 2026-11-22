package cef

import (
	"errors"
	"io/fs"
	"os"
	"syscall"
)

// fileIdentity identifies a file by device and inode, which change when a
// package manager replaces the file instead of editing it in place.
type fileIdentity struct {
	dev uint64
	ino uint64
}

// runtimeLibrary remembers which libcef file this process loaded.
//
// Renderer subprocesses load libcef again from the same path when they start.
// If a package update replaces that file while Dumber runs, new renderers run
// a different CEF version than the browser process and Chromium kills them on
// their first IPC message, so no page can load until Dumber restarts.
type runtimeLibrary struct {
	path     string
	identity fileIdentity
}

// newRuntimeLibrary records the identity of the libcef file at path. It
// returns nil when the path is unknown or cannot be read, which disables the
// replacement check rather than reporting false replacements.
func newRuntimeLibrary(path string) *runtimeLibrary {
	if path == "" {
		return nil
	}
	identity, err := statIdentity(path)
	if err != nil {
		return nil
	}
	return &runtimeLibrary{path: path, identity: identity}
}

// replaced reports whether the file at the recorded path is no longer the
// library this process loaded. A nil receiver reports false.
func (l *runtimeLibrary) replaced() bool {
	if l == nil {
		return false
	}
	identity, err := statIdentity(l.path)
	if errors.Is(err, fs.ErrNotExist) {
		return true
	}
	if err != nil {
		return false
	}
	return identity != l.identity
}

// runtimeLibraryReplaced reports whether the libcef file loaded at startup has
// been replaced on disk. It is safe on a nil Engine.
func (e *Engine) runtimeLibraryReplaced() bool {
	return e != nil && e.runtimeLib.replaced()
}

func statIdentity(path string) (fileIdentity, error) {
	info, err := os.Stat(path)
	if err != nil {
		return fileIdentity{}, err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return fileIdentity{}, errors.New("file identity unavailable")
	}
	return fileIdentity{dev: stat.Dev, ino: stat.Ino}, nil
}
