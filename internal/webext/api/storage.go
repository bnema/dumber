package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"sync"

	"github.com/bnema/dumber/internal/db"
)

const storageTypeLocal = "storage.local"

// StorageAPI implements chrome.storage WebExtension API
type StorageAPI struct {
	local *StorageArea
	sync  *StorageArea // TODO: Implement sync storage
}

// StorageArea represents a storage area (local, sync, managed)
type StorageArea struct {
	extensionID string
	queries     *db.Queries
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
func NewStorageAPI(extensionID string, dbConn *sql.DB) (*StorageAPI, error) {
	return &StorageAPI{
		local: &StorageArea{
			extensionID: extensionID,
			queries:     db.New(dbConn),
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

	ctx := context.Background()
	result := make(map[string]interface{})

	switch v := keys.(type) {
	case nil:
		// Get all items for this extension
		rows, err := s.queries.GetAllExtensionStorageItems(ctx, s.extensionID, storageTypeLocal)
		if err != nil {
			log.Printf("[storage.local] GetAll error for %s: %v", s.extensionID, err)
			return nil, err
		}
		log.Printf("[storage.local] GetAll raw rows for %s: %d", s.extensionID, len(rows))

		for _, row := range rows {
			var value interface{}
			if err := json.Unmarshal([]byte(row.Value), &value); err != nil {
				return nil, err
			}
			result[row.Key] = value
		}

	case string:
		// Get single key
		log.Printf("[storage.local] Get single key for %s: %s", s.extensionID, v)
		valueJSON, err := s.queries.GetExtensionStorageItem(ctx, s.extensionID, storageTypeLocal, v)
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
		// Get multiple keys
		log.Printf("[storage.local] Get multiple keys for %s: %v", s.extensionID, v)
		for _, key := range v {
			valueJSON, err := s.queries.GetExtensionStorageItem(ctx, s.extensionID, storageTypeLocal, key)
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
			valueJSON, err := s.queries.GetExtensionStorageItem(ctx, s.extensionID, storageTypeLocal, key)
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
	}

	log.Printf("[storage.local] Get for extension %s: %d items", s.extensionID, len(result))
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
		oldValueJSON, err := s.queries.GetExtensionStorageItem(ctx, s.extensionID, storageTypeLocal, key)

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
		err = s.queries.UpsertExtensionStorageItem(ctx, s.extensionID, storageTypeLocal, key, string(newValueJSON))
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

	ctx := context.Background()
	changes := make(map[string]StorageChange)

	for _, key := range keyList {
		// Get old value for change event
		oldValueJSON, err := s.queries.GetExtensionStorageItem(ctx, s.extensionID, storageTypeLocal, key)
		if err == sql.ErrNoRows {
			continue
		}
		if err != nil {
			return err
		}

		var oldValue interface{}
		json.Unmarshal([]byte(oldValueJSON), &oldValue)

		// Delete from database
		err = s.queries.DeleteExtensionStorageItem(ctx, s.extensionID, storageTypeLocal, key)
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

	ctx := context.Background()

	// Get all current items for change event
	rows, err := s.queries.GetAllExtensionStorageItems(ctx, s.extensionID, storageTypeLocal)
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
	err = s.queries.ClearExtensionStorage(ctx, s.extensionID, storageTypeLocal)
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
