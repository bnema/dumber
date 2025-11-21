package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
)

// StorageAPI implements chrome.storage WebExtension API
type StorageAPI struct {
	local *StorageArea
	sync  *StorageArea // TODO: Implement sync storage
}

// StorageArea represents a storage area (local, sync, managed)
type StorageArea struct {
	extensionID string
	db          *sql.DB
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
func NewStorageAPI(extensionID string, db *sql.DB) (*StorageAPI, error) {
	return &StorageAPI{
		local: &StorageArea{
			extensionID: extensionID,
			db:          db,
			listeners:   make([]StorageChangeListener, 0),
		},
	}, nil
}

// Local returns the local storage area
func (s *StorageAPI) Local() *StorageArea {
	return s.local
}

// Get retrieves items from storage
// keys can be: nil (get all), string (single key), []string (multiple keys), or map[string]interface{} (keys with defaults)
func (s *StorageArea) Get(keys interface{}) (map[string]interface{}, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make(map[string]interface{})

	switch v := keys.(type) {
	case nil:
		// Get all items for this extension
		rows, err := s.db.Query(
			"SELECT key, value FROM extension_storage WHERE extension_id = ?",
			s.extensionID,
		)
		if err != nil {
			return nil, err
		}
		defer rows.Close()

		for rows.Next() {
			var key, valueJSON string
			if err := rows.Scan(&key, &valueJSON); err != nil {
				return nil, err
			}

			var value interface{}
			if err := json.Unmarshal([]byte(valueJSON), &value); err != nil {
				return nil, err
			}
			result[key] = value
		}

	case string:
		// Get single key
		var valueJSON string
		err := s.db.QueryRow(
			"SELECT value FROM extension_storage WHERE extension_id = ? AND key = ?",
			s.extensionID, v,
		).Scan(&valueJSON)

		if err == sql.ErrNoRows {
			// Key doesn't exist, return empty result
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
		// Get multiple keys
		for _, key := range v {
			var valueJSON string
			err := s.db.QueryRow(
				"SELECT value FROM extension_storage WHERE extension_id = ? AND key = ?",
				s.extensionID, key,
			).Scan(&valueJSON)

			if err == sql.ErrNoRows {
				continue // Skip missing keys
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
			var valueJSON string
			err := s.db.QueryRow(
				"SELECT value FROM extension_storage WHERE extension_id = ? AND key = ?",
				s.extensionID, key,
			).Scan(&valueJSON)

			if err == sql.ErrNoRows {
				// Use default value
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
	}

	log.Printf("[storage.local] Get for extension %s: %d items", s.extensionID, len(result))
	return result, nil
}

// Set sets items in storage
func (s *StorageArea) Set(items map[string]interface{}) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	changes := make(map[string]StorageChange)

	for key, newValue := range items {
		// Get old value for change event
		var oldValueJSON string
		err := s.db.QueryRow(
			"SELECT value FROM extension_storage WHERE extension_id = ? AND key = ?",
			s.extensionID, key,
		).Scan(&oldValueJSON)

		var oldValue interface{}
		if err != sql.ErrNoRows && err != nil {
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
		_, err = s.db.Exec(`
			INSERT INTO extension_storage (extension_id, key, value, updated_at)
			VALUES (?, ?, ?, CURRENT_TIMESTAMP)
			ON CONFLICT(extension_id, key)
			DO UPDATE SET value = excluded.value, updated_at = CURRENT_TIMESTAMP
		`, s.extensionID, key, string(newValueJSON))

		if err != nil {
			return fmt.Errorf("failed to set key %s: %w", key, err)
		}

		// Record change
		changes[key] = StorageChange{
			OldValue: oldValue,
			NewValue: newValue,
		}
	}

	log.Printf("[storage.local] Set for extension %s: %d items", s.extensionID, len(items))

	// Notify listeners
	s.notifyListeners(changes, "local")

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
	default:
		return fmt.Errorf("keys must be string or []string")
	}

	changes := make(map[string]StorageChange)

	for _, key := range keyList {
		// Get old value for change event
		var oldValueJSON string
		err := s.db.QueryRow(
			"SELECT value FROM extension_storage WHERE extension_id = ? AND key = ?",
			s.extensionID, key,
		).Scan(&oldValueJSON)

		if err == sql.ErrNoRows {
			continue // Key doesn't exist
		}
		if err != nil {
			return err
		}

		var oldValue interface{}
		json.Unmarshal([]byte(oldValueJSON), &oldValue)

		// Delete from database
		_, err = s.db.Exec(
			"DELETE FROM extension_storage WHERE extension_id = ? AND key = ?",
			s.extensionID, key,
		)
		if err != nil {
			return fmt.Errorf("failed to remove key %s: %w", key, err)
		}

		// Record change (newValue is undefined/nil)
		changes[key] = StorageChange{
			OldValue: oldValue,
			NewValue: nil,
		}
	}

	log.Printf("[storage.local] Remove for extension %s: %d items", s.extensionID, len(keyList))

	// Notify listeners
	s.notifyListeners(changes, "local")

	return nil
}

// Clear removes all items from storage
func (s *StorageArea) Clear() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Get all current items for change event
	rows, err := s.db.Query(
		"SELECT key, value FROM extension_storage WHERE extension_id = ?",
		s.extensionID,
	)
	if err != nil {
		return err
	}
	defer rows.Close()

	changes := make(map[string]StorageChange)
	for rows.Next() {
		var key, valueJSON string
		if err := rows.Scan(&key, &valueJSON); err != nil {
			return err
		}

		var oldValue interface{}
		json.Unmarshal([]byte(valueJSON), &oldValue)

		changes[key] = StorageChange{
			OldValue: oldValue,
			NewValue: nil,
		}
	}

	// Clear all items for this extension
	_, err = s.db.Exec(
		"DELETE FROM extension_storage WHERE extension_id = ?",
		s.extensionID,
	)
	if err != nil {
		return err
	}

	log.Printf("[storage.local] Clear for extension %s: %d items removed", s.extensionID, len(changes))

	// Notify listeners
	s.notifyListeners(changes, "local")

	return nil
}

// OnChanged registers a listener for storage changes
func (s *StorageArea) OnChanged(listener StorageChangeListener) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.listeners = append(s.listeners, listener)
	log.Printf("[storage.local] Registered onChanged listener for extension %s", s.extensionID)
}

// notifyListeners notifies all registered listeners of storage changes
func (s *StorageArea) notifyListeners(changes map[string]StorageChange, areaName string) {
	for _, listener := range s.listeners {
		go listener(changes, areaName)
	}
}

// --- Dispatcher-compatible API (works across all extensions) ---

// StorageAPIDispatcher provides storage API methods for the dispatcher
// This works with any extension ID passed as a parameter
type StorageAPIDispatcher struct {
	dataDir string // Base data directory for all extensions
	mu      sync.RWMutex
}

// NewStorageAPIDispatcher creates a storage API for the dispatcher
func NewStorageAPIDispatcher(dataDir string) *StorageAPIDispatcher {
	return &StorageAPIDispatcher{
		dataDir: dataDir,
	}
}

// Get retrieves items from storage for a specific extension
func (s *StorageAPIDispatcher) Get(ctx context.Context, extID string, keys interface{}) (map[string]interface{}, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	storagePath := filepath.Join(s.dataDir, "extensions", extID, "storage.json")

	// Read storage file
	data, err := os.ReadFile(storagePath)
	if os.IsNotExist(err) {
		// No storage file yet, return empty/defaults
		return s.getDefaults(keys), nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to read storage: %w", err)
	}

	var storage map[string]interface{}
	if err := json.Unmarshal(data, &storage); err != nil {
		return nil, fmt.Errorf("failed to parse storage: %w", err)
	}

	result := make(map[string]interface{})

	switch v := keys.(type) {
	case nil:
		// Get all items
		return storage, nil

	case string:
		// Get single key
		if val, ok := storage[v]; ok {
			result[v] = val
		}

	case []interface{}:
		// Get multiple keys (from JSON array)
		for _, key := range v {
			if keyStr, ok := key.(string); ok {
				if val, ok := storage[keyStr]; ok {
					result[keyStr] = val
				}
			}
		}

	case []string:
		// Get multiple keys
		for _, key := range v {
			if val, ok := storage[key]; ok {
				result[key] = val
			}
		}

	case map[string]interface{}:
		// Get keys with default values
		for key, defaultVal := range v {
			if val, ok := storage[key]; ok {
				result[key] = val
			} else {
				result[key] = defaultVal
			}
		}
	}

	log.Printf("[storage.local] Get for extension %s: %d items", extID, len(result))
	return result, nil
}

// Set sets items in storage for a specific extension
func (s *StorageAPIDispatcher) Set(ctx context.Context, extID string, items map[string]interface{}) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Ensure directory exists
	storageDir := filepath.Join(s.dataDir, "extensions", extID)
	if err := os.MkdirAll(storageDir, 0755); err != nil {
		return fmt.Errorf("failed to create storage directory: %w", err)
	}

	storagePath := filepath.Join(storageDir, "storage.json")

	// Read existing storage
	var storage map[string]interface{}
	data, err := os.ReadFile(storagePath)
	if err == nil {
		if err := json.Unmarshal(data, &storage); err != nil {
			return fmt.Errorf("failed to parse existing storage: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("failed to read storage: %w", err)
	} else {
		storage = make(map[string]interface{})
	}

	// Update with new items
	for key, value := range items {
		storage[key] = value
	}

	// Write back to file
	data, err = json.MarshalIndent(storage, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal storage: %w", err)
	}

	if err := os.WriteFile(storagePath, data, 0644); err != nil {
		return fmt.Errorf("failed to write storage: %w", err)
	}

	log.Printf("[storage.local] Set for extension %s: %d items", extID, len(items))
	return nil
}

// Remove removes items from storage for a specific extension
func (s *StorageAPIDispatcher) Remove(ctx context.Context, extID string, keys interface{}) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	storagePath := filepath.Join(s.dataDir, "extensions", extID, "storage.json")

	// Read existing storage
	var storage map[string]interface{}
	data, err := os.ReadFile(storagePath)
	if os.IsNotExist(err) {
		// No storage file, nothing to remove
		return nil
	}
	if err != nil {
		return fmt.Errorf("failed to read storage: %w", err)
	}

	if err := json.Unmarshal(data, &storage); err != nil {
		return fmt.Errorf("failed to parse storage: %w", err)
	}

	// Remove keys
	switch v := keys.(type) {
	case string:
		delete(storage, v)

	case []interface{}:
		for _, key := range v {
			if keyStr, ok := key.(string); ok {
				delete(storage, keyStr)
			}
		}

	case []string:
		for _, key := range v {
			delete(storage, key)
		}
	}

	// Write back
	data, err = json.MarshalIndent(storage, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal storage: %w", err)
	}

	if err := os.WriteFile(storagePath, data, 0644); err != nil {
		return fmt.Errorf("failed to write storage: %w", err)
	}

	log.Printf("[storage.local] Remove for extension %s", extID)
	return nil
}

// Clear removes all items from storage for a specific extension
func (s *StorageAPIDispatcher) Clear(ctx context.Context, extID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	storagePath := filepath.Join(s.dataDir, "extensions", extID, "storage.json")

	// Write empty storage
	emptyStorage := make(map[string]interface{})
	data, err := json.MarshalIndent(emptyStorage, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal empty storage: %w", err)
	}

	// Ensure directory exists
	storageDir := filepath.Join(s.dataDir, "extensions", extID)
	if err := os.MkdirAll(storageDir, 0755); err != nil {
		return fmt.Errorf("failed to create storage directory: %w", err)
	}

	if err := os.WriteFile(storagePath, data, 0644); err != nil {
		return fmt.Errorf("failed to write storage: %w", err)
	}

	log.Printf("[storage.local] Clear for extension %s", extID)
	return nil
}

// getDefaults returns default values based on keys type
func (s *StorageAPIDispatcher) getDefaults(keys interface{}) map[string]interface{} {
	result := make(map[string]interface{})

	switch v := keys.(type) {
	case map[string]interface{}:
		// Return the defaults
		return v
	}

	return result
}
