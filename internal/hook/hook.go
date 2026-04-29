package hook

import (
	"fmt"
	"log/slog"
	"sync"
)

// Point represents a specific moment in the request lifecycle where hooks can fire.
type Point int

const (
	BeforeRoute Point = iota
	AfterRoute
	BeforeUpstream
	AfterUpstream
	AfterResponse
)

// PointName returns a human-readable name for a hook point.
func PointName(p Point) string {
	switch p {
	case BeforeRoute:
		return "BeforeRoute"
	case AfterRoute:
		return "AfterRoute"
	case BeforeUpstream:
		return "BeforeUpstream"
	case AfterUpstream:
		return "AfterUpstream"
	case AfterResponse:
		return "AfterResponse"
	default:
		return fmt.Sprintf("Unknown(%d)", p)
	}
}

// Level determines how hook errors are handled.
type Level int

const (
	Critical Level = iota // Critical hooks abort the pipeline on error.
	Optional              // Optional hooks log errors and continue.
)

// HookFunc is the function signature for hook callbacks.
type HookFunc func(ctx *Context) error

// Hook represents a named callback registered at a specific lifecycle point.
type Hook struct {
	Name  string
	Point Point
	Level Level
	Fn    HookFunc
}

// Manager manages hook registration and dispatching.
type Manager struct {
	mu    sync.RWMutex
	hooks map[Point][]Hook
}

// NewManager creates a new hook Manager.
func NewManager() *Manager {
	return &Manager{hooks: make(map[Point][]Hook)}
}

// Register adds a hook to be fired at its configured point.
func (m *Manager) Register(h Hook) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.hooks[h.Point] = append(m.hooks[h.Point], h)
}

// Fire executes all hooks registered at the given point in order.
// If a Critical hook returns an error, Fire aborts and returns that error.
// If an Optional hook returns an error, it is logged and execution continues.
func (m *Manager) Fire(ctx *Context, point Point) error {
	m.mu.RLock()
	hooks := m.hooks[point]
	m.mu.RUnlock()

	for _, h := range hooks {
		if err := h.Fn(ctx); err != nil {
			if h.Level == Critical {
				return err
			}
			slog.Warn("optional hook failed", "hook", h.Name, "point", PointName(point), "error", err)
		}
	}
	return nil
}
