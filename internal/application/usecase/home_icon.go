package usecase

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	appport "bnema.dev/dumber/internal/application/port"
	"bnema.dev/dumber/internal/domain/entity"
	"bnema.dev/dumber/internal/domain/favicon"
	"bnema.dev/dumber/internal/logging"
)

// HomeIconSize is the square size of home service icons.
const HomeIconSize = 128

// ErrHomeIconNotFound reports that no usable icon exists for a home service.
var ErrHomeIconNotFound = appport.ErrHomeIconNotFound

const homeIconContentType = "image/png"

// HomeIconNegativeTTL is how long a failed resolution is remembered.
const HomeIconNegativeTTL = 10 * time.Minute

// homeIconResolveTimeout bounds one shared resolution (page, manifest and
// every candidate fetch).
const homeIconResolveTimeout = 20 * time.Second

var (
	_ appport.HomeIconResolver  = (*HomeIconUseCase)(nil)
	_ appport.HomeIconVersioner = (*HomeIconUseCase)(nil)
)

// HomeIconDeps wires the home service icon use case.
type HomeIconDeps struct {
	Source    appport.HomeIconSource
	Converter appport.FaviconImageConverter
	Cache     appport.HomeIconCache
	// Config and ConfigDir enable ResolveHomeIconByID.
	Config    appport.HomeConfigSource
	ConfigDir string
	Now       func() time.Time
	TTL       time.Duration
}

// HomeIconUseCase resolves the best icon for home page service tiles.
type HomeIconUseCase struct {
	source    appport.HomeIconSource
	converter appport.FaviconImageConverter
	cache     appport.HomeIconCache
	config    appport.HomeConfigSource
	configDir string
	now       func() time.Time
	ttl       time.Duration

	mu       sync.Mutex
	failures map[string]time.Time // key -> failure time
	inflight map[string]*homeIconCall
}

type homeIconCall struct {
	done chan struct{}
	png  []byte
	err  error
}

// NewHomeIconUseCase creates a HomeIconUseCase.
func NewHomeIconUseCase(deps HomeIconDeps) *HomeIconUseCase {
	if deps.Now == nil {
		deps.Now = time.Now
	}
	if deps.TTL <= 0 {
		deps.TTL = favicon.DefaultTTL
	}
	return &HomeIconUseCase{
		source:    deps.Source,
		converter: deps.Converter,
		cache:     deps.Cache,
		config:    deps.Config,
		configDir: deps.ConfigDir,
		now:       deps.Now,
		ttl:       deps.TTL,
		failures:  make(map[string]time.Time),
		inflight:  make(map[string]*homeIconCall),
	}
}

// ResolveHomeIconByID resolves the icon of a home.toml service by id. The
// override path is joined to the config dir and must stay inside it.
func (uc *HomeIconUseCase) ResolveHomeIconByID(ctx context.Context, serviceID string) ([]byte, string, error) {
	if !entity.IsValidHomeID(serviceID) {
		return nil, "", fmt.Errorf("%w: invalid service id", ErrHomeIconNotFound)
	}
	if uc.config == nil {
		return nil, "", fmt.Errorf("%w: home config unavailable", ErrHomeIconNotFound)
	}
	snap, err := uc.config.Load(ctx)
	if err != nil {
		return nil, "", fmt.Errorf("%w: load home config: %w", ErrHomeIconNotFound, err)
	}
	service, ok := snap.Layout.FindService(serviceID)
	if !ok {
		return nil, "", fmt.Errorf("%w: unknown service %q", ErrHomeIconNotFound, serviceID)
	}
	override := ""
	if service.Icon != "" {
		override, ok = resolveUnderDir(uc.configDir, service.Icon)
		if !ok {
			return nil, "", fmt.Errorf("%w: icon path escapes config dir", ErrHomeIconNotFound)
		}
	}
	return uc.resolveShared(ctx, homeIconKey(service.URL, override)+"\x00"+serviceConfigHash(service), service.URL, override)
}

// HomeIconVersion returns a short hash of the service URL, icon path and the
// override file's mtime. The page appends it to /api/home-icon so an edited
// icon bypasses the browser's max-age cache; the endpoint ignores it.
func (uc *HomeIconUseCase) HomeIconVersion(ctx context.Context, service entity.HomeService) string {
	mtime := ""
	if service.Icon != "" && uc.source != nil {
		if path, ok := resolveUnderDir(uc.configDir, service.Icon); ok {
			if t, err := uc.source.LocalIconModTime(ctx, path); err == nil {
				mtime = strconv.FormatInt(t.UnixNano(), 10)
			}
		}
	}
	sum := sha256.Sum256([]byte(service.URL + "\x00" + service.Icon + "\x00" + mtime))
	return hex.EncodeToString(sum[:6])
}

// serviceConfigHash fingerprints a service's own home.toml entry, so any
// edit to it bypasses remembered failures and in-flight work immediately.
func serviceConfigHash(service entity.HomeService) string {
	data, err := json.Marshal(service)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:8])
}

func homeIconKey(serviceURL, overridePath string) string {
	return serviceURL + "\x00" + overridePath
}

// resolveUnderDir joins rel to dir and reports whether the cleaned result
// stays strictly inside dir.
func resolveUnderDir(dir, rel string) (string, bool) {
	if dir == "" || filepath.IsAbs(rel) {
		return "", false
	}
	base := filepath.Clean(dir)
	joined := filepath.Join(base, rel)
	if !strings.HasPrefix(joined, base+string(filepath.Separator)) {
		return "", false
	}
	return joined, true
}

// ResolveHomeServiceIcon returns a 128px PNG for a home service. A non-empty
// localOverridePath (absolute, already validated) wins; if it cannot be used,
// remote discovery runs. Returns ErrHomeIconNotFound when nothing works.
// Failures are remembered for HomeIconNegativeTTL and concurrent calls for
// the same inputs share one resolution.
func (uc *HomeIconUseCase) ResolveHomeServiceIcon(
	ctx context.Context,
	serviceURL string,
	localOverridePath string,
) (pngBytes []byte, contentType string, err error) {
	return uc.resolveShared(ctx, homeIconKey(serviceURL, localOverridePath), serviceURL, localOverridePath)
}

// resolveShared applies the negative cache and singleflight for key.
func (uc *HomeIconUseCase) resolveShared(
	ctx context.Context,
	key, serviceURL, localOverridePath string,
) (pngBytes []byte, contentType string, err error) {
	uc.mu.Lock()
	if failedAt, ok := uc.failures[key]; ok {
		if uc.now().Before(failedAt.Add(HomeIconNegativeTTL)) {
			uc.mu.Unlock()
			return nil, "", fmt.Errorf("%w: recent failure", ErrHomeIconNotFound)
		}
		delete(uc.failures, key)
	}
	call, ok := uc.inflight[key]
	if !ok {
		call = &homeIconCall{done: make(chan struct{})}
		uc.inflight[key] = call
		// The shared work must not die with the first caller's request:
		// other tiles may be waiting, and a canceled fetch would otherwise
		// be remembered as a failure.
		workCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), homeIconResolveTimeout)
		go func() {
			defer cancel()
			call.png, _, call.err = uc.resolve(workCtx, serviceURL, localOverridePath)
			uc.mu.Lock()
			delete(uc.inflight, key)
			if errors.Is(call.err, ErrHomeIconNotFound) {
				uc.failures[key] = uc.now()
			}
			uc.mu.Unlock()
			close(call.done)
		}()
	}
	uc.mu.Unlock()
	select {
	case <-call.done:
		if call.err != nil {
			return nil, "", call.err
		}
		return call.png, homeIconContentType, nil
	case <-ctx.Done():
		return nil, "", ctx.Err()
	}
}

func (uc *HomeIconUseCase) resolve(
	ctx context.Context,
	serviceURL string,
	localOverridePath string,
) (pngBytes []byte, contentType string, err error) {
	log := logging.FromContext(ctx)
	if localOverridePath != "" {
		img, readErr := uc.source.ReadLocalIcon(ctx, localOverridePath)
		if readErr == nil {
			out, convErr := uc.normalize(ctx, img)
			if convErr == nil {
				return out, homeIconContentType, nil
			}
			readErr = convErr
		}
		log.Debug().Err(readErr).Str("path", localOverridePath).Msg("home icon override unusable, falling back to discovery")
	}
	if serviceURL == "" {
		return nil, "", ErrHomeIconNotFound
	}
	if uc.cache != nil {
		if cached, storedAt, ok := uc.cache.Get(ctx, serviceURL); ok && storedAt.Add(uc.ttl).After(uc.now()) {
			return cached, homeIconContentType, nil
		}
	}
	candidates, err := uc.source.DiscoverIconURLs(ctx, serviceURL)
	if err != nil {
		log.Debug().Err(err).Str("url", serviceURL).Msg("home icon discovery failed")
	}
	for _, iconURL := range candidates {
		if ctx.Err() != nil {
			return nil, "", ctx.Err()
		}
		img, fetchErr := uc.source.FetchIcon(ctx, serviceURL, iconURL)
		if fetchErr != nil {
			log.Debug().Err(fetchErr).Str("icon", iconURL).Msg("home icon candidate fetch failed")
			continue
		}
		out, convErr := uc.normalize(ctx, img)
		if convErr != nil {
			log.Debug().Err(convErr).Str("icon", iconURL).Msg("home icon candidate unusable")
			continue
		}
		if uc.cache != nil {
			if putErr := uc.cache.Put(ctx, serviceURL, out); putErr != nil {
				log.Debug().Err(putErr).Msg("home icon cache write failed")
			}
		}
		return out, homeIconContentType, nil
	}
	return nil, "", ErrHomeIconNotFound
}

func (uc *HomeIconUseCase) normalize(ctx context.Context, img *appport.HomeIconImage) ([]byte, error) {
	if img == nil || len(img.Bytes) == 0 {
		return nil, fmt.Errorf("%w: empty icon", ErrHomeIconNotFound)
	}
	converted, err := uc.converter.Convert(ctx, img.Bytes, img.ContentType, []int{HomeIconSize})
	if err != nil {
		return nil, err
	}
	if converted == nil || len(converted.SizedPNG[HomeIconSize]) == 0 {
		return nil, fmt.Errorf("%w: conversion produced no %dpx png", ErrHomeIconNotFound, HomeIconSize)
	}
	return converted.SizedPNG[HomeIconSize], nil
}
