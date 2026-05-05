package router

import (
	"sync"
	"time"
)

const (
	maxConsecutive429 = 3
	cooldownDuration  = 60 * time.Second
)

// ProviderKeys holds the keys for a single provider for bulk sync.
type ProviderKeys struct {
	Provider     string
	PrimaryKey   string
	FallbackKeys []string
}

// KeyManager manages API key selection with 429 tracking and cooldown.
// Thread-safe. Each provider has its own key state.
type KeyManager struct {
	mu     sync.Mutex
	states map[string]*providerKeyState
}

type providerKeyState struct {
	keys      []string          // [primary, fallback1, fallback2, ...]
	consec429 map[int]int       // key index → consecutive 429 count
	cooldowns map[int]time.Time // key index → cooldown expiry
}

// NewKeyManager creates an empty KeyManager.
func NewKeyManager() *KeyManager {
	return &KeyManager{states: make(map[string]*providerKeyState)}
}

// RegisterProvider registers the primary and fallback keys for a provider.
func (m *KeyManager) RegisterProvider(provider string, primaryKey string, fallbackKeys []string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	keys := []string{primaryKey}
	keys = append(keys, fallbackKeys...)
	m.states[provider] = &providerKeyState{
		keys:      keys,
		consec429: make(map[int]int),
		cooldowns: make(map[int]time.Time),
	}
}

// GetKey returns the best available key for the given provider.
// Priority: primary → fallback keys in order. Skips keys in cooldown.
// Returns ("", false) if all keys are in cooldown or provider not found.
func (m *KeyManager) GetKey(provider string) (string, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	s, ok := m.states[provider]
	if !ok {
		return "", false
	}

	now := time.Now()
	for i, key := range s.keys {
		if key == "" {
			continue
		}
		if expiry, cooled := s.cooldowns[i]; cooled && now.Before(expiry) {
			continue
		}
		// Clean expired cooldown
		delete(s.cooldowns, i)
		return key, true
	}
	return "", false
}

// Mark429 records a 429 for the given key. Returns true if the key was cooled down.
func (m *KeyManager) Mark429(provider, apiKey string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	s, ok := m.states[provider]
	if !ok {
		return false
	}

	idx := s.keyIndex(apiKey)
	if idx < 0 {
		return false
	}

	s.consec429[idx]++
	if s.consec429[idx] >= maxConsecutive429 {
		s.cooldowns[idx] = time.Now().Add(cooldownDuration)
		s.consec429[idx] = 0
		return true
	}
	return false
}

// ResetKey clears consecutive 429 count for the given key (called on success).
func (m *KeyManager) ResetKey(provider, apiKey string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	s, ok := m.states[provider]
	if !ok {
		return
	}

	idx := s.keyIndex(apiKey)
	if idx >= 0 {
		delete(s.consec429, idx)
	}
}

// HasFallbackKeys returns true if the provider has fallback keys registered.
func (m *KeyManager) HasFallbackKeys(provider string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	s, ok := m.states[provider]
	if !ok {
		return false
	}
	return len(s.keys) > 1
}

// SyncProviders rebuilds all provider key states from the given map.
// Providers not in the map are removed. This is safe to call on reload.
func (m *KeyManager) SyncProviders(entries []ProviderKeys) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Build new states from scratch to remove stale providers.
	newStates := make(map[string]*providerKeyState, len(entries))
	for _, e := range entries {
		keys := []string{e.PrimaryKey}
		keys = append(keys, e.FallbackKeys...)
		newStates[e.Provider] = &providerKeyState{
			keys:      keys,
			consec429: make(map[int]int),
			cooldowns: make(map[int]time.Time),
		}
	}
	m.states = newStates
}

// AllKeys returns all registered keys for a provider (primary + fallbacks).
func (m *KeyManager) AllKeys(provider string) []string {
	m.mu.Lock()
	defer m.mu.Unlock()

	s, ok := m.states[provider]
	if !ok {
		return nil
	}
	out := make([]string, len(s.keys))
	copy(out, s.keys)
	return out
}

func (s *providerKeyState) keyIndex(apiKey string) int {
	for i, k := range s.keys {
		if k == apiKey {
			return i
		}
	}
	return -1
}
