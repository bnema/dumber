package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/bnema/dumber/internal/db"
)

const (
	storageTypeLocal = "storage.local"
	storageTypeSync  = "storage.sync"
)

// StorageAPI implements chrome.storage WebExtension API
type StorageAPI struct {
	local       *StorageArea
	sync        *StorageArea
	installDir  string
	extensionID string
	db          *sql.DB
}

// StorageArea represents a storage area (local, sync, managed)
type StorageArea struct {
	extensionID string
	storageType string // "storage.local" or "storage.sync"
	areaName    string // "local" or "sync" for change events
	queries     *db.Queries
	installDir  string
	mu          sync.RWMutex
	listeners   []StorageChangeListener
}

// StorageChangeListener is called when storage changes
type StorageChangeListener func(changes map[string]StorageChange, areaName string)

// StorageChange represents a change to a storage item
type StorageChange struct {
	OldValue interface{} `json:"oldValue,omitempty"`
	NewValue interface{} `json:"newValue,omitempty"`
}

// NewStorageAPI creates a new storage API instance
// The extension_storage table is created by migration 010_extensions.sql
func NewStorageAPI(extensionID, installDir string, dbConn *sql.DB) (*StorageAPI, error) {
	queries := db.New(dbConn)
	maybeResetUBlockSelfie(extensionID, installDir, queries)

	return &StorageAPI{
		installDir:  installDir,
		extensionID: extensionID,
		db:          dbConn,
		local: &StorageArea{
			extensionID: extensionID,
			storageType: storageTypeLocal,
			areaName:    "local",
			queries:     queries,
			installDir:  installDir,
			listeners:   make([]StorageChangeListener, 0),
		},
		sync: &StorageArea{
			extensionID: extensionID,
			storageType: storageTypeSync,
			areaName:    "sync",
			queries:     queries,
			installDir:  installDir,
			listeners:   make([]StorageChangeListener, 0),
		},
	}, nil
}

// maybeResetUBlockSelfie clears obviously-corrupt uBlock cache/selfie data so the
// extension will refetch its filter list definitions (assets.json).
// We only reset when the cached availableFilterLists contains only the built-in
// user list, which indicates the bootstrap assets never loaded correctly.
func maybeResetUBlockSelfie(extensionID, installDir string, queries *db.Queries) {
	if extensionID != "ublock-origin" {
		return
	}

	ctx := context.Background()
	hasAssetSourceRegistry := exists(ctx, queries, extensionID, "assetSourceRegistry")
	shouldClear := false
	rawLists, err := queries.GetExtensionStorageItem(ctx, extensionID, storageTypeLocal, "availableFilterLists")
	if err != nil && err != sql.ErrNoRows {
		log.Printf("[storage.local] uBlock selfie check skipped (read error): %v", err)
		return
	}

	if err == nil {
		var lists map[string]interface{}
		if err := json.Unmarshal([]byte(rawLists), &lists); err != nil {
			log.Printf("[storage.local] uBlock selfie check skipped (unmarshal error): %v", err)
			return
		}

		// Only the user filters entry exists -> cached selfie is incomplete/invalid.
		if len(lists) == 1 {
			if _, ok := lists["user-filters"]; ok {
				shouldClear = true
				log.Printf("[storage.local] Detected invalid uBlock selfie (only user-filters)")
			}
		}
	}

	// Also clear if tiny selfie cache entries exist (stale/invalid) even without availableFilterLists.
	cacheKeys := []string{
		"cache/selfie/staticMain",
		"cache/selfie/staticExtFilteringEngine",
		"cache/selfie/staticNetFilteringEngine",
	}
	for _, k := range cacheKeys {
		if exists(ctx, queries, extensionID, k) {
			shouldClear = true
			break
		}
	}

	if !shouldClear {
		// If we already have a full asset source registry, nothing else to do.
		if hasAssetSourceRegistry {
			return
		}
	}

	log.Printf("[storage.local] Clearing uBlock cached selfie/filter metadata to force reload")

	keys, err := queries.GetExtensionStorageKeys(ctx, extensionID, storageTypeLocal)
	if err != nil {
		log.Printf("[storage.local] uBlock selfie reset failed to list keys: %v", err)
		return
	}

	for _, key := range keys {
		if key == "availableFilterLists" ||
			key == "selectedFilterLists" ||
			key == "assetSourceRegistry" ||
			key == "assetCacheRegistry" ||
			strings.HasPrefix(key, "cache/selfie/") {
			if delErr := queries.DeleteExtensionStorageItem(ctx, extensionID, storageTypeLocal, key); delErr != nil {
				log.Printf("[storage.local] Failed to clear %s for %s: %v", key, extensionID, delErr)
			}
		}
	}

	// Seed assetSourceRegistry (and default selected lists) directly from the bundled assets.json
	seedFromAssetsJSON(ctx, queries, extensionID, installDir)
}

// exists reports whether a key exists for the given extension/storage type.
func exists(ctx context.Context, queries *db.Queries, extID, key string) bool {
	_, err := queries.GetExtensionStorageItem(ctx, extID, storageTypeLocal, key)
	return err == nil
}

// seedFromAssetsJSON loads assets/assets.json from the installed extension and writes
// assetSourceRegistry and selectedFilterLists into storage so uBO can bootstrap without XHR.
func seedFromAssetsJSON(ctx context.Context, queries *db.Queries, extensionID, installDir string) {
	if installDir == "" {
		return
	}
	assetsPath := filepath.Join(installDir, "assets", "assets.json")
	data, err := os.ReadFile(assetsPath)
	if err != nil {
		log.Printf("[storage.local] Failed to seed uBlock assets.json: %v", err)
		return
	}

	var assets map[string]interface{}
	if err := json.Unmarshal(data, &assets); err != nil {
		log.Printf("[storage.local] Failed to parse uBlock assets.json: %v", err)
		return
	}

	// Compute default list selection (filters with off unset)
	defaultLists := make([]string, 0)
	for k, raw := range assets {
		entry, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		if entry["content"] != "filters" {
			continue
		}
		if _, hasOff := entry["off"]; hasOff {
			continue
		}
		defaultLists = append(defaultLists, k)
	}

	// Store assetSourceRegistry and default selected lists
	if err := queries.UpsertExtensionStorageItem(ctx, extensionID, storageTypeLocal, "assetSourceRegistry", string(data)); err != nil {
		log.Printf("[storage.local] Failed to seed assetSourceRegistry: %v", err)
	}
	if err := queries.UpsertExtensionStorageItem(ctx, extensionID, storageTypeLocal, "selectedFilterLists", mustJSON(defaultLists)); err != nil {
		log.Printf("[storage.local] Failed to seed selectedFilterLists: %v", err)
	}
}

func mustJSON(v interface{}) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "null"
	}
	return string(b)
}

// Local returns the local storage area
func (s *StorageAPI) Local() *StorageArea {
	return s.local
}

// Sync returns the sync storage area
// Note: In this implementation, sync storage is stored locally.
// For true cross-device sync, integration with a sync service would be needed.
func (s *StorageAPI) Sync() *StorageArea {
	return s.sync
}

// Get retrieves items from storage
// keys can be: nil (get all), string (single key), []string (multiple keys), or map[string]interface{} (keys with defaults)
func (s *StorageArea) Get(keys interface{}) (map[string]interface{}, error) {
	// For uBlock local storage, aggressively purge an invalid selfie before every read so the
	// JS side is forced to refetch assets.json and rebuild filter metadata.
	if s.storageType == storageTypeLocal {
		maybeResetUBlockSelfie(s.extensionID, s.installDir, s.queries)
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	ctx := context.Background()
	result := make(map[string]interface{})

	switch v := keys.(type) {
	case nil:
		// Get all items for this extension
		rows, err := s.queries.GetAllExtensionStorageItems(ctx, s.extensionID, s.storageType)
		if err != nil {
			log.Printf("[storage.%s] GetAll error for %s: %v", s.areaName, s.extensionID, err)
			return nil, err
		}
		log.Printf("[storage.%s] GetAll raw rows for %s: %d", s.areaName, s.extensionID, len(rows))

		for _, row := range rows {
			var value interface{}
			if err := json.Unmarshal([]byte(row.Value), &value); err != nil {
				return nil, err
			}
			result[row.Key] = value
		}

	case string:
		// Get single key
		log.Printf("[storage.%s] Get single key for %s: %s", s.areaName, s.extensionID, v)
		valueJSON, err := s.queries.GetExtensionStorageItem(ctx, s.extensionID, s.storageType, v)
		if err == sql.ErrNoRows {
			return result, nil
		}
		if err != nil {
			return nil, err
		}

		var value interface{}
		if err := json.Unmarshal([]byte(valueJSON), &value); err != nil {
			return nil, err
		}
		result[v] = value

	case []string:
		// Get multiple keys (native Go string slice)
		log.Printf("[storage.%s] Get multiple keys for %s: %v", s.areaName, s.extensionID, v)
		for _, key := range v {
			valueJSON, err := s.queries.GetExtensionStorageItem(ctx, s.extensionID, s.storageType, key)
			if err == sql.ErrNoRows {
				continue
			}
			if err != nil {
				return nil, err
			}

			var value interface{}
			if err := json.Unmarshal([]byte(valueJSON), &value); err != nil {
				return nil, err
			}
			result[key] = value
		}

	case []interface{}:
		// Get multiple keys (from Sobek JS array export)
		log.Printf("[storage.%s] Get multiple keys ([]interface{}) for %s: %d keys", s.areaName, s.extensionID, len(v))
		for _, keyRaw := range v {
			key, ok := keyRaw.(string)
			if !ok {
				continue
			}
			valueJSON, err := s.queries.GetExtensionStorageItem(ctx, s.extensionID, s.storageType, key)
			if err == sql.ErrNoRows {
				continue
			}
			if err != nil {
				return nil, err
			}

			var value interface{}
			if err := json.Unmarshal([]byte(valueJSON), &value); err != nil {
				return nil, err
			}
			result[key] = value
		}

	case map[string]interface{}:
		// Get keys with default values
		for key, defaultValue := range v {
			valueJSON, err := s.queries.GetExtensionStorageItem(ctx, s.extensionID, s.storageType, key)
			if err == sql.ErrNoRows {
				result[key] = defaultValue
				continue
			}
			if err != nil {
				return nil, err
			}

			var value interface{}
			if err := json.Unmarshal([]byte(valueJSON), &value); err != nil {
				return nil, err
			}
			result[key] = value
		}

	default:
		log.Printf("[storage.%s] Get unhandled key type for %s: %T", s.areaName, s.extensionID, keys)
	}

	log.Printf("[storage.%s] Get for extension %s: %d items", s.areaName, s.extensionID, len(result))
	return result, nil
}

// Set sets items in storage
func (s *StorageArea) Set(items map[string]interface{}) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	ctx := context.Background()
	changes := make(map[string]StorageChange)

	for key, newValue := range items {
		// Get old value for change event
		oldValueJSON, err := s.queries.GetExtensionStorageItem(ctx, s.extensionID, s.storageType, key)

		var oldValue interface{}
		if err != nil && err != sql.ErrNoRows {
			return err
		}
		if err == nil {
			json.Unmarshal([]byte(oldValueJSON), &oldValue)
		}

		// Serialize new value
		newValueJSON, err := json.Marshal(newValue)
		if err != nil {
			return fmt.Errorf("failed to marshal value for key %s: %w", key, err)
		}

		// Upsert into database
		err = s.queries.UpsertExtensionStorageItem(ctx, s.extensionID, s.storageType, key, string(newValueJSON))
		if err != nil {
			return fmt.Errorf("failed to set key %s: %w", key, err)
		}

		// Record change
		changes[key] = StorageChange{
			OldValue: oldValue,
			NewValue: newValue,
		}
	}

	log.Printf("[storage.%s] Set for extension %s: %d items", s.areaName, s.extensionID, len(items))

	// Notify listeners
	s.notifyListeners(changes, s.areaName)

	return nil
}

// Remove removes items from storage
func (s *StorageArea) Remove(keys interface{}) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	var keyList []string
	switch v := keys.(type) {
	case string:
		keyList = []string{v}
	case []string:
		keyList = v
	case []interface{}:
		// Handle arrays from JS
		for _, k := range v {
			if str, ok := k.(string); ok {
				keyList = append(keyList, str)
			}
		}
	default:
		return fmt.Errorf("keys must be string or []string")
	}

	ctx := context.Background()
	changes := make(map[string]StorageChange)

	for _, key := range keyList {
		// Get old value for change event
		oldValueJSON, err := s.queries.GetExtensionStorageItem(ctx, s.extensionID, s.storageType, key)
		if err == sql.ErrNoRows {
			continue
		}
		if err != nil {
			return err
		}

		var oldValue interface{}
		json.Unmarshal([]byte(oldValueJSON), &oldValue)

		// Delete from database
		err = s.queries.DeleteExtensionStorageItem(ctx, s.extensionID, s.storageType, key)
		if err != nil {
			return fmt.Errorf("failed to remove key %s: %w", key, err)
		}

		// Record change (newValue is undefined/nil)
		changes[key] = StorageChange{
			OldValue: oldValue,
			NewValue: nil,
		}
	}

	log.Printf("[storage.%s] Remove for extension %s: %d items", s.areaName, s.extensionID, len(keyList))

	// Notify listeners
	s.notifyListeners(changes, s.areaName)

	return nil
}

// Clear removes all items from storage
func (s *StorageArea) Clear() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	ctx := context.Background()

	// Get all current items for change event
	rows, err := s.queries.GetAllExtensionStorageItems(ctx, s.extensionID, s.storageType)
	if err != nil {
		return err
	}

	changes := make(map[string]StorageChange)
	for _, row := range rows {
		var oldValue interface{}
		json.Unmarshal([]byte(row.Value), &oldValue)

		changes[row.Key] = StorageChange{
			OldValue: oldValue,
			NewValue: nil,
		}
	}

	// Clear all items for this extension
	err = s.queries.ClearExtensionStorage(ctx, s.extensionID, s.storageType)
	if err != nil {
		return err
	}

	log.Printf("[storage.%s] Clear for extension %s: %d items removed", s.areaName, s.extensionID, len(changes))

	// Notify listeners
	s.notifyListeners(changes, s.areaName)

	return nil
}

// OnChanged registers a listener for storage changes
func (s *StorageArea) OnChanged(listener StorageChangeListener) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.listeners = append(s.listeners, listener)
	log.Printf("[storage.%s] Registered onChanged listener for extension %s", s.areaName, s.extensionID)
}

// GetBytesInUse returns the amount of space (in bytes) used by one or more items
// API: browser.storage.local.getBytesInUse()
func (s *StorageArea) GetBytesInUse(keys interface{}) (int64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	ctx := context.Background()
	var totalBytes int64

	switch v := keys.(type) {
	case nil:
		// Get size of all items
		rows, err := s.queries.GetAllExtensionStorageItems(ctx, s.extensionID, s.storageType)
		if err != nil {
			return 0, err
		}
		for _, row := range rows {
			totalBytes += int64(len(row.Key) + len(row.Value))
		}

	case string:
		// Get size of single key
		valueJSON, err := s.queries.GetExtensionStorageItem(ctx, s.extensionID, s.storageType, v)
		if err == sql.ErrNoRows {
			return 0, nil
		}
		if err != nil {
			return 0, err
		}
		totalBytes = int64(len(v) + len(valueJSON))

	case []string:
		// Get size of multiple keys
		for _, key := range v {
			valueJSON, err := s.queries.GetExtensionStorageItem(ctx, s.extensionID, s.storageType, key)
			if err == sql.ErrNoRows {
				continue
			}
			if err != nil {
				return 0, err
			}
			totalBytes += int64(len(key) + len(valueJSON))
		}

	case []interface{}:
		// Handle arrays from JS
		for _, keyRaw := range v {
			key, ok := keyRaw.(string)
			if !ok {
				continue
			}
			valueJSON, err := s.queries.GetExtensionStorageItem(ctx, s.extensionID, s.storageType, key)
			if err == sql.ErrNoRows {
				continue
			}
			if err != nil {
				return 0, err
			}
			totalBytes += int64(len(key) + len(valueJSON))
		}
	}

	return totalBytes, nil
}

// notifyListeners notifies all registered listeners of storage changes
func (s *StorageArea) notifyListeners(changes map[string]StorageChange, areaName string) {
	for _, listener := range s.listeners {
		go listener(changes, areaName)
	}
}
