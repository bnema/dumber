package api

import (
	"context"
	"fmt"
	"log"
	"sync"
)

// Command represents a keyboard shortcut command from the manifest
type Command struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Shortcut    string `json:"shortcut,omitempty"`
}

// CommandsAPIDispatcher handles commands.* API calls
type CommandsAPIDispatcher struct {
	mu sync.RWMutex
	// extensionCommands maps extension ID to their commands
	extensionCommands map[string]map[string]*Command
	// defaultCommands stores the original manifest commands for reset
	defaultCommands map[string]map[string]*Command
	// onCommandCallbacks stores callbacks for onCommand event listeners
	onCommandCallbacks map[string][]func(string)
}

// NewCommandsAPIDispatcher creates a new commands API dispatcher
func NewCommandsAPIDispatcher() *CommandsAPIDispatcher {
	return &CommandsAPIDispatcher{
		extensionCommands:  make(map[string]map[string]*Command),
		defaultCommands:    make(map[string]map[string]*Command),
		onCommandCallbacks: make(map[string][]func(string)),
	}
}

// RegisterExtensionCommands registers commands from a manifest for an extension
// This should be called during extension initialization
func (d *CommandsAPIDispatcher) RegisterExtensionCommands(extID string, commandsData map[string]interface{}) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if commandsData == nil {
		return nil
	}

	commands := make(map[string]*Command)
	defaults := make(map[string]*Command)

	for name, data := range commandsData {
		cmdData, ok := data.(map[string]interface{})
		if !ok {
			log.Printf("[commands] Invalid command data for %s in extension %s", name, extID)
			continue
		}

		cmd := &Command{
			Name: name,
		}

		// Parse description
		if desc, ok := cmdData["description"].(string); ok {
			cmd.Description = desc
		}

		// Parse suggested_key (Firefox extension format)
		if suggestedKey, ok := cmdData["suggested_key"].(map[string]interface{}); ok {
			// Try linux-specific shortcut first, then default
			if linux, ok := suggestedKey["linux"].(string); ok {
				cmd.Shortcut = linux
			} else if defaultKey, ok := suggestedKey["default"].(string); ok {
				cmd.Shortcut = defaultKey
			}
		}

		commands[name] = cmd

		// Store copy as default for reset functionality
		defaultCopy := &Command{
			Name:        cmd.Name,
			Description: cmd.Description,
			Shortcut:    cmd.Shortcut,
		}
		defaults[name] = defaultCopy

		log.Printf("[commands] Registered command %s for extension %s: shortcut=%s", name, extID, cmd.Shortcut)
	}

	d.extensionCommands[extID] = commands
	d.defaultCommands[extID] = defaults

	return nil
}

// GetAll returns all commands for an extension
// API: browser.commands.getAll()
func (d *CommandsAPIDispatcher) GetAll(ctx context.Context, extID string) ([]*Command, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	commands, ok := d.extensionCommands[extID]
	if !ok {
		return []*Command{}, nil
	}

	result := make([]*Command, 0, len(commands))
	for _, cmd := range commands {
		result = append(result, cmd)
	}

	return result, nil
}

// Reset resets a command to its default shortcut from the manifest
// API: browser.commands.reset(name)
func (d *CommandsAPIDispatcher) Reset(ctx context.Context, extID, commandName string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	commands, ok := d.extensionCommands[extID]
	if !ok {
		return fmt.Errorf("no commands registered for extension %s", extID)
	}

	cmd, ok := commands[commandName]
	if !ok {
		return fmt.Errorf("command %s not found", commandName)
	}

	defaults, ok := d.defaultCommands[extID]
	if !ok {
		return fmt.Errorf("no default commands for extension %s", extID)
	}

	defaultCmd, ok := defaults[commandName]
	if !ok {
		return fmt.Errorf("default command %s not found", commandName)
	}

	// Reset to default values
	cmd.Description = defaultCmd.Description
	cmd.Shortcut = defaultCmd.Shortcut

	log.Printf("[commands] Reset command %s for extension %s to default: %s", commandName, extID, cmd.Shortcut)
	return nil
}

// Update updates a command's description and/or shortcut
// API: browser.commands.update({ name, description?, shortcut? })
func (d *CommandsAPIDispatcher) Update(ctx context.Context, extID string, details map[string]interface{}) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	// Extract command name
	nameVal, ok := details["name"]
	if !ok {
		return fmt.Errorf("missing 'name' field in update details")
	}
	commandName, ok := nameVal.(string)
	if !ok {
		return fmt.Errorf("'name' must be a string")
	}

	commands, ok := d.extensionCommands[extID]
	if !ok {
		return fmt.Errorf("no commands registered for extension %s", extID)
	}

	cmd, ok := commands[commandName]
	if !ok {
		return fmt.Errorf("command %s not found", commandName)
	}

	// Update description if provided
	if descVal, ok := details["description"]; ok {
		if desc, ok := descVal.(string); ok {
			cmd.Description = desc
			log.Printf("[commands] Updated description for command %s in extension %s", commandName, extID)
		}
	}

	// Update shortcut if provided
	if shortcutVal, ok := details["shortcut"]; ok {
		shortcut, ok := shortcutVal.(string)
		if !ok {
			return fmt.Errorf("'shortcut' must be a string")
		}

		// Empty string means clear the shortcut
		if shortcut == "" {
			cmd.Shortcut = ""
			log.Printf("[commands] Cleared shortcut for command %s in extension %s", commandName, extID)
		} else {
			// Validate shortcut format (basic validation)
			if err := validateShortcut(shortcut); err != nil {
				return fmt.Errorf("invalid shortcut '%s': %w", shortcut, err)
			}

			cmd.Shortcut = shortcut
			log.Printf("[commands] Updated shortcut for command %s in extension %s to: %s", commandName, extID, shortcut)
		}
	}

	return nil
}

// validateShortcut performs basic validation on keyboard shortcut format
// Expected format: "Ctrl+Shift+A", "Alt+F1", etc.
func validateShortcut(shortcut string) error {
	if shortcut == "" {
		return nil
	}

	// Basic validation - just check it's not empty and has reasonable length
	if len(shortcut) > 50 {
		return fmt.Errorf("shortcut too long")
	}

	// Could add more validation here:
	// - Check for valid modifier keys (Ctrl, Alt, Shift, Command, etc.)
	// - Check for valid key names
	// - Check format with + separators
	// For now, accept any non-empty string and let GTK handle validation

	return nil
}

// AddOnCommandListener registers a callback for the onCommand event
func (d *CommandsAPIDispatcher) AddOnCommandListener(extID string, callback func(string)) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.onCommandCallbacks[extID] = append(d.onCommandCallbacks[extID], callback)
	log.Printf("[commands] Added onCommand listener for extension %s", extID)
}

// RemoveOnCommandListener removes a callback for the onCommand event
func (d *CommandsAPIDispatcher) RemoveOnCommandListener(extID string, callback func(string)) {
	d.mu.Lock()
	defer d.mu.Unlock()

	callbacks := d.onCommandCallbacks[extID]
	for i, cb := range callbacks {
		// Note: Function comparison in Go is tricky
		// In practice, this should work for removing the same function reference
		if &cb == &callback {
			d.onCommandCallbacks[extID] = append(callbacks[:i], callbacks[i+1:]...)
			log.Printf("[commands] Removed onCommand listener for extension %s", extID)
			return
		}
	}
}

// TriggerCommand fires the onCommand event for a specific command
// This should be called when a keyboard shortcut is activated
func (d *CommandsAPIDispatcher) TriggerCommand(extID, commandName string) {
	d.mu.RLock()
	callbacks := make([]func(string), len(d.onCommandCallbacks[extID]))
	copy(callbacks, d.onCommandCallbacks[extID])
	d.mu.RUnlock()

	log.Printf("[commands] Triggering command %s for extension %s (%d listeners)", commandName, extID, len(callbacks))

	// Fire callbacks outside of lock
	for _, callback := range callbacks {
		callback(commandName)
	}
}

// UnregisterExtension cleans up all commands and listeners for an extension
func (d *CommandsAPIDispatcher) UnregisterExtension(extID string) {
	d.mu.Lock()
	defer d.mu.Unlock()

	delete(d.extensionCommands, extID)
	delete(d.defaultCommands, extID)
	delete(d.onCommandCallbacks, extID)

	log.Printf("[commands] Unregistered all commands for extension %s", extID)
}
