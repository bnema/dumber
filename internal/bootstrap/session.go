package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
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
	dirPerm             = 0o755
	filePerm            = 0o644
	pidFileName         = "dumber.pid"
)

// BrowserSession holds the state for a running browser session.
type BrowserSession struct {
	Session    *entity.Session
	Logger     zerolog.Logger
	LogCleanup func()
	LogPath    string

	reportsMu        sync.RWMutex
	crashReportPaths []string

	Persist func(context.Context) error
	End     func(context.Context) error
}

// SetCrashReports stores crash report paths detected at startup.
func (s *BrowserSession) SetCrashReports(paths []string) {
	if s == nil {
		return
	}
	s.reportsMu.Lock()
	defer s.reportsMu.Unlock()
	s.crashReportPaths = append([]string(nil), paths...)
}

// CrashReports returns crash report paths detected at startup.
func (s *BrowserSession) CrashReports() []string {
	if s == nil {
		return nil
	}
	s.reportsMu.RLock()
	defer s.reportsMu.RUnlock()
	return append([]string(nil), s.crashReportPaths...)
}

// StartBrowserSession creates and persists a browser session.
// It checks for previous crashes via PID file, creates the session log,
// and sets up cleanup.
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

	logDir := cfg.Logging.LogDir
	if logDir == "" {
		if dir, err := config.GetLogDir(); err == nil {
			logDir = dir
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

	// Compute and log the session log file path early so the user can
	// copy-paste it from the console output.
	var logPath string
	if logDir != "" {
		logPath = filepath.Join(logDir, corelogging.SessionFilename(string(session.ID)))
		logger.Info().Str("log_file", logPath).Msg("session log file")
	}

	sessionCtx := corelogging.WithContext(context.Background(), logger)

	bs := &BrowserSession{
		Session:    session,
		Logger:     logger,
		LogCleanup: cleanup,
		LogPath:    logPath,
	}

	var persistOnce sync.Once
	var persistErr error

	persistFn := func(persistCtx context.Context) error {
		persistOnce.Do(func() {
			// Check for crash from previous run.
			if logDir != "" {
				reports := detectPreviousCrash(logDir, &logger)
				if len(reports) > 0 {
					bs.SetCrashReports(reports)
				}
			}

			saveCtx := corelogging.WithContext(context.Background(), logger)
			if err := sessionRepo.Save(saveCtx, session); err != nil {
				persistErr = fmt.Errorf("save session: %w", err)
				return
			}

			logger.Info().
				Str("session_id", string(session.ID)).
				Msg("session started")

			// Write PID file so next startup can detect crashes.
			if logDir != "" {
				writePIDFile(logDir, &logger)
			}

			// Background cleanup of old sessions.
			go runSessionCleanup(persistCtx, sessionUC, cleanupUC, cfg, session.ID, log)
		})
		return persistErr
	}

	endFn := func(endCtx context.Context) error {
		_ = persistFn(endCtx)
		endErr := sessionUC.EndSession(endCtx, session.ID, time.Now())

		// Remove PID file on clean shutdown.
		if logDir != "" {
			removePIDFile(logDir)
		}
		return endErr
	}

	bs.End = endFn

	if deferPersist {
		bs.Persist = persistFn
	} else {
		if err := persistFn(ctx); err != nil {
			if cleanup != nil {
				cleanup()
			}
			return nil, ctx, err
		}
	}

	return bs, sessionCtx, nil
}

// ---------------------------------------------------------------------------
// PID file: single file {logDir}/dumber.pid
// ---------------------------------------------------------------------------

func pidFilePath(logDir string) string {
	return filepath.Join(logDir, pidFileName)
}

func writePIDFile(logDir string, logger *zerolog.Logger) {
	p := pidFilePath(logDir)
	content := fmt.Sprintf("%d\n", os.Getpid())
	if err := os.WriteFile(p, []byte(content), filePerm); err != nil && logger != nil {
		logger.Warn().Err(err).Msg("failed to write PID file")
	}
}

func removePIDFile(logDir string) {
	_ = os.Remove(pidFilePath(logDir))
}

// detectPreviousCrash checks the PID file from a previous run.
// If the file exists and the process is dead, the previous session crashed.
// Returns generated crash report paths (if any).
func detectPreviousCrash(logDir string, logger *zerolog.Logger) []string {
	p := pidFilePath(logDir)
	raw, err := os.ReadFile(p)
	if err != nil {
		return nil // No PID file → clean previous exit (or first run).
	}

	pid, parseErr := strconv.Atoi(strings.TrimSpace(string(raw)))
	if parseErr != nil {
		// Corrupt PID file, remove and move on.
		_ = os.Remove(p)
		return nil
	}

	// Check if the process is still running.
	proc, procErr := os.FindProcess(pid)
	if procErr == nil {
		// On Unix, FindProcess always succeeds. Send signal 0 to probe existence.
		if killErr := proc.Signal(syscall.Signal(0)); killErr == nil {
			// Process is alive → another instance running, don't touch PID file.
			if logger != nil {
				logger.Warn().Int("pid", pid).Msg("previous instance still running (PID file exists)")
			}
			return nil
		}
	}

	// Previous process is dead → crash. Generate report from last log.
	_ = os.Remove(p)

	lastLog := findLastSessionLog(logDir)
	if lastLog == "" {
		return nil
	}

	lastSessionID := sessionIDFromLogPath(lastLog)
	if lastSessionID == "" {
		return nil
	}

	if logger != nil {
		logger.Warn().
			Int("crashed_pid", pid).
			Str("session_id", lastSessionID).
			Msg("previous session crashed (stale PID file)")
	}

	reportPath, reportErr := writeCrashReport(logDir, lastSessionID, pid)
	if reportErr != nil {
		if logger != nil {
			logger.Warn().Err(reportErr).Msg("failed to write crash report")
		}
		return nil
	}
	if reportPath == "" {
		return nil
	}
	return []string{reportPath}
}

// findLastSessionLog returns the most recent session_*.log file in logDir.
func findLastSessionLog(logDir string) string {
	matches, err := filepath.Glob(filepath.Join(logDir, "session_*.log"))
	if err != nil || len(matches) == 0 {
		return ""
	}
	var newest string
	var newestMod time.Time
	for _, m := range matches {
		info, statErr := os.Stat(m)
		if statErr != nil {
			continue
		}
		if info.ModTime().After(newestMod) {
			newest = m
			newestMod = info.ModTime()
		}
	}
	return newest
}

func sessionIDFromLogPath(logPath string) string {
	base := filepath.Base(logPath)
	id, ok := corelogging.ParseSessionFilename(base)
	if !ok {
		return ""
	}
	return id
}

// ---------------------------------------------------------------------------
// Background cleanup
// ---------------------------------------------------------------------------

func runSessionCleanup(
	startupCtx context.Context,
	sessionUC *usecase.ManageSessionUseCase,
	cleanupUC *usecase.CleanupSessionsUseCase,
	cfg *config.Config,
	currentSessionID entity.SessionID,
	log *zerolog.Logger,
) {
	bgCtx := context.WithoutCancel(startupCtx)

	// End stale active sessions that have no running process.
	recent, err := sessionUC.GetRecentSessions(bgCtx, recentSessionsLimit)
	if err != nil {
		if log != nil {
			log.Warn().Err(err).Msg("background: failed to list recent sessions")
		}
	} else {
		now := time.Now()
		for _, s := range recent {
			if s != nil &&
				s.ID != currentSessionID &&
				s.Type == entity.SessionTypeBrowser &&
				s.IsActive() {
				_ = sessionUC.EndSession(bgCtx, s.ID, now)
			}
		}
	}

	// Clean up old exited sessions.
	output, err := cleanupUC.Execute(bgCtx, usecase.CleanupSessionsInput{
		MaxExitedSessions:       cfg.Session.MaxExitedSessions,
		MaxExitedSessionAgeDays: cfg.Session.MaxExitedSessionAgeDays,
	})
	if err != nil {
		if log != nil {
			log.Warn().Err(err).Msg("background: failed to cleanup old sessions")
		}
	} else if output.TotalDeleted > 0 && log != nil {
		log.Info().Int64("deleted", output.TotalDeleted).Msg("background: cleaned up old sessions")
	}

	// Prune old marker files left by previous versions.
	sweepLegacyMarkers(cfg.Logging.LogDir, log)
}

// sweepLegacyMarkers removes .startup.marker, .shutdown.marker,
// .abrupt.marker, and .lock files left by the old session system.
func sweepLegacyMarkers(logDir string, log *zerolog.Logger) {
	if logDir == "" {
		return
	}
	patterns := []string{
		"session_*.startup.marker",
		"session_*.shutdown.marker",
		"session_*.abrupt.marker",
		"session_*.lock",
	}
	swept := 0
	for _, pat := range patterns {
		matches, _ := filepath.Glob(filepath.Join(logDir, pat))
		for _, m := range matches {
			if os.Remove(m) == nil {
				swept++
			}
		}
	}
	if swept > 0 && log != nil {
		log.Info().Int("swept", swept).Msg("cleaned up legacy session marker files")
	}
}
