package usecase

import (
	"context"
	"fmt"
	"time"

	appport "bnema.dev/dumber/internal/application/port"
	"bnema.dev/dumber/internal/domain/favicon"
	"bnema.dev/dumber/internal/logging"
)

// HomeIconSize is the square size of home service icons.
const HomeIconSize = 128

// ErrHomeIconNotFound reports that no usable icon exists for a home service.
var ErrHomeIconNotFound = appport.ErrHomeIconNotFound

const homeIconContentType = "image/png"

// HomeIconDeps wires the home service icon use case.
type HomeIconDeps struct {
	Source    appport.HomeIconSource
	Converter appport.FaviconImageConverter
	Cache     appport.HomeIconCache
	Now       func() time.Time
	TTL       time.Duration
}

// HomeIconUseCase resolves the best icon for home page service tiles.
type HomeIconUseCase struct {
	source    appport.HomeIconSource
	converter appport.FaviconImageConverter
	cache     appport.HomeIconCache
	now       func() time.Time
	ttl       time.Duration
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
		now:       deps.Now,
		ttl:       deps.TTL,
	}
}

// ResolveHomeServiceIcon returns a 128px PNG for a home service. A non-empty
// localOverridePath (absolute, already validated) wins; if it cannot be used,
// remote discovery runs. Returns ErrHomeIconNotFound when nothing works.
func (uc *HomeIconUseCase) ResolveHomeServiceIcon(
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
		img, fetchErr := uc.source.FetchIcon(ctx, iconURL)
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
