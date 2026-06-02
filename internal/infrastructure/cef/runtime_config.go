package cef

import (
	"context"
	"errors"

	"github.com/bnema/dumber/internal/application/port"
	domainerrors "github.com/bnema/dumber/internal/domain/errors"
)

var (
	ErrDownloadsUnsupported      = errors.New("cef: downloads are not supported yet")
	ErrRelatedWebViewUnsupported = domainerrors.ErrRelatedWebViewUnsupported
	ErrCookiePolicyUnsupported   = errors.New("cef: non-default cookie policy is not supported yet")
)

type RuntimeConfig struct {
	CEFDir                      string
	LogFile                     string
	LogSeverity                 int32
	RenderStack                 string
	AdaptiveWindowlessFrameRate bool
	WindowlessFrameRate         int32
	WindowlessFrameRateMax      int32
	Input                       RuntimeInputConfig
	EnableAudioHandler          bool
	TraceHandlers               bool
	ApplicationScale            float64
}

type RuntimeInputConfig struct {
	ScrollWheelMultiplier      float64
	ScrollTouchpadMultiplier   float64
	ScrollHorizontalMultiplier float64
	ScrollVerticalMultiplier   float64
	ScrollMaxDelta             int32
}

type HandlerRegistrar func(context.Context, port.WebUIHandlerRouter, port.HandlerDependencies) error

type AccentHandlerRegistrar func(context.Context, port.WebUIHandlerRouter, port.AccentKeyHandler) error

type EngineDependencies struct {
	RegisterHandlers           HandlerRegistrar
	RegisterAccentHandlers     AccentHandlerRegistrar
	CurrentConfigPayload       func() ([]byte, error)
	DefaultConfigPayload       func() ([]byte, error)
	ContextMenuBuilder         port.ContextMenuBuilder
	ContextMenuExecutorFactory port.ContextMenuActionExecutorFactory
	ContextMenuRenderer        ContextMenuRenderer
	Clipboard                  port.Clipboard
	ImageDataResolver          port.ImageDataResolver
}
