package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
)

// NotificationType represents the type of notification template
type NotificationType string

const (
	NotificationTypeBasic    NotificationType = "basic"
	NotificationTypeImage    NotificationType = "image"
	NotificationTypeList     NotificationType = "list"
	NotificationTypeProgress NotificationType = "progress"
)

// NotificationButton represents a button in a notification
type NotificationButton struct {
	Title string `json:"title"`
}

// Notification represents a browser notification
type Notification struct {
	Type        NotificationType     `json:"type"`
	IconURL     string               `json:"iconUrl,omitempty"`
	Title       string               `json:"title"`
	Message     string               `json:"message"`
	Priority    int                  `json:"priority,omitempty"`    // -2 to 2
	EventTime   int64                `json:"eventTime,omitempty"`   // Unix timestamp in milliseconds
	Buttons     []NotificationButton `json:"buttons,omitempty"`     // Max 2 buttons
	ImageURL    string               `json:"imageUrl,omitempty"`    // For image type
	Progress    int                  `json:"progress,omitempty"`    // 0-100 for progress type
	IsClickable bool                 `json:"isClickable,omitempty"` // Whether notification can be clicked
}

// NotificationsAPIDispatcher handles notifications.* API calls
type NotificationsAPIDispatcher struct {
	mu            sync.RWMutex
	notifications map[string]map[string]*Notification // extensionID -> notificationID -> Notification
}

// NewNotificationsAPIDispatcher creates a new notifications API dispatcher
func NewNotificationsAPIDispatcher() *NotificationsAPIDispatcher {
	return &NotificationsAPIDispatcher{
		notifications: make(map[string]map[string]*Notification),
	}
}

// generateNotificationID generates a random notification ID
func generateNotificationID() string {
	bytes := make([]byte, 16)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)
}

// Create creates a new notification
func (d *NotificationsAPIDispatcher) Create(ctx context.Context, extensionID string, notificationID string, options map[string]interface{}) (string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	// Ensure notifications map exists for this extension
	if d.notifications[extensionID] == nil {
		d.notifications[extensionID] = make(map[string]*Notification)
	}

	// Generate ID if not provided
	if notificationID == "" {
		notificationID = generateNotificationID()
	}

	// Extract required fields
	title, ok := options["title"].(string)
	if !ok || title == "" {
		return "", fmt.Errorf("notifications.create(): missing or invalid 'title' field")
	}

	message, ok := options["message"].(string)
	if !ok || message == "" {
		return "", fmt.Errorf("notifications.create(): missing or invalid 'message' field")
	}

	// Create notification
	notification := &Notification{
		Type:        NotificationTypeBasic, // Default type
		Title:       title,
		Message:     message,
		IsClickable: true, // Default to clickable
	}

	// Extract optional fields
	if typeStr, ok := options["type"].(string); ok {
		switch NotificationType(typeStr) {
		case NotificationTypeBasic, NotificationTypeImage, NotificationTypeList, NotificationTypeProgress:
			notification.Type = NotificationType(typeStr)
		}
	}

	if iconURL, ok := options["iconUrl"].(string); ok {
		notification.IconURL = iconURL
	}

	if priority, ok := options["priority"].(float64); ok {
		// Clamp priority to -2 to 2 range
		p := int(priority)
		if p < -2 {
			p = -2
		} else if p > 2 {
			p = 2
		}
		notification.Priority = p
	}

	if eventTime, ok := options["eventTime"].(float64); ok {
		notification.EventTime = int64(eventTime)
	}

	if imageURL, ok := options["imageUrl"].(string); ok {
		notification.ImageURL = imageURL
	}

	if progress, ok := options["progress"].(float64); ok {
		// Clamp progress to 0-100 range
		p := int(progress)
		if p < 0 {
			p = 0
		} else if p > 100 {
			p = 100
		}
		notification.Progress = p
	}

	if isClickable, ok := options["isClickable"].(bool); ok {
		notification.IsClickable = isClickable
	}

	// Extract buttons (max 2)
	if buttonsRaw, ok := options["buttons"].([]interface{}); ok {
		buttons := make([]NotificationButton, 0, 2)
		for i, btnRaw := range buttonsRaw {
			if i >= 2 {
				break // Max 2 buttons
			}
			if btnMap, ok := btnRaw.(map[string]interface{}); ok {
				if btnTitle, ok := btnMap["title"].(string); ok {
					buttons = append(buttons, NotificationButton{Title: btnTitle})
				}
			}
		}
		notification.Buttons = buttons
	}

	// Store notification
	d.notifications[extensionID][notificationID] = notification

	// Note: In a real implementation, you would emit a system notification here
	// For Dumber's keyboard-driven interface, you might show notifications differently

	return notificationID, nil
}

// Update updates an existing notification
func (d *NotificationsAPIDispatcher) Update(ctx context.Context, extensionID string, notificationID string, options map[string]interface{}) (bool, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	notifications, exists := d.notifications[extensionID]
	if !exists {
		return false, nil // Notification doesn't exist
	}

	existing, exists := notifications[notificationID]
	if !exists {
		return false, nil // Notification doesn't exist
	}

	// Update fields that are provided
	if title, ok := options["title"].(string); ok {
		existing.Title = title
	}

	if message, ok := options["message"].(string); ok {
		existing.Message = message
	}

	if typeStr, ok := options["type"].(string); ok {
		switch NotificationType(typeStr) {
		case NotificationTypeBasic, NotificationTypeImage, NotificationTypeList, NotificationTypeProgress:
			existing.Type = NotificationType(typeStr)
		}
	}

	if iconURL, ok := options["iconUrl"].(string); ok {
		existing.IconURL = iconURL
	}

	if priority, ok := options["priority"].(float64); ok {
		p := int(priority)
		if p < -2 {
			p = -2
		} else if p > 2 {
			p = 2
		}
		existing.Priority = p
	}

	if eventTime, ok := options["eventTime"].(float64); ok {
		existing.EventTime = int64(eventTime)
	}

	if imageURL, ok := options["imageUrl"].(string); ok {
		existing.ImageURL = imageURL
	}

	if progress, ok := options["progress"].(float64); ok {
		p := int(progress)
		if p < 0 {
			p = 0
		} else if p > 100 {
			p = 100
		}
		existing.Progress = p
	}

	if isClickable, ok := options["isClickable"].(bool); ok {
		existing.IsClickable = isClickable
	}

	if buttonsRaw, ok := options["buttons"].([]interface{}); ok {
		buttons := make([]NotificationButton, 0, 2)
		for i, btnRaw := range buttonsRaw {
			if i >= 2 {
				break
			}
			if btnMap, ok := btnRaw.(map[string]interface{}); ok {
				if btnTitle, ok := btnMap["title"].(string); ok {
					buttons = append(buttons, NotificationButton{Title: btnTitle})
				}
			}
		}
		existing.Buttons = buttons
	}

	// Note: In a real implementation, you would update the system notification here

	return true, nil
}

// Clear clears a notification by ID
func (d *NotificationsAPIDispatcher) Clear(ctx context.Context, extensionID string, notificationID string) (bool, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	notifications, exists := d.notifications[extensionID]
	if !exists {
		return false, nil // Extension has no notifications
	}

	if _, exists := notifications[notificationID]; !exists {
		return false, nil // Notification doesn't exist
	}

	delete(notifications, notificationID)

	// Note: In a real implementation, you would withdraw the system notification here

	return true, nil
}

// GetAll retrieves all active notifications for an extension
func (d *NotificationsAPIDispatcher) GetAll(ctx context.Context, extensionID string) (map[string]bool, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	notifications, exists := d.notifications[extensionID]
	if !exists {
		return make(map[string]bool), nil // Return empty map
	}

	// Return map of notification IDs to true (indicating they're active)
	result := make(map[string]bool, len(notifications))
	for id := range notifications {
		result[id] = true
	}

	return result, nil
}

// CleanupExtension removes all notifications for an extension (called when extension is unloaded)
func (d *NotificationsAPIDispatcher) CleanupExtension(extensionID string) {
	d.mu.Lock()
	defer d.mu.Unlock()

	delete(d.notifications, extensionID)
}
