package api

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// Alarm represents a scheduled alarm
type Alarm struct {
	Name            string   `json:"name"`
	ScheduledTime   float64  `json:"scheduledTime"`   // Unix timestamp in milliseconds
	PeriodInMinutes *float64 `json:"periodInMinutes"` // nil for one-time alarms
}

// alarmInternal holds the internal state of an alarm
type alarmInternal struct {
	alarm         Alarm
	timer         *time.Timer
	ticker        *time.Ticker
	stopChan      chan struct{}
	extensionID   string
	eventCallback func(alarm Alarm) // Callback to emit onAlarm event
}

// AlarmsAPIDispatcher handles alarms.* API calls
type AlarmsAPIDispatcher struct {
	mu     sync.RWMutex
	alarms map[string]map[string]*alarmInternal // extensionID -> alarmName -> alarmInternal
}

// NewAlarmsAPIDispatcher creates a new alarms API dispatcher
func NewAlarmsAPIDispatcher() *AlarmsAPIDispatcher {
	return &AlarmsAPIDispatcher{
		alarms: make(map[string]map[string]*alarmInternal),
	}
}

// SetEventCallback sets the callback function for emitting onAlarm events
func (d *AlarmsAPIDispatcher) SetEventCallback(extensionID string, callback func(alarm Alarm)) {
	d.mu.Lock()
	defer d.mu.Unlock()

	// Ensure alarms map exists for this extension
	if d.alarms[extensionID] == nil {
		d.alarms[extensionID] = make(map[string]*alarmInternal)
	}

	// Update callback for all existing alarms
	for _, internal := range d.alarms[extensionID] {
		internal.eventCallback = callback
	}
}

// Create creates a new alarm
func (d *AlarmsAPIDispatcher) Create(ctx context.Context, extensionID string, nameArg interface{}, alarmInfo map[string]interface{}, eventCallback func(alarm Alarm)) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	// Ensure alarms map exists for this extension
	if d.alarms[extensionID] == nil {
		d.alarms[extensionID] = make(map[string]*alarmInternal)
	}

	// Extract alarm name (optional, defaults to empty string)
	name := ""
	if nameArg != nil {
		if nameStr, ok := nameArg.(string); ok {
			name = nameStr
		}
	}

	// Extract alarm info parameters
	var delayInMinutes, periodInMinutes, when float64
	hasDelay := false
	hasPeriod := false
	hasWhen := false

	if alarmInfo != nil {
		if delay, ok := alarmInfo["delayInMinutes"].(float64); ok && delay > 0 {
			delayInMinutes = delay
			hasDelay = true
		}
		if period, ok := alarmInfo["periodInMinutes"].(float64); ok && period > 0 {
			periodInMinutes = period
			hasPeriod = true
		}
		if w, ok := alarmInfo["when"].(float64); ok && w > 0 {
			when = w
			hasWhen = true
		}
	}

	// Validate: can't have both delayInMinutes and when
	if hasDelay && hasWhen {
		return fmt.Errorf("alarms.create(): both 'when' and 'delayInMinutes' cannot be set")
	}

	// Calculate scheduled time and initial delay
	var scheduledTime float64
	var initialDelay time.Duration

	now := time.Now()
	nowMs := float64(now.UnixMilli())

	switch {
	case hasDelay:
		// Schedule after delay
		initialDelay = time.Duration(delayInMinutes*60*1000) * time.Millisecond
		scheduledTime = nowMs + (delayInMinutes * 60 * 1000)
	case hasWhen:
		// Schedule at specific time
		if when < nowMs {
			// Time in the past, fire immediately
			initialDelay = 0
			scheduledTime = nowMs
		} else {
			initialDelay = time.Duration(when-nowMs) * time.Millisecond
			scheduledTime = when
		}
	default:
		// No delay or when specified, fire immediately
		initialDelay = 0
		scheduledTime = nowMs
	}

	// Create alarm object
	alarm := Alarm{
		Name:          name,
		ScheduledTime: scheduledTime,
	}
	if hasPeriod {
		alarm.PeriodInMinutes = &periodInMinutes
	}

	// Clear existing alarm with same name if any
	if existing, exists := d.alarms[extensionID][name]; exists {
		d.stopAlarmInternal(existing)
		delete(d.alarms[extensionID], name)
	}

	// Create internal alarm state
	internal := &alarmInternal{
		alarm:         alarm,
		stopChan:      make(chan struct{}),
		extensionID:   extensionID,
		eventCallback: eventCallback,
	}

	// Store alarm
	d.alarms[extensionID][name] = internal

	// Start alarm in goroutine
	go d.runAlarm(internal, initialDelay, periodInMinutes, hasPeriod)

	return nil
}

// runAlarm runs the alarm timer/ticker
func (d *AlarmsAPIDispatcher) runAlarm(internal *alarmInternal, initialDelay time.Duration, periodInMinutes float64, hasPeriod bool) {
	// Wait for initial delay
	if initialDelay > 0 {
		internal.timer = time.NewTimer(initialDelay)
		select {
		case <-internal.timer.C:
			// Timer fired
		case <-internal.stopChan:
			// Alarm was cleared
			return
		}
	}

	// Fire the alarm
	if internal.eventCallback != nil {
		internal.eventCallback(internal.alarm)
	}

	// If one-time alarm, remove it
	if !hasPeriod {
		d.mu.Lock()
		if alarms, exists := d.alarms[internal.extensionID]; exists {
			delete(alarms, internal.alarm.Name)
		}
		d.mu.Unlock()
		return
	}

	// For repeating alarms, start ticker
	period := time.Duration(periodInMinutes*60*1000) * time.Millisecond
	internal.ticker = time.NewTicker(period)

	for {
		select {
		case <-internal.ticker.C:
			// Update scheduled time
			internal.alarm.ScheduledTime = float64(time.Now().UnixMilli())

			// Fire the alarm
			if internal.eventCallback != nil {
				internal.eventCallback(internal.alarm)
			}

		case <-internal.stopChan:
			// Alarm was cleared
			internal.ticker.Stop()
			return
		}
	}
}

// stopAlarmInternal stops an alarm's timer/ticker
func (d *AlarmsAPIDispatcher) stopAlarmInternal(internal *alarmInternal) {
	close(internal.stopChan)
	if internal.timer != nil {
		internal.timer.Stop()
	}
	if internal.ticker != nil {
		internal.ticker.Stop()
	}
}

// Get retrieves a specific alarm by name
func (d *AlarmsAPIDispatcher) Get(ctx context.Context, extensionID string, nameArg interface{}) (*Alarm, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	// Extract alarm name (optional, defaults to empty string)
	name := ""
	if nameArg != nil {
		if nameStr, ok := nameArg.(string); ok {
			name = nameStr
		}
	}

	// Get extension's alarms
	alarms, exists := d.alarms[extensionID]
	if !exists {
		return nil, nil // No alarms for this extension
	}

	// Get specific alarm
	internal, exists := alarms[name]
	if !exists {
		return nil, nil // Alarm not found
	}

	// Return a copy
	alarm := internal.alarm
	return &alarm, nil
}

// GetAll retrieves all alarms for an extension
func (d *AlarmsAPIDispatcher) GetAll(ctx context.Context, extensionID string) ([]Alarm, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	// Get extension's alarms
	alarms, exists := d.alarms[extensionID]
	if !exists {
		return []Alarm{}, nil // No alarms for this extension
	}

	// Collect all alarms
	result := make([]Alarm, 0, len(alarms))
	for _, internal := range alarms {
		result = append(result, internal.alarm)
	}

	return result, nil
}

// Clear clears a specific alarm by name
func (d *AlarmsAPIDispatcher) Clear(ctx context.Context, extensionID string, nameArg interface{}) (bool, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	// Extract alarm name (optional, defaults to empty string)
	name := ""
	if nameArg != nil {
		if nameStr, ok := nameArg.(string); ok {
			name = nameStr
		}
	}

	// Get extension's alarms
	alarms, exists := d.alarms[extensionID]
	if !exists {
		return false, nil // No alarms for this extension
	}

	// Get specific alarm
	internal, exists := alarms[name]
	if !exists {
		return false, nil // Alarm not found
	}

	// Stop and remove alarm
	d.stopAlarmInternal(internal)
	delete(alarms, name)

	return true, nil
}

// ClearAll clears all alarms for an extension
func (d *AlarmsAPIDispatcher) ClearAll(ctx context.Context, extensionID string) (bool, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	// Get extension's alarms
	alarms, exists := d.alarms[extensionID]
	if !exists || len(alarms) == 0 {
		return false, nil // No alarms for this extension
	}

	// Stop all alarms
	for _, internal := range alarms {
		d.stopAlarmInternal(internal)
	}

	// Clear the map
	delete(d.alarms, extensionID)

	return true, nil
}

// CleanupExtension removes all alarms for an extension (called when extension is unloaded)
func (d *AlarmsAPIDispatcher) CleanupExtension(extensionID string) {
	d.ClearAll(context.Background(), extensionID)
}
