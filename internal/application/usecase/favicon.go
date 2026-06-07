package usecase

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	appport "github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/domain/favicon"
)

var ErrFaviconMiss = appport.ErrFaviconMiss

type ResolvePurpose = appport.FaviconResolvePurpose

const (
	ResolvePurposeUI         = appport.FaviconResolvePurposeUI
	ResolvePurposeSystemview = appport.FaviconResolvePurposeSystemview
	ResolvePurposeRefresh    = appport.FaviconResolvePurposeRefresh
)

type ResolveOptions = appport.FaviconResolveOptions
type ResolvedFavicon = appport.ResolvedFavicon

// SystemviewIconSize is the fixed PNG size served to internal systemview pages.
const SystemviewIconSize = 32

type faviconInvalidatorRegistry struct {
	mu           sync.RWMutex
	fallback     appport.FaviconInvalidators
	invalidators []appport.FaviconInvalidator
}

func (r *faviconInvalidatorRegistry) Register(invalidator appport.FaviconInvalidator) {
	if r == nil || invalidator == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.invalidators = append(r.invalidators, invalidator)
}

func (r *faviconInvalidatorRegistry) InvalidateAll(ctx context.Context, key favicon.Key) error {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	fallback := r.fallback
	invalidators := append([]appport.FaviconInvalidator(nil), r.invalidators...)
	r.mu.RUnlock()
	// The fallback invalidator owns pre-registered invalidation sets and is
	// treated as fatal; dynamic invalidators below are best-effort and joined.
	if fallback != nil {
		if err := fallback.InvalidateAll(ctx, key); err != nil {
			return err
		}
	}
	var errs []error
	for _, invalidator := range invalidators {
		if err := invalidator.Invalidate(ctx, key); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

type FaviconUseCase struct {
	repo         appport.FaviconRepository
	blobs        appport.FaviconBlobStore
	fetcher      appport.FaviconFetcher
	discoverer   appport.FaviconDiscoverer
	converter    appport.FaviconImageConverter
	scheduler    appport.FaviconRefreshScheduler
	invalidators *faviconInvalidatorRegistry
	now          func() time.Time
	ttl          time.Duration
	background   context.Context
}

type FaviconDeps struct {
	Repository   appport.FaviconRepository
	BlobStore    appport.FaviconBlobStore
	Fetcher      appport.FaviconFetcher
	Discoverer   appport.FaviconDiscoverer
	Converter    appport.FaviconImageConverter
	Scheduler    appport.FaviconRefreshScheduler
	Invalidators appport.FaviconInvalidators
	Now          func() time.Time
	TTL          time.Duration
	Background   context.Context
}

func NewFaviconUseCase(deps FaviconDeps) *FaviconUseCase {
	if deps.Now == nil {
		deps.Now = time.Now
	}
	if deps.TTL == 0 {
		deps.TTL = favicon.DefaultTTL
	}
	if deps.Background == nil {
		deps.Background = context.Background()
	}
	registry := &faviconInvalidatorRegistry{}
	if deps.Invalidators != nil {
		registry.fallback = deps.Invalidators
	}
	return &FaviconUseCase{
		repo:         deps.Repository,
		blobs:        deps.BlobStore,
		fetcher:      deps.Fetcher,
		discoverer:   deps.Discoverer,
		converter:    deps.Converter,
		scheduler:    deps.Scheduler,
		invalidators: registry,
		now:          deps.Now,
		ttl:          deps.TTL,
		background:   deps.Background,
	}
}

func (uc *FaviconUseCase) RegisterFaviconInvalidator(invalidator appport.FaviconInvalidator) {
	if uc == nil || uc.invalidators == nil || invalidator == nil {
		return
	}
	uc.invalidators.Register(invalidator)
}

func (uc *FaviconUseCase) Resolve(
	ctx context.Context,
	rawURLOrDomain string,
	size int,
	options ResolveOptions,
) (*appport.ResolvedFavicon, error) {
	keys := favicon.Candidates(rawURLOrDomain)
	if len(keys) == 0 {
		return nil, ErrFaviconMiss
	}

	for _, key := range keys {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		meta, err := uc.repo.Get(ctx, key)
		if err != nil {
			return nil, err
		}
		if meta == nil {
			continue
		}
		bytes, contentType, err := uc.readForSize(ctx, key, size)
		if err != nil {
			if !errors.Is(err, ErrFaviconMiss) {
				return nil, err
			}
			continue
		}
		res := &appport.ResolvedFavicon{Key: key, Bytes: bytes, ContentType: contentType, Metadata: meta}
		if favicon.ShouldRefresh(meta, uc.now(), uc.ttl) && options.ScheduleBackgroundRefresh {
			res.BackgroundRefreshScheduled = uc.scheduleRefresh(key, meta.PageURL)
		}
		return res, nil
	}

	if options.AllowBlockingRefresh {
		if err := uc.refresh(ctx, rawURLOrDomain, true); err != nil {
			return nil, err
		}
		return uc.Resolve(ctx, rawURLOrDomain, size, ResolveOptions{Purpose: options.Purpose})
	}
	if options.ScheduleBackgroundRefresh {
		uc.scheduleRefresh(keys[0], rawURLOrDomain)
	}
	return nil, ErrFaviconMiss
}

func (uc *FaviconUseCase) ResolveSystemviewIcon(ctx context.Context, rawDomain string, size int) (*appport.ResolvedFavicon, error) {
	if size != SystemviewIconSize {
		return nil, ErrFaviconMiss
	}
	return uc.Resolve(ctx, rawDomain, size, ResolveOptions{Purpose: ResolvePurposeSystemview})
}

func (uc *FaviconUseCase) Observe(
	ctx context.Context,
	pageURL, iconURL string,
	bytes []byte,
	source favicon.Source,
	contentType string,
) (*favicon.Metadata, error) {
	key, ok := favicon.CanonicalKey(pageURL)
	if !ok || len(bytes) == 0 {
		return nil, ErrFaviconMiss
	}
	old, err := uc.repo.Get(ctx, key)
	if err != nil {
		return nil, err
	}
	now := uc.now()
	if !favicon.HasContentChanged(old, bytes) {
		if err := uc.repo.UpdateLastChecked(ctx, key, old.ContentHash, now); err != nil {
			return nil, err
		}
		old.LastCheckedAt = now
		return old, nil
	}

	var converted *appport.ConvertedFavicon
	if uc.converter != nil {
		var err error
		converted, err = uc.converter.Convert(ctx, bytes, contentType, []int{32})
		if err != nil {
			return nil, err
		}
	}

	if err := uc.blobs.WriteOriginal(ctx, key, bytes, contentType); err != nil {
		return nil, err
	}
	if err := uc.blobs.RemoveDerived(ctx, key); err != nil {
		return nil, err
	}
	if converted != nil {
		if len(converted.PNG) > 0 {
			if err := uc.blobs.WritePNG(ctx, key, converted.PNG); err != nil {
				return nil, err
			}
		}
		if b := converted.SizedPNG[32]; len(b) > 0 {
			if err := uc.blobs.WriteSizedPNG(ctx, key, 32, b); err != nil {
				return nil, err
			}
		}
	}
	meta := favicon.Metadata{
		Key:           key,
		SourceURL:     iconURL,
		PageURL:       pageURL,
		Source:        source,
		ContentHash:   favicon.Hash(bytes),
		ContentType:   contentType,
		UpdatedAt:     now,
		LastCheckedAt: now,
	}
	if err := uc.repo.Upsert(ctx, meta); err != nil {
		return nil, err
	}
	if err := uc.invalidateCaches(ctx, key); err != nil {
		return nil, err
	}
	return &meta, nil
}

func (uc *FaviconUseCase) RefreshIfStale(ctx context.Context, pageURL string) error {
	return uc.refresh(ctx, pageURL, false)
}

func (uc *FaviconUseCase) RefreshFromIconURLs(ctx context.Context, pageURL string, iconURLs []string) error {
	if uc.fetcher == nil {
		return ErrFaviconMiss
	}
	var lastMiss error
	for _, iconURL := range iconURLs {
		if err := ctx.Err(); err != nil {
			return err
		}
		fetched, err := uc.fetcher.Fetch(ctx, appport.FaviconFetchRequest{PageURL: pageURL, IconURL: iconURL})
		if errors.Is(err, ErrFaviconMiss) {
			lastMiss = err
			continue
		}
		if err != nil {
			return err
		}
		_, err = uc.Observe(ctx, fetched.PageURL, fetched.IconURL, fetched.Bytes, fetched.Source, fetched.ContentType)
		if errors.Is(err, ErrFaviconMiss) {
			lastMiss = err
			continue
		}
		return err
	}
	if lastMiss != nil {
		return lastMiss
	}
	return uc.RefreshIfStale(ctx, pageURL)
}

func (uc *FaviconUseCase) refresh(ctx context.Context, pageURL string, force bool) error {
	keys := favicon.Candidates(pageURL)
	if len(keys) == 0 {
		return ErrFaviconMiss
	}
	if !force {
		fresh, err := uc.anyCandidateFresh(ctx, keys)
		if err != nil {
			return err
		}
		if fresh {
			return nil
		}
	}
	return uc.fetchAndObserve(ctx, pageURL)
}

func (uc *FaviconUseCase) refreshKey(ctx context.Context, key favicon.Key, pageURL string) error {
	meta, err := uc.repo.Get(ctx, key)
	if err != nil {
		return err
	}
	if meta != nil && !favicon.ShouldRefresh(meta, uc.now(), uc.ttl) {
		return nil
	}
	return uc.fetchAndObserve(ctx, pageURL)
}

func (uc *FaviconUseCase) anyCandidateFresh(ctx context.Context, keys []favicon.Key) (bool, error) {
	for _, key := range keys {
		meta, err := uc.repo.Get(ctx, key)
		if err != nil {
			return false, err
		}
		if meta != nil && !favicon.ShouldRefresh(meta, uc.now(), uc.ttl) {
			return true, nil
		}
	}
	return false, nil
}

func (uc *FaviconUseCase) fetchAndObserve(ctx context.Context, pageURL string) error {
	if uc.fetcher == nil {
		return ErrFaviconMiss
	}
	fetched, err := uc.fetcher.Fetch(ctx, appport.FaviconFetchRequest{PageURL: pageURL})
	if err != nil {
		return err
	}
	_, err = uc.Observe(ctx, fetched.PageURL, fetched.IconURL, fetched.Bytes, fetched.Source, fetched.ContentType)
	return err
}

func (uc *FaviconUseCase) EnsureSized(ctx context.Context, key favicon.Key, size int) error {
	if _, _, err := uc.blobs.ReadSizedPNG(ctx, key, size); err == nil {
		return nil
	} else if !errors.Is(err, ErrFaviconMiss) {
		return err
	}
	orig, ct, err := uc.blobs.ReadOriginal(ctx, key)
	if err != nil {
		return err
	}
	converted, err := uc.converter.Convert(ctx, orig, ct, []int{size})
	if err != nil {
		return err
	}
	if len(converted.PNG) > 0 {
		if err := uc.blobs.WritePNG(ctx, key, converted.PNG); err != nil {
			return err
		}
	}
	if b := converted.SizedPNG[size]; len(b) > 0 {
		return uc.blobs.WriteSizedPNG(ctx, key, size, b)
	}
	return ErrFaviconMiss
}

func (uc *FaviconUseCase) Invalidate(ctx context.Context, key favicon.Key) error {
	if uc.blobs != nil {
		if err := uc.blobs.RemoveDerived(ctx, key); err != nil {
			return err
		}
	}
	return uc.invalidateCaches(ctx, key)
}

func (uc *FaviconUseCase) invalidateCaches(ctx context.Context, key favicon.Key) error {
	if uc.invalidators != nil {
		return uc.invalidators.InvalidateAll(ctx, key)
	}
	return nil
}

func (uc *FaviconUseCase) readForSize(ctx context.Context, key favicon.Key, size int) ([]byte, string, error) {
	if size < 0 {
		return nil, "", fmt.Errorf("invalid favicon size: %d", size)
	}
	if size > 0 {
		if err := uc.EnsureSized(ctx, key, size); err != nil {
			return nil, "", err
		}
		return uc.blobs.ReadSizedPNG(ctx, key, size)
	}
	return uc.blobs.ReadPNG(ctx, key)
}

func (uc *FaviconUseCase) scheduleRefresh(key favicon.Key, pageURL string) bool {
	if uc.scheduler == nil {
		return false
	}
	return uc.scheduler.Schedule(uc.background, key, func(ctx context.Context) { _ = uc.refreshKey(ctx, key, pageURL) })
}
