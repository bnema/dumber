package cef

import (
	"context"
	"errors"

	"github.com/bnema/dumber/internal/application/port"
)

var (
	ErrDownloadsUnsupported      = errors.New("cef: downloads are not supported yet")
	ErrRelatedWebViewUnsupported = errors.New("cef: related popup webviews are not supported yet")
	ErrCookiePolicyUnsupported   = errors.New("cef: non-default cookie policy is not supported yet")
)

type RuntimeConfig struct {
	CEFDir                   string
	LogFile                  string
	LogSeverity              int32
	WindowlessFrameRate      int32
	EnableAudioHandler       bool
	EnableContextMenuHandler bool
	TraceHandlers            bool
}

type TranscodingRuntimeConfig struct {
	Enabled       bool
	HWAccel       string
	MaxConcurrent int
	Quality       string
}

type HandlerRegistrar func(context.Context, port.WebUIHandlerRouter, port.HandlerDependencies) error

type AccentHandlerRegistrar func(context.Context, port.WebUIHandlerRouter, port.AccentKeyHandler) error

type MediaClassifier struct {
	IsProprietaryVideoMIME     func(string) bool
	IsOpenVideoMIME            func(string) bool
	IsStreamingManifestMIME    func(string) bool
	IsStreamingManifestURL     func(string) bool
	IsEagerTranscodeURL        func(string) bool
	ParseSyntheticTranscodeURL func(string) (string, string, string, bool)
}

func (m MediaClassifier) normalize() MediaClassifier {
	if m.IsProprietaryVideoMIME == nil {
		m.IsProprietaryVideoMIME = func(string) bool { return false }
	}
	if m.IsOpenVideoMIME == nil {
		m.IsOpenVideoMIME = func(string) bool { return false }
	}
	if m.IsStreamingManifestMIME == nil {
		m.IsStreamingManifestMIME = func(string) bool { return false }
	}
	if m.IsStreamingManifestURL == nil {
		m.IsStreamingManifestURL = func(string) bool { return false }
	}
	if m.IsEagerTranscodeURL == nil {
		m.IsEagerTranscodeURL = func(string) bool { return false }
	}
	if m.ParseSyntheticTranscodeURL == nil {
		m.ParseSyntheticTranscodeURL = func(string) (string, string, string, bool) { return "", "", "", false }
	}
	return m
}

type EngineDependencies struct {
	RegisterHandlers       HandlerRegistrar
	RegisterAccentHandlers AccentHandlerRegistrar
	CurrentConfigPayload   func() ([]byte, error)
	DefaultConfigPayload   func() ([]byte, error)
	MediaClassifier        MediaClassifier
}
