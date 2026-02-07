package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/application/usecase"
	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/domain/repository"
	"github.com/bnema/dumber/internal/infrastructure/config"
	infralogging "github.com/bnema/dumber/internal/infrastructure/logging"
	corelogging "github.com/bnema/dumber/internal/logging"
	"github.com/rs/zerolog"
)

const (
	recentSessionsLimit = 200
	lockDirPerm         = 0o755
	lockFilePerm        = 0o600
	markerFilePerm      = 0o644
)

type BrowserSession struct {
	Session    *entity.Session
	Logger     zerolog.Logger
	LogCleanup func()

	Persist func(context.Context) error
	End     func(context.Context) error
}

func StartBrowserSession(
	ctx context.Context,
	cfg *config.Config,
	sessionRepo repository.SessionRepository,
	deferPersist bool,
) (*BrowserSession, context.Context, error) {
	log := corelogging.FromContext(ctx)
	if sessionRepo == nil {
		return nil, ctx, errors.New("session repository is nil")
	}

	lockDir := cfg.Logging.LogDir
	if lockDir == "" {
		if dir, err := config.GetLogDir(); err == nil {
			lockDir = dir
		}
	}

	sessionLoggerAdapter := infralogging.NewSessionLoggerAdapter()
	sessionUC := usecase.NewManageSessionUseCase(sessionRepo, sessionLoggerAdapter)
	cleanupUC := usecase.NewCleanupSessionsUseCase(sessionRepo)

	now := time.Now()
	session := &entity.Session{
		ID:        entity.SessionID(corelogging.GenerateSessionID()),
		Type:      entity.SessionTypeBrowser,
		StartedAt: now.UTC(),
	}
	if err := session.Validate(); err != nil {
		return nil, ctx, err
	}

	logger, cleanup, err := sessionLoggerAdapter.CreateLogger(ctx, session.ID, port.SessionLogConfig{
		Level:         cfg.Logging.Level,
		Format:        cfg.Logging.Format,
		TimeFormat:    corelogging.ConsoleTimeFormat,
		LogDir:        cfg.Logging.LogDir,
		WriteToStderr: true,
		EnableFileLog: cfg.Logging.EnableFileLog,
	})
	if err != nil {
		log.Warn().Err(err).Msg("failed to create session logger")
	}

	sessionCtx := corelogging.WithContext(context.Background(), logger)

	var (
		lockMu     sync.Mutex
		lockFile   *os.File
		lockPath   string
		persistMu  sync.Once
		persistErr error
	)

	// persistFn ensures the session is saved exactly once. Using sync.Once is intentional:
	// if the first persist fails, we cache the error and return it on subsequent calls
	// rather than retrying. This prevents duplicate session records and ensures consistent
	// error handling during shutdown sequences where persistFn may be called multiple times.
	persistFn := func(persistCtx context.Context) error {
		persistMu.Do(func() {
			if lockDir != "" {
				abruptSessions, abruptErr := markAbruptExits(lockDir, time.Now().UTC())
				if abruptErr != nil {
					logger.Warn().Err(abruptErr).Msg("failed to check abrupt exit markers")
				} else {
					for _, abruptSessionID := range abruptSessions {
						logger.Warn().
							Str("session_id", abruptSessionID).
							Msg("abrupt-exit marker detected (startup without shutdown)")
					}
				}
			}

			saveCtx := corelogging.WithContext(context.Background(), logger)
			if err := sessionRepo.Save(saveCtx, session); err != nil {
				persistErr = fmt.Errorf("save session: %w", err)
				return
			}

			logger.Info().
				Str("session_id", string(session.ID)).
				Str("type", string(session.Type)).
				Msg("session started")
			if lockDir != "" {
				if err := writeStartupMarker(lockDir, string(session.ID), now.UTC()); err != nil {
					logger.Warn().Err(err).Str("session_id", string(session.ID)).Msg("failed to write startup marker")
				}
			}

			lf, lp, lockErr := lockSessionFile(lockDir, session.ID)
			if lockErr != nil {
				// Don’t fail startup if locking fails; it only affects stale-session cleanup.
				logger.Warn().Err(lockErr).Msg("failed to acquire session lock")
			} else {
				lockMu.Lock()
				lockFile = lf
				lockPath = lp
				lockMu.Unlock()
			}

			// Defer stale session cleanup and old session pruning to background.
			// This avoids blocking startup for non-critical housekeeping tasks.
			runSessionCleanupAsync(persistCtx, sessionUC, cleanupUC, cfg, lockDir, log)
		})
		return persistErr
	}

	endFn := func(endCtx context.Context) error {
		_ = persistFn(endCtx)
		endErr := sessionUC.EndSession(endCtx, session.ID, time.Now())

		lockMu.Lock()
		lf := lockFile
		lp := lockPath
		lockMu.Unlock()

		if lockDir != "" {
			shutdownAt := time.Now().UTC()
			if markerErr := writeShutdownMarker(lockDir, string(session.ID), shutdownAt); markerErr != nil {
				logger.Warn().Err(markerErr).Str("session_id", string(session.ID)).Msg("failed to write clean shutdown marker")
			}
		}

		if lf != nil {
			_ = unlockAndClose(lf)
			_ = os.Remove(lp)
		}
		return endErr
	}

	browserSession := &BrowserSession{
		Session:    session,
		Logger:     logger,
		LogCleanup: cleanup,
		End:        endFn,
	}

	if deferPersist {
		browserSession.Persist = persistFn
	} else {
		if err := persistFn(ctx); err != nil {
			if cleanup != nil {
				cleanup()
			}
			return nil, ctx, err
		}
	}

	return browserSession, sessionCtx, nil
}

func endStaleActiveBrowserSessions(
	ctx context.Context, sessionUC *usecase.ManageSessionUseCase, lockDir string, log *zerolog.Logger,
) error {
	// We only have GetRecentSessions; fetch enough to cover likely actives.
	recent, err := sessionUC.GetRecentSessions(ctx, recentSessionsLimit)
	if err != nil {
		return err
	}

	now := time.Now()
	for _, s := range recent {
		if s == nil || s.Type != entity.SessionTypeBrowser || !s.IsActive() {
			continue
		}

		lockPath := sessionLockPath(lockDir, s.ID)
		if _, statErr := os.Stat(lockPath); statErr != nil {
			// No lock file: conservative behavior, don’t auto-end.
			continue
		}

		f, err := os.OpenFile(lockPath, os.O_RDWR, lockFilePerm)
		if err != nil {
			continue
		}

		locked, lockErr := tryLockExclusiveNonBlocking(f)
		if lockErr != nil {
			_ = f.Close()
			continue
		}
		if !locked {
			// Another process is holding the lock => session still alive.
			_ = f.Close()
			continue
		}

		// We acquired the lock => no running process holds it => stale session.
		if endErr := sessionUC.EndSession(ctx, s.ID, now); endErr != nil {
			if log != nil {
				log.Warn().Err(endErr).Str("session_id", string(s.ID)).Msg("failed to end stale session")
			}
		}
		_ = unlockAndClose(f)
		_ = os.Remove(lockPath)
	}

	return nil
}

func sessionLockPath(lockDir string, sessionID entity.SessionID) string {
	return filepath.Join(lockDir, fmt.Sprintf("session_%s.lock", sessionID))
}

func lockSessionFile(lockDir string, sessionID entity.SessionID) (*os.File, string, error) {
	if lockDir == "" {
		return nil, "", errors.New("lock dir is empty")
	}
	if err := os.MkdirAll(lockDir, lockDirPerm); err != nil {
		return nil, "", err
	}

	lockPath := sessionLockPath(lockDir, sessionID)
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, lockFilePerm)
	if err != nil {
		return nil, "", err
	}

	locked, lockErr := tryLockExclusiveNonBlocking(f)
	if lockErr != nil {
		_ = f.Close()
		return nil, "", lockErr
	}
	if !locked {
		_ = f.Close()
		return nil, "", errors.New("session lock already held")
	}
	return f, lockPath, nil
}

func tryLockExclusiveNonBlocking(f *os.File) (bool, error) {
	if f == nil {
		return false, errors.New("nil file")
	}
	err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, syscall.EWOULDBLOCK) {
		return false, nil
	}
	return false, err
}

func unlockAndClose(f *os.File) error {
	if f == nil {
		return nil
	}
	_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
	return f.Close()
}

func startupMarkerPath(lockDir, sessionID string) string {
	return filepath.Join(lockDir, fmt.Sprintf("session_%s.startup.marker", sessionID))
}

func shutdownMarkerPath(lockDir, sessionID string) string {
	return filepath.Join(lockDir, fmt.Sprintf("session_%s.shutdown.marker", sessionID))
}

func abruptMarkerPath(lockDir, sessionID string) string {
	return filepath.Join(lockDir, fmt.Sprintf("session_%s.abrupt.marker", sessionID))
}

func writeStartupMarker(lockDir, sessionID string, startedAt time.Time) error {
	if lockDir == "" || sessionID == "" {
		return errors.New("startup marker requires lockDir and sessionID")
	}
	if err := os.MkdirAll(lockDir, lockDirPerm); err != nil {
		return err
	}
	content := []byte(startedAt.Format(time.RFC3339Nano) + "\n")
	if err := os.WriteFile(startupMarkerPath(lockDir, sessionID), content, markerFilePerm); err != nil {
		return err
	}
	_ = os.Remove(shutdownMarkerPath(lockDir, sessionID))
	_ = os.Remove(abruptMarkerPath(lockDir, sessionID))
	return nil
}

func writeShutdownMarker(lockDir, sessionID string, endedAt time.Time) error {
	if lockDir == "" || sessionID == "" {
		return errors.New("shutdown marker requires lockDir and sessionID")
	}
	if err := os.MkdirAll(lockDir, lockDirPerm); err != nil {
		return err
	}
	content := []byte(endedAt.Format(time.RFC3339Nano) + "\n")
	if err := os.WriteFile(shutdownMarkerPath(lockDir, sessionID), content, markerFilePerm); err != nil {
		return err
	}
	return nil
}

func markAbruptExits(lockDir string, detectedAt time.Time) ([]string, error) {
	if lockDir == "" {
		return nil, nil
	}
	pattern := filepath.Join(lockDir, "session_*.startup.marker")
	startupMarkers, err := filepath.Glob(pattern)
	if err != nil {
		return nil, err
	}

	abruptSessions := make([]string, 0, len(startupMarkers))
	for _, startupPath := range startupMarkers {
		base := filepath.Base(startupPath)
		if !strings.HasPrefix(base, "session_") || !strings.HasSuffix(base, ".startup.marker") {
			continue
		}
		sessionID := strings.TrimSuffix(strings.TrimPrefix(base, "session_"), ".startup.marker")
		if sessionID == "" {
			continue
		}
		if _, err := os.Stat(shutdownMarkerPath(lockDir, sessionID)); err == nil {
			continue
		} else if !os.IsNotExist(err) {
			return abruptSessions, err
		}
		if _, err := os.Stat(abruptMarkerPath(lockDir, sessionID)); err == nil {
			continue
		} else if !os.IsNotExist(err) {
			return abruptSessions, err
		}

		payload := []byte(fmt.Sprintf("detected_at=%s\nstartup_marker=%s\n", detectedAt.Format(time.RFC3339Nano), startupPath))
		if err := os.WriteFile(abruptMarkerPath(lockDir, sessionID), payload, markerFilePerm); err != nil {
			return abruptSessions, err
		}
		abruptSessions = append(abruptSessions, sessionID)
	}

	return abruptSessions, nil
}

// runSessionCleanupAsync performs stale session cleanup and old session pruning
// in a background goroutine. This avoids blocking startup for non-critical tasks.
// Uses context.Background() to ensure cleanup completes even if startup context is canceled.
func runSessionCleanupAsync(
	startupCtx context.Context,
	sessionUC *usecase.ManageSessionUseCase,
	cleanupUC *usecase.CleanupSessionsUseCase,
	cfg *config.Config,
	lockDir string,
	log *zerolog.Logger,
) {
	// Silence unused parameter warning - startupCtx is intentionally unused.
	_ = startupCtx

	go func() {
		// Use a detached background context instead of the startup context:
		// session cleanup is critical and must run to completion even if the
		// startup context is canceled or times out.
		bgCtx := context.Background()

		// End stale active sessions (orphaned from crashed processes)
		if lockDir != "" {
			if err := endStaleActiveBrowserSessions(bgCtx, sessionUC, lockDir, log); err != nil {
				if log != nil {
					log.Warn().Err(err).Msg("background: failed to mark stale sessions ended")
				}
			}
		}

		// Clean up old exited sessions based on config limits
		cleanupOutput, err := cleanupUC.Execute(bgCtx, usecase.CleanupSessionsInput{
			MaxExitedSessions:       cfg.Session.MaxExitedSessions,
			MaxExitedSessionAgeDays: cfg.Session.MaxExitedSessionAgeDays,
		})
		if err != nil {
			if log != nil {
				log.Warn().Err(err).Msg("background: failed to cleanup old sessions")
			}
		} else if cleanupOutput.TotalDeleted > 0 {
			if log != nil {
				log.Info().Int64("deleted", cleanupOutput.TotalDeleted).Msg("background: cleaned up old sessions")
			}
		}
	}()
}
