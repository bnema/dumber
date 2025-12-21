package usecase

import (
	"context"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/logging"
)

const (
	defaultFindDebounce = 100 * time.Millisecond
	maxFindMatchCount   = 1000
	findHistoryLimit    = 20
)

type caseMode int

const (
	caseModeAuto caseMode = iota
	caseModeManual
)

// FindState represents the current find-in-page state.
type FindState struct {
	Query          string
	MatchCount     uint
	CurrentIndex   uint
	NotFound       bool
	CaseSensitive  bool
	AtWordStarts   bool
	WrapAround     bool
	HighlightOn    bool
	HistoryEnabled bool
}

// FindInPageUseCase manages find-in-page business logic.
type FindInPageUseCase struct {
	mu  sync.RWMutex
	ctx context.Context

	controller port.FindController
	signalIDs  []uint32

	query        string
	matchCount   uint
	currentIndex uint
	notFound     bool
	navigating   bool // true during SearchNext/SearchPrev to ignore found-text signals

	caseMode      caseMode
	caseSensitive bool
	atWordStarts  bool
	wrapAround    bool
	highlightOn   bool

	history      []string
	historyIndex int

	debounceDelay time.Duration
	debounceTimer *time.Timer
	debounceMu    sync.Mutex

	onStateChange func(state FindState)
}

// NewFindInPageUseCase creates a new find-in-page use case.
func NewFindInPageUseCase(ctx context.Context) *FindInPageUseCase {
	return &FindInPageUseCase{
		ctx:           ctx,
		caseMode:      caseModeAuto,
		wrapAround:    true,
		highlightOn:   true,
		historyIndex:  -1,
		debounceDelay: defaultFindDebounce,
	}
}

// Bind attaches a FindController and connects its signals.
func (uc *FindInPageUseCase) Bind(controller port.FindController) {
	log := logging.FromContext(uc.ctx).With().Str("component", "find").Logger()

	uc.mu.Lock()
	defer uc.mu.Unlock()

	uc.disconnectSignalsLocked()
	uc.controller = controller
	uc.signalIDs = uc.signalIDs[:0]

	if controller == nil {
		log.Debug().Msg("bind called with nil controller")
		return
	}

	uc.signalIDs = append(uc.signalIDs, controller.OnFoundText(func(matchCount uint) {
		uc.onFoundText(matchCount)
	}))
	uc.signalIDs = append(uc.signalIDs, controller.OnFailedToFindText(func() {
		uc.onFailedToFind()
	}))
	uc.signalIDs = append(uc.signalIDs, controller.OnCountedMatches(func(matchCount uint) {
		uc.onCountedMatches(matchCount)
	}))

	log.Debug().Int("signalCount", len(uc.signalIDs)).Msg("controller bound")
}

// Unbind detaches the FindController and disconnects signals.
func (uc *FindInPageUseCase) Unbind() {
	log := logging.FromContext(uc.ctx).With().Str("component", "find").Logger()

	uc.mu.Lock()
	defer uc.mu.Unlock()
	uc.disconnectSignalsLocked()
	uc.controller = nil
	log.Debug().Msg("controller unbound")
}

// SetOnStateChange sets a callback for state changes.
func (uc *FindInPageUseCase) SetOnStateChange(fn func(state FindState)) {
	uc.mu.Lock()
	defer uc.mu.Unlock()
	uc.onStateChange = fn
}

// SetQuery updates the search query with debouncing.
func (uc *FindInPageUseCase) SetQuery(text string) {
	log := logging.FromContext(uc.ctx).With().Str("component", "find").Logger()

	uc.mu.Lock()
	uc.query = text
	uc.notFound = false
	uc.currentIndex = 0
	uc.navigating = false // New search, accept found-text signals again
	uc.mu.Unlock()

	log.Debug().Str("query", text).Msg("query set")

	if strings.TrimSpace(text) == "" {
		uc.finishSearch()
		uc.notifyState()
		return
	}

	uc.debounceMu.Lock()
	if uc.debounceTimer != nil {
		uc.debounceTimer.Stop()
	}
	uc.debounceTimer = time.AfterFunc(uc.debounceDelay, func() {
		uc.executeSearch(text)
	})
	uc.debounceMu.Unlock()

	uc.notifyState()
}

// SearchNext selects the next match.
func (uc *FindInPageUseCase) SearchNext() {
	log := logging.FromContext(uc.ctx).With().Str("component", "find").Logger()

	uc.mu.Lock()
	controller := uc.controller
	matchCount := uc.matchCount
	query := uc.query
	uc.mu.Unlock()

	log.Debug().
		Str("query", query).
		Uint("matchCount", matchCount).
		Bool("hasController", controller != nil).
		Msg("SearchNext called")

	if controller == nil || strings.TrimSpace(query) == "" {
		log.Debug().Msg("SearchNext: no controller or empty query")
		return
	}
	if matchCount == 0 {
		log.Debug().Msg("SearchNext: no matches, executing fresh search")
		uc.flushSearch(query)
		return
	}

	uc.mu.Lock()
	uc.navigating = true
	uc.mu.Unlock()

	log.Debug().Msg("SearchNext: calling controller.SearchNext()")
	controller.SearchNext()
	uc.advanceIndex(1)
}

// SearchPrevious selects the previous match.
func (uc *FindInPageUseCase) SearchPrevious() {
	log := logging.FromContext(uc.ctx).With().Str("component", "find").Logger()

	uc.mu.Lock()
	controller := uc.controller
	matchCount := uc.matchCount
	query := uc.query
	uc.mu.Unlock()

	log.Debug().
		Str("query", query).
		Uint("matchCount", matchCount).
		Bool("hasController", controller != nil).
		Msg("SearchPrevious called")

	if controller == nil || strings.TrimSpace(query) == "" {
		return
	}
	if matchCount == 0 {
		uc.flushSearch(query)
		return
	}

	uc.mu.Lock()
	uc.navigating = true
	uc.mu.Unlock()

	log.Debug().Msg("SearchPrevious: calling controller.SearchPrevious()")
	controller.SearchPrevious()
	uc.advanceIndex(-1)
}

// Finish clears the current search and highlights.
func (uc *FindInPageUseCase) Finish() {
	uc.finishSearch()
}

// SetCaseSensitiveEnabled toggles manual case sensitivity.
func (uc *FindInPageUseCase) SetCaseSensitiveEnabled(enabled bool) {
	uc.mu.Lock()
	if enabled {
		uc.caseMode = caseModeManual
		uc.caseSensitive = true
	} else {
		uc.caseMode = caseModeAuto
		uc.caseSensitive = false
	}
	uc.mu.Unlock()

	uc.executeSearch(uc.currentQuery())
}

// SetAtWordStarts toggles word-start matching.
func (uc *FindInPageUseCase) SetAtWordStarts(enabled bool) {
	uc.mu.Lock()
	uc.atWordStarts = enabled
	uc.mu.Unlock()

	uc.executeSearch(uc.currentQuery())
}

// SetWrapAround toggles wrap-around searching.
func (uc *FindInPageUseCase) SetWrapAround(enabled bool) {
	uc.mu.Lock()
	uc.wrapAround = enabled
	uc.mu.Unlock()

	uc.executeSearch(uc.currentQuery())
}

// SetHighlightEnabled toggles match highlighting.
func (uc *FindInPageUseCase) SetHighlightEnabled(enabled bool) {
	uc.mu.Lock()
	uc.highlightOn = enabled
	uc.mu.Unlock()

	uc.executeSearch(uc.currentQuery())
}

// History returns a copy of the search history.
func (uc *FindInPageUseCase) History() []string {
	uc.mu.RLock()
	defer uc.mu.RUnlock()

	history := make([]string, len(uc.history))
	copy(history, uc.history)
	return history
}

// PrevHistory selects the previous history item.
func (uc *FindInPageUseCase) PrevHistory() (string, bool) {
	uc.mu.Lock()
	defer uc.mu.Unlock()

	if len(uc.history) == 0 {
		return "", false
	}
	if uc.historyIndex < 0 {
		uc.historyIndex = 0
	} else if uc.historyIndex < len(uc.history)-1 {
		uc.historyIndex++
	}
	return uc.history[uc.historyIndex], true
}

// NextHistory selects the next history item.
func (uc *FindInPageUseCase) NextHistory() (string, bool) {
	uc.mu.Lock()
	defer uc.mu.Unlock()

	if len(uc.history) == 0 || uc.historyIndex < 0 {
		return "", false
	}
	if uc.historyIndex > 0 {
		uc.historyIndex--
	} else {
		uc.historyIndex = -1
		return "", true
	}
	return uc.history[uc.historyIndex], true
}

func (uc *FindInPageUseCase) executeSearch(text string) {
	log := logging.FromContext(uc.ctx).With().Str("component", "find").Logger()

	if strings.TrimSpace(text) == "" {
		uc.finishSearch()
		uc.notifyState()
		return
	}

	uc.mu.RLock()
	controller := uc.controller
	uc.mu.RUnlock()

	if controller == nil {
		log.Debug().Msg("executeSearch: no controller")
		return
	}

	uc.mu.RLock()
	opts := uc.buildOptionsLocked(text)
	highlight := uc.highlightOn
	uc.mu.RUnlock()

	if !highlight {
		controller.SearchFinish()
		uc.setMatchCount(0, true)
		return
	}

	log.Debug().
		Str("text", text).
		Bool("caseInsensitive", opts.CaseInsensitive).
		Bool("wrapAround", opts.WrapAround).
		Msg("executing search")

	controller.Search(text, opts, maxFindMatchCount)
	uc.addHistory(text)
	uc.notifyState()
}

func (uc *FindInPageUseCase) flushSearch(text string) {
	uc.debounceMu.Lock()
	if uc.debounceTimer != nil {
		uc.debounceTimer.Stop()
	}
	uc.debounceTimer = nil
	uc.debounceMu.Unlock()

	uc.executeSearch(text)
}

func (uc *FindInPageUseCase) finishSearch() {
	uc.debounceMu.Lock()
	if uc.debounceTimer != nil {
		uc.debounceTimer.Stop()
	}
	uc.debounceMu.Unlock()

	uc.mu.Lock()
	controller := uc.controller
	uc.matchCount = 0
	uc.currentIndex = 0
	uc.notFound = false
	uc.mu.Unlock()

	if controller != nil {
		controller.SearchFinish()
	}
	uc.notifyState()
}

func (uc *FindInPageUseCase) buildOptionsLocked(text string) port.FindOptions {
	caseInsensitive := true
	if uc.caseMode == caseModeManual {
		caseInsensitive = !uc.caseSensitive
	} else {
		caseInsensitive = !containsUpper(text)
	}

	return port.FindOptions{
		CaseInsensitive: caseInsensitive,
		AtWordStarts:    uc.atWordStarts,
		WrapAround:      uc.wrapAround,
	}
}

func (uc *FindInPageUseCase) currentQuery() string {
	uc.mu.RLock()
	defer uc.mu.RUnlock()
	return uc.query
}

func (uc *FindInPageUseCase) onFoundText(matchCount uint) {
	log := logging.FromContext(uc.ctx).With().Str("component", "find").Logger()

	uc.mu.Lock()
	navigating := uc.navigating
	if navigating {
		// During navigation, WebKit emits found-text with matchCount=1 (the current match)
		// NOT the total count. Ignore it to preserve our actual total.
		log.Debug().
			Uint("signalMatchCount", matchCount).
			Uint("preservedTotal", uc.matchCount).
			Msg("onFoundText (navigating, ignoring)")
		uc.mu.Unlock()
		return
	}
	uc.mu.Unlock()

	log.Debug().
		Uint("matchCount", matchCount).
		Msg("onFoundText")

	uc.setMatchCount(matchCount, false)
}

func (uc *FindInPageUseCase) onFailedToFind() {
	log := logging.FromContext(uc.ctx).With().Str("component", "find").Logger()

	uc.mu.Lock()
	navigating := uc.navigating
	if navigating {
		log.Debug().Msg("onFailedToFind (navigating, ignoring)")
		uc.mu.Unlock()
		return
	}
	uc.matchCount = 0
	uc.currentIndex = 0
	uc.notFound = true
	uc.mu.Unlock()

	log.Debug().Msg("onFailedToFind")
	uc.notifyState()
}

func (uc *FindInPageUseCase) onCountedMatches(matchCount uint) {
	log := logging.FromContext(uc.ctx).With().Str("component", "find").Logger()
	log.Debug().Uint("matchCount", matchCount).Msg("onCountedMatches")
	uc.setMatchCount(matchCount, true)
}

func (uc *FindInPageUseCase) setMatchCount(matchCount uint, counted bool) {
	uc.mu.Lock()
	uc.matchCount = matchCount
	if matchCount == 0 {
		uc.currentIndex = 0
		uc.notFound = counted
	} else if uc.currentIndex == 0 {
		uc.currentIndex = 1
		uc.notFound = false
	}
	uc.mu.Unlock()
	uc.notifyState()
}

func (uc *FindInPageUseCase) advanceIndex(delta int) {
	log := logging.FromContext(uc.ctx).With().Str("component", "find").Logger()

	uc.mu.Lock()
	defer uc.mu.Unlock()

	if uc.matchCount == 0 {
		return
	}

	index := int(uc.currentIndex)
	if index == 0 {
		index = 1
	}
	index += delta
	if index <= 0 {
		index = int(uc.matchCount)
	} else if index > int(uc.matchCount) {
		index = 1
	}
	uc.currentIndex = uint(index)

	log.Debug().
		Int("delta", delta).
		Uint("newIndex", uc.currentIndex).
		Uint("matchCount", uc.matchCount).
		Msg("advanceIndex")

	uc.notifyStateLocked()
}

func (uc *FindInPageUseCase) addHistory(text string) {
	uc.mu.Lock()
	defer uc.mu.Unlock()

	if strings.TrimSpace(text) == "" {
		return
	}

	// Remove existing entry
	for i, item := range uc.history {
		if item == text {
			uc.history = append(uc.history[:i], uc.history[i+1:]...)
			break
		}
	}

	uc.history = append([]string{text}, uc.history...)
	if len(uc.history) > findHistoryLimit {
		uc.history = uc.history[:findHistoryLimit]
	}
	uc.historyIndex = -1
}

func (uc *FindInPageUseCase) notifyState() {
	uc.mu.RLock()
	defer uc.mu.RUnlock()
	uc.notifyStateLocked()
}

func (uc *FindInPageUseCase) notifyStateLocked() {
	if uc.onStateChange == nil {
		return
	}

	query := uc.query
	caseSensitive := uc.caseSensitive
	if uc.caseMode == caseModeAuto {
		caseSensitive = containsUpper(query)
	}

	state := FindState{
		Query:          query,
		MatchCount:     uc.matchCount,
		CurrentIndex:   uc.currentIndex,
		NotFound:       uc.notFound,
		CaseSensitive:  caseSensitive,
		AtWordStarts:   uc.atWordStarts,
		WrapAround:     uc.wrapAround,
		HighlightOn:    uc.highlightOn,
		HistoryEnabled: len(uc.history) > 0,
	}
	uc.onStateChange(state)
}

func (uc *FindInPageUseCase) disconnectSignalsLocked() {
	if uc.controller == nil {
		uc.signalIDs = uc.signalIDs[:0]
		return
	}

	for _, id := range uc.signalIDs {
		uc.controller.DisconnectSignal(id)
	}
	uc.signalIDs = uc.signalIDs[:0]
}

func containsUpper(text string) bool {
	for _, r := range text {
		if unicode.IsUpper(r) {
			return true
		}
	}
	return false
}
