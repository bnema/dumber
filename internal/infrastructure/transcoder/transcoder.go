package transcoder

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/infrastructure/config"
)

// Compile-time check: Transcoder implements port.MediaTranscoder.
var _ port.MediaTranscoder = (*Transcoder)(nil)

// Transcoder implements port.MediaTranscoder using GPU-accelerated FFmpeg
// pipelines. It probes hardware capabilities at creation time and manages
// a bounded pool of concurrent transcode sessions.
type Transcoder struct {
	cfg    config.TranscodingConfig
	hwCaps port.HWCapabilities
	pool   *sessionPool
	logger zerolog.Logger
}

// New creates a Transcoder. It probes the GPU at creation time using the
// configured hardware acceleration preference. If no compatible GPU
// encoder is found, Available() will return false and Start() will fail.
func New(cfg config.TranscodingConfig, logger *zerolog.Logger) *Transcoder {
	maxConc := cfg.MaxConcurrent
	if maxConc <= 0 {
		maxConc = 2
	}

	quality := cfg.Quality
	if quality == "" {
		quality = "medium"
	}
	cfg.Quality = quality

	hwaccel := cfg.HWAccel
	if hwaccel == "" {
		hwaccel = "auto"
	}

	l := logger.With().Str("component", "transcoder").Logger()

	hwCaps := ProbeGPU(hwaccel, &l)
	if len(hwCaps.Encoders) > 0 {
		l.Info().
			Str("api", hwCaps.API).
			Strs("encoders", hwCaps.Encoders).
			Strs("decoders", hwCaps.Decoders).
			Int("max_concurrent", maxConc).
			Msg("GPU transcoding available")
	} else {
		l.Warn().Msg("no GPU encoder found, transcoding will be unavailable")
	}

	return &Transcoder{
		cfg:    cfg,
		hwCaps: hwCaps,
		pool:   newSessionPool(maxConc),
		logger: l,
	}
}

// Available returns true if a compatible GPU encoder was found during
// hardware probing.
func (t *Transcoder) Available() bool {
	return len(t.hwCaps.Encoders) > 0
}

// Capabilities returns the detected GPU hardware info.
func (t *Transcoder) Capabilities() port.HWCapabilities {
	return t.hwCaps
}

// Start begins a transcode session. It spawns a goroutine running the
// FFmpeg pipeline and returns a TranscodeSession whose Read() streams
// the transcoded WebM output. The session is automatically removed from
// the pool when the pipeline completes or the context is cancelled.
func (t *Transcoder) Start(ctx context.Context, sourceURL string, headers map[string]string) (port.TranscodeSession, error) {
	if !t.Available() {
		return nil, errors.New("transcoder: no GPU encoder available")
	}

	// Create a cancellable context for this session.
	sessionCtx, cancel := context.WithCancel(ctx)

	pr, pw := io.Pipe()

	id := uuid.NewString()
	s := &session{
		id:        id,
		pr:        pr,
		cancel:    cancel,
		createdAt: time.Now(),
	}
	s.lastRead.Store(time.Now().Unix())

	// Check capacity and register before spawning the goroutine.
	if err := t.pool.add(s); err != nil {
		cancel()
		pr.Close()
		pw.Close()
		return nil, fmt.Errorf("transcoder: %w", err)
	}

	p := newPipeline(t.hwCaps, sourceURL, headers, t.cfg.Quality, pw)

	t.logger.Info().
		Str("session_id", id).
		Str("source", sourceURL).
		Int("active_sessions", t.pool.count()).
		Msg("starting transcode session")

	go func() {
		p.run(sessionCtx)
		t.pool.remove(id)
		t.logger.Info().
			Str("session_id", id).
			Msg("transcode session finished")
	}()

	return s, nil
}

// Close shuts down all active transcode sessions.
func (t *Transcoder) Close() {
	t.logger.Info().
		Int("active_sessions", t.pool.count()).
		Msg("closing all transcode sessions")
	t.pool.closeAll()
}
