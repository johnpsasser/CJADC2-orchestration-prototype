// Package agent provides the base framework for CJADC2 agents
package agent

import (
	"context"
	"fmt"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
)

// AgentType identifies the type of agent
type AgentType string

const (
	AgentTypeSensor     AgentType = "sensor"
	AgentTypeClassifier AgentType = "classifier"
	AgentTypeCorrelator AgentType = "correlator"
	AgentTypePlanner    AgentType = "planner"
	AgentTypeAuthorizer AgentType = "authorizer"
	AgentTypeEffector   AgentType = "effector"
)

// HealthStatus represents agent health
type HealthStatus struct {
	Healthy bool   `json:"healthy"`
	Status  string `json:"status"`
	Details string `json:"details,omitempty"`
}

// Agent is the interface that all agents must implement
type Agent interface {
	// Identity
	ID() string
	Type() AgentType

	// Lifecycle
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
	Health() HealthStatus

	// Metrics
	Metrics() *prometheus.Registry
}

// Config holds configuration for an agent
type Config struct {
	ID        string
	Type      AgentType
	NATSUrl   string
	OPAUrl    string
	DBUrl     string
	OTELUrl   string
	Secret    []byte
	ExtraVars map[string]string
}

// Factory creates agents of a specific type
type Factory func(cfg Config) (Agent, error)

// Registry manages agent factories
type Registry struct {
	mu        sync.RWMutex
	factories map[AgentType]Factory
}

var globalRegistry = &Registry{
	factories: make(map[AgentType]Factory),
}

// Register adds a new agent factory to the global registry
func Register(agentType AgentType, factory Factory) {
	globalRegistry.mu.Lock()
	defer globalRegistry.mu.Unlock()
	globalRegistry.factories[agentType] = factory
}

// Create instantiates an agent of the given type
func Create(agentType AgentType, cfg Config) (Agent, error) {
	globalRegistry.mu.RLock()
	factory, ok := globalRegistry.factories[agentType]
	globalRegistry.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("unknown agent type: %s", agentType)
	}

	return factory(cfg)
}

// ListTypes returns all registered agent types
func ListTypes() []AgentType {
	globalRegistry.mu.RLock()
	defer globalRegistry.mu.RUnlock()

	types := make([]AgentType, 0, len(globalRegistry.factories))
	for t := range globalRegistry.factories {
		types = append(types, t)
	}
	return types
}
