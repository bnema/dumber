// Package services defines the service interface and common service patterns
package services

// Service is the base interface that all application services must implement
// This allows for consistent service management, lifecycle control, and dependency injection
type Service interface {
	// ServiceName returns the unique identifier for this service
	// Used for logging, debugging, and service registration
	ServiceName() string
}

// Initializable services can be initialized after construction
type Initializable interface {
	Service
	// Initialize performs any setup required after construction
	// Returns an error if initialization fails
	Initialize() error
}

// Cleanable services can cleanup resources on shutdown
type Cleanable interface {
	Service
	// Cleanup releases any resources held by the service
	// Should be idempotent and safe to call multiple times
	Cleanup() error
}

// LifecycleService implements both Initializable and Cleanable
// Use this for services that need both initialization and cleanup
type LifecycleService interface {
	Initializable
	Cleanable
}
