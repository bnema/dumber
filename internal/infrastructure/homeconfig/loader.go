// Package homeconfig loads the user's dumb://home layout from home.toml.
package homeconfig

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"

	"github.com/pelletier/go-toml/v2"

	"bnema.dev/dumber/internal/application/dto"
	"bnema.dev/dumber/internal/application/port"
	"bnema.dev/dumber/internal/domain/entity"
)

// FileName is the home layout file name inside the config directory.
const FileName = "home.toml"

const (
	dirPerm  = 0o755
	filePerm = 0o644
)

// Loader reads home.toml and remembers the last valid layout.
type Loader struct {
	path string

	mu        sync.Mutex
	lastValid *entity.HomeLayout
}

var _ port.HomeConfigSource = (*Loader)(nil)

// New returns a loader for <configDir>/home.toml.
func New(configDir string) *Loader {
	return &Loader{path: filepath.Join(configDir, FileName)}
}

// Path returns the absolute path of home.toml.
func (l *Loader) Path() string { return l.path }

// Load reads, decodes, normalizes and validates home.toml.
func (l *Loader) Load(_ context.Context) (port.HomeConfigSnapshot, error) {
	snap := port.HomeConfigSnapshot{Path: l.path}

	data, err := os.ReadFile(l.path)
	if errors.Is(err, fs.ErrNotExist) {
		snap.Layout = entity.DefaultHomeLayout()
		return snap, nil
	}
	if err != nil {
		snap.Layout = l.fallback()
		return snap, fmt.Errorf("read %s: %w", l.path, err)
	}
	snap.Exists = true

	var layout entity.HomeLayout
	dec := toml.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&layout); err != nil {
		snap.ParseError = parseError(l.path, err)
		snap.Layout = l.fallback()
		return snap, nil
	}

	layout.Normalize()
	if errs := layout.Validate(); len(errs) > 0 {
		snap.ValidationErrors = errs
		snap.Layout = l.fallback()
		return snap, nil
	}

	l.mu.Lock()
	l.lastValid = &layout
	l.mu.Unlock()
	snap.Layout = layout
	return snap, nil
}

func (l *Loader) fallback() entity.HomeLayout {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.lastValid != nil {
		return *l.lastValid
	}
	return entity.DefaultHomeLayout()
}

func parseError(file string, err error) *dto.HomeParseError {
	pe := &dto.HomeParseError{File: file, Message: err.Error()}

	var missing *toml.StrictMissingError
	if errors.As(err, &missing) && len(missing.Errors) > 0 {
		first := missing.Errors[0]
		pe.Line, pe.Column = first.Position()
		pe.Message = fmt.Sprintf("unknown key %q", joinKey(first.Key()))
		if n := len(missing.Errors); n > 1 {
			pe.Message += fmt.Sprintf(" (and %d more)", n-1)
		}
		return pe
	}

	var decErr *toml.DecodeError
	if errors.As(err, &decErr) {
		pe.Line, pe.Column = decErr.Position()
		pe.Message = decErr.Error()
	}
	return pe
}

func joinKey(k toml.Key) string {
	out := ""
	for i, part := range k {
		if i > 0 {
			out += "."
		}
		out += part
	}
	return out
}

// EnsureExists writes the starter home.toml if it does not exist yet.
func (l *Loader) EnsureExists() (bool, error) {
	if _, err := os.Stat(l.path); err == nil {
		return false, nil
	} else if !errors.Is(err, fs.ErrNotExist) {
		return false, fmt.Errorf("stat %s: %w", l.path, err)
	}
	if err := os.MkdirAll(filepath.Dir(l.path), dirPerm); err != nil {
		return false, fmt.Errorf("create config dir: %w", err)
	}
	f, err := os.OpenFile(l.path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, filePerm)
	if errors.Is(err, fs.ErrExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("create %s: %w", l.path, err)
	}
	if _, err := f.WriteString(StarterTemplate); err != nil {
		_ = f.Close()
		return false, fmt.Errorf("write %s: %w", l.path, err)
	}
	if err := f.Close(); err != nil {
		return false, fmt.Errorf("close %s: %w", l.path, err)
	}
	return true, nil
}
