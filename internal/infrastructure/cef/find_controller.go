package cef

import (
	"sync"

	purecef "github.com/bnema/purego-cef/cef"

	"github.com/bnema/dumber/internal/application/port"
)

// Compile-time interface check.
var _ port.FindController = (*cefFindController)(nil)

// cefFindController implements port.FindController using CEF's BrowserHost.Find API.
type cefFindController struct {
	mu        sync.RWMutex
	host      purecef.BrowserHost
	searchTxt string
	matchCase int32 // cached from most recent Search call

	// counting tracks whether the current search is a CountMatches request
	// so that handleFindResult dispatches to the right callbacks.
	counting bool

	// Signal callbacks keyed by unique IDs.
	nextSignalID uint
	onFound      map[uint]func(matchCount uint)
	onFailed     map[uint]func()
	onCounted    map[uint]func(matchCount uint)
}

func newFindController() *cefFindController {
	return &cefFindController{
		onFound:   make(map[uint]func(matchCount uint)),
		onFailed:  make(map[uint]func()),
		onCounted: make(map[uint]func(matchCount uint)),
	}
}

func (fc *cefFindController) setHost(host purecef.BrowserHost) {
	fc.mu.Lock()
	fc.host = host
	fc.mu.Unlock()
}

// Search starts a new find-in-page search.
func (fc *cefFindController) Search(text string, opts port.FindOptions, _ uint) {
	fc.mu.Lock()
	fc.searchTxt = text
	fc.counting = false
	mc := matchCaseFromOpts(opts)
	fc.matchCase = mc
	host := fc.host
	fc.mu.Unlock()

	if host == nil || text == "" {
		return
	}
	host.Find(text, 1, mc, 0) // forward=1, findnext=0 (new search)
}

// CountMatches starts a search whose results are dispatched to onCounted callbacks.
func (fc *cefFindController) CountMatches(text string, opts port.FindOptions, _ uint) {
	fc.mu.Lock()
	fc.searchTxt = text
	fc.counting = true
	mc := matchCaseFromOpts(opts)
	fc.matchCase = mc
	host := fc.host
	fc.mu.Unlock()

	if host == nil || text == "" {
		return
	}
	host.Find(text, 1, mc, 0)
}

// SearchNext continues the search in the forward direction.
func (fc *cefFindController) SearchNext() {
	fc.mu.RLock()
	host := fc.host
	text := fc.searchTxt
	mc := fc.matchCase
	fc.mu.RUnlock()

	if host == nil || text == "" {
		return
	}
	host.Find(text, 1, mc, 1) // forward=1, findnext=1
}

// SearchPrevious continues the search in the backward direction.
func (fc *cefFindController) SearchPrevious() {
	fc.mu.RLock()
	host := fc.host
	text := fc.searchTxt
	mc := fc.matchCase
	fc.mu.RUnlock()

	if host == nil || text == "" {
		return
	}
	host.Find(text, 0, mc, 1) // forward=0, findnext=1
}

// SearchFinish stops the current search and clears the highlight.
func (fc *cefFindController) SearchFinish() {
	fc.mu.Lock()
	host := fc.host
	fc.searchTxt = ""
	fc.mu.Unlock()

	if host != nil {
		host.StopFinding(1)
	}
}

// GetSearchText returns the current search text.
func (fc *cefFindController) GetSearchText() string {
	fc.mu.RLock()
	defer fc.mu.RUnlock()
	return fc.searchTxt
}

// OnFoundText registers a callback fired when matches are found.
func (fc *cefFindController) OnFoundText(callback func(matchCount uint)) uint {
	if callback == nil {
		return 0
	}
	fc.mu.Lock()
	defer fc.mu.Unlock()
	fc.nextSignalID++
	id := fc.nextSignalID
	fc.onFound[id] = callback
	return id
}

// OnFailedToFindText registers a callback fired when no matches are found.
func (fc *cefFindController) OnFailedToFindText(callback func()) uint {
	if callback == nil {
		return 0
	}
	fc.mu.Lock()
	defer fc.mu.Unlock()
	fc.nextSignalID++
	id := fc.nextSignalID
	fc.onFailed[id] = callback
	return id
}

// OnCountedMatches registers a callback fired with the final match count.
func (fc *cefFindController) OnCountedMatches(callback func(matchCount uint)) uint {
	if callback == nil {
		return 0
	}
	fc.mu.Lock()
	defer fc.mu.Unlock()
	fc.nextSignalID++
	id := fc.nextSignalID
	fc.onCounted[id] = callback
	return id
}

// DisconnectSignal removes a previously registered callback.
func (fc *cefFindController) DisconnectSignal(id uint) {
	if id == 0 {
		return
	}
	fc.mu.Lock()
	defer fc.mu.Unlock()
	delete(fc.onFound, id)
	delete(fc.onFailed, id)
	delete(fc.onCounted, id)
}

// handleFindResult is called by the FindHandler when CEF reports find results.
func (fc *cefFindController) handleFindResult(_, count, _, finalUpdate int32) {
	fc.mu.RLock()
	counting := fc.counting

	// Snapshot callback maps so we can invoke them outside the lock.
	var foundCBs []func(matchCount uint)
	var failedCBs []func()
	var countedCBs []func(matchCount uint)

	if count > 0 && !counting {
		foundCBs = make([]func(matchCount uint), 0, len(fc.onFound))
		for _, cb := range fc.onFound {
			foundCBs = append(foundCBs, cb)
		}
	}
	if count == 0 && finalUpdate != 0 && !counting {
		failedCBs = make([]func(), 0, len(fc.onFailed))
		for _, cb := range fc.onFailed {
			failedCBs = append(failedCBs, cb)
		}
	}
	if finalUpdate != 0 && counting {
		countedCBs = make([]func(matchCount uint), 0, len(fc.onCounted))
		for _, cb := range fc.onCounted {
			countedCBs = append(countedCBs, cb)
		}
	}
	fc.mu.RUnlock()

	var safeCount uint
	if count > 0 {
		safeCount = uint(count)
	}
	for _, cb := range foundCBs {
		cb(safeCount)
	}
	for _, cb := range failedCBs {
		cb()
	}
	for _, cb := range countedCBs {
		cb(safeCount)
	}
}

// matchCaseFromOpts converts FindOptions to CEF's matchcase parameter.
// CEF: matchcase=1 means case-sensitive, matchcase=0 means case-insensitive.
func matchCaseFromOpts(opts port.FindOptions) int32 {
	if opts.CaseInsensitive {
		return 0
	}
	return 1
}
