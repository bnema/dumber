package api

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
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
// StorageAPIDispatcher removed - Dispatcher now uses Manager's storage API directly
