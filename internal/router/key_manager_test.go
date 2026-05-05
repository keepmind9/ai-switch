package router

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestKeyManager_GetKey_Primary(t *testing.T) {
	m := NewKeyManager()
	m.RegisterProvider("test", "primary-key", []string{"fallback1", "fallback2"})

	key, ok := m.GetKey("test")
	assert.True(t, ok)
	assert.Equal(t, "primary-key", key)
}

func TestKeyManager_GetKey_FallbackWhenPrimary429(t *testing.T) {
	m := NewKeyManager()
	m.RegisterProvider("test", "primary", []string{"fallback1", "fallback2"})

	// Simulate 3 consecutive 429s on primary → cooldown
	for i := 0; i < 3; i++ {
		m.Mark429("test", "primary")
	}

	key, ok := m.GetKey("test")
	assert.True(t, ok)
	assert.Equal(t, "fallback1", key)
}

func TestKeyManager_GetKey_AllKeys429(t *testing.T) {
	m := NewKeyManager()
	m.RegisterProvider("test", "primary", []string{"fallback1"})

	// Cooldown all keys
	for i := 0; i < 3; i++ {
		m.Mark429("test", "primary")
	}
	for i := 0; i < 3; i++ {
		m.Mark429("test", "fallback1")
	}

	_, ok := m.GetKey("test")
	assert.False(t, ok, "all keys in cooldown should return false")
}

func TestKeyManager_GetKey_CooldownExpires(t *testing.T) {
	m := NewKeyManager()
	m.RegisterProvider("test", "primary", []string{"fallback1"})

	// Cooldown primary
	for i := 0; i < 3; i++ {
		m.Mark429("test", "primary")
	}

	// Should get fallback
	key, ok := m.GetKey("test")
	assert.True(t, ok)
	assert.Equal(t, "fallback1", key)

	// Manually expire cooldown
	m.mu.Lock()
	s := m.states["test"]
	s.cooldowns[0] = time.Now().Add(-time.Second)
	m.mu.Unlock()

	// Should get primary again
	key, ok = m.GetKey("test")
	assert.True(t, ok)
	assert.Equal(t, "primary", key)
}

func TestKeyManager_Mark429_ConsecutiveReset(t *testing.T) {
	m := NewKeyManager()
	m.RegisterProvider("test", "primary", nil)

	// 2 consecutive 429s — not enough for cooldown
	cooled := m.Mark429("test", "primary")
	assert.False(t, cooled)
	cooled = m.Mark429("test", "primary")
	assert.False(t, cooled)

	// Reset (success)
	m.ResetKey("test", "primary")

	// 2 more 429s — counter was reset, should not cooldown
	cooled = m.Mark429("test", "primary")
	assert.False(t, cooled)
	cooled = m.Mark429("test", "primary")
	assert.False(t, cooled)
}

func TestKeyManager_Mark429_TriggersCooldown(t *testing.T) {
	m := NewKeyManager()
	m.RegisterProvider("test", "primary", nil)

	for i := 0; i < 2; i++ {
		cooled := m.Mark429("test", "primary")
		assert.False(t, cooled)
	}
	// 3rd consecutive 429 triggers cooldown
	cooled := m.Mark429("test", "primary")
	assert.True(t, cooled)

	// Key should be unavailable
	_, ok := m.GetKey("test")
	assert.False(t, ok)
}

func TestKeyManager_ResetKey(t *testing.T) {
	m := NewKeyManager()
	m.RegisterProvider("test", "primary", nil)

	// 2 429s
	m.Mark429("test", "primary")
	m.Mark429("test", "primary")

	// Success resets counter
	m.ResetKey("test", "primary")

	// 2 more 429s — should NOT trigger cooldown (counter was reset)
	cooled := m.Mark429("test", "primary")
	assert.False(t, cooled)
}

func TestKeyManager_AllKeys(t *testing.T) {
	m := NewKeyManager()
	m.RegisterProvider("test", "primary", []string{"fb1", "fb2"})

	keys := m.AllKeys("test")
	require.Len(t, keys, 3)
	assert.Equal(t, "primary", keys[0])
	assert.Equal(t, "fb1", keys[1])
	assert.Equal(t, "fb2", keys[2])
}

func TestKeyManager_UnknownProvider(t *testing.T) {
	m := NewKeyManager()
	_, ok := m.GetKey("unknown")
	assert.False(t, ok)
}

func TestKeyManager_NoFallbackKeys(t *testing.T) {
	m := NewKeyManager()
	m.RegisterProvider("test", "primary", nil)

	// Cooldown the only key
	for i := 0; i < 3; i++ {
		m.Mark429("test", "primary")
	}

	_, ok := m.GetKey("test")
	assert.False(t, ok)
}

// --- New tests for HasFallbackKeys ---

func TestKeyManager_HasFallbackKeys_True(t *testing.T) {
	m := NewKeyManager()
	m.RegisterProvider("test", "primary", []string{"fb1"})
	assert.True(t, m.HasFallbackKeys("test"))
}

func TestKeyManager_HasFallbackKeys_False(t *testing.T) {
	m := NewKeyManager()
	m.RegisterProvider("test", "primary", nil)
	assert.False(t, m.HasFallbackKeys("test"))
}

func TestKeyManager_HasFallbackKeys_UnknownProvider(t *testing.T) {
	m := NewKeyManager()
	assert.False(t, m.HasFallbackKeys("unknown"))
}

func TestKeyManager_HasFallbackKeys_EmptySlice(t *testing.T) {
	m := NewKeyManager()
	m.RegisterProvider("test", "primary", []string{})
	assert.False(t, m.HasFallbackKeys("test"))
}

// --- Tests for SyncProviders ---

func TestKeyManager_SyncProviders_ReplacesAll(t *testing.T) {
	m := NewKeyManager()
	m.RegisterProvider("old", "key-old", nil)

	m.SyncProviders([]ProviderKeys{
		{Provider: "new", PrimaryKey: "key-new", FallbackKeys: []string{"fb1"}},
	})

	// Old provider should be gone
	_, ok := m.GetKey("old")
	assert.False(t, ok, "old provider should be removed after SyncProviders")

	// New provider should work
	key, ok := m.GetKey("new")
	assert.True(t, ok)
	assert.Equal(t, "key-new", key)
}

func TestKeyManager_SyncProviders_ResetsCooldowns(t *testing.T) {
	m := NewKeyManager()
	m.RegisterProvider("test", "primary", []string{"fb1"})

	// Cooldown primary
	for i := 0; i < 3; i++ {
		m.Mark429("test", "primary")
	}

	// Verify it's in cooldown
	key, ok := m.GetKey("test")
	assert.True(t, ok)
	assert.Equal(t, "fb1", key)

	// Re-sync same provider
	m.SyncProviders([]ProviderKeys{
		{Provider: "test", PrimaryKey: "primary", FallbackKeys: []string{"fb1"}},
	})

	// Cooldowns should be reset — primary should be available again
	key, ok = m.GetKey("test")
	assert.True(t, ok)
	assert.Equal(t, "primary", key)
}

func TestKeyManager_SyncProviders_Empty(t *testing.T) {
	m := NewKeyManager()
	m.RegisterProvider("test", "primary", nil)

	m.SyncProviders(nil)

	_, ok := m.GetKey("test")
	assert.False(t, ok, "all providers should be removed")
}

// --- Test for 429 fallback sequence ---

func TestKeyManager_FallbackSequence(t *testing.T) {
	// Simulate a realistic 429 retry sequence:
	// 1. Primary gets 429 three times → cooled down
	// 2. Fallback1 gets 429 three times → cooled down
	// 3. Fallback2 is available
	m := NewKeyManager()
	m.RegisterProvider("test", "primary", []string{"fb1", "fb2"})

	// Drain primary with 3 consecutive 429s
	for i := 0; i < 3; i++ {
		m.Mark429("test", "primary")
	}

	// GetKey should return fb1
	key, ok := m.GetKey("test")
	assert.True(t, ok)
	assert.Equal(t, "fb1", key)

	// Drain fb1
	for i := 0; i < 3; i++ {
		m.Mark429("test", "fb1")
	}

	// GetKey should return fb2
	key, ok = m.GetKey("test")
	assert.True(t, ok)
	assert.Equal(t, "fb2", key)

	// Success on fb2 → reset
	m.ResetKey("test", "fb2")

	// fb2 should still be available
	key, ok = m.GetKey("test")
	assert.True(t, ok)
	assert.Equal(t, "fb2", key)
}

// --- Test for per-request key rotation ---

func TestKeyManager_RequestKeyRotation(t *testing.T) {
	// Simulate: each request gets a fresh key from GetKey,
	// 429 marks the key but doesn't cooldown immediately.
	// The key is still available for this request's next GetKey call
	// but the caller uses triedKeys to avoid retrying.
	m := NewKeyManager()
	m.RegisterProvider("test", "primary", []string{"fb1"})

	// First call returns primary
	key, ok := m.GetKey("test")
	assert.True(t, ok)
	assert.Equal(t, "primary", key)

	// Simulate 429 on primary (1st consecutive)
	cooled := m.Mark429("test", "primary")
	assert.False(t, cooled, "first 429 should not trigger cooldown")

	// GetKey still returns primary (not in cooldown yet)
	key, ok = m.GetKey("test")
	assert.True(t, ok)
	assert.Equal(t, "primary", key)

	// But the caller (stepForward) uses triedKeys to skip it
	// and would need to call GetKey again — but in practice
	// triedKeys prevents this. Let's verify fb1 is reachable
	// by cooling down primary first.
	for i := 0; i < 2; i++ {
		m.Mark429("test", "primary")
	}
	// Now primary is cooled down (3 total)

	key, ok = m.GetKey("test")
	assert.True(t, ok)
	assert.Equal(t, "fb1", key)
}

// --- Concurrency test ---

func TestKeyManager_ConcurrentAccess(t *testing.T) {
	m := NewKeyManager()
	m.RegisterProvider("test", "primary", []string{"fb1", "fb2"})

	var wg sync.WaitGroup
	const goroutines = 50

	// Concurrent access should not panic or race.
	// Some GetKey calls may fail when all keys are cooled down — that's expected.
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				key, ok := m.GetKey("test")
				if ok {
					m.Mark429("test", key)
					m.ResetKey("test", key)
				}
			}
		}()
	}

	wg.Wait()
	// No assertion needed — this test checks for race conditions and panics.
	// Run with -race to detect data races.
}

// --- Edge case: Mark429 on unknown key ---

func TestKeyManager_Mark429_UnknownKey(t *testing.T) {
	m := NewKeyManager()
	m.RegisterProvider("test", "primary", nil)

	cooled := m.Mark429("test", "nonexistent-key")
	assert.False(t, cooled)
}

func TestKeyManager_ResetKey_UnknownKey(t *testing.T) {
	m := NewKeyManager()
	m.RegisterProvider("test", "primary", nil)

	// Should not panic
	m.ResetKey("test", "nonexistent-key")
}

// --- Edge case: empty primary key ---

func TestKeyManager_EmptyPrimaryKey(t *testing.T) {
	m := NewKeyManager()
	m.RegisterProvider("test", "", []string{"fb1"})

	key, ok := m.GetKey("test")
	assert.True(t, ok)
	assert.Equal(t, "fb1", key, "should skip empty primary key")
}

func TestKeyManager_AllKeysEmpty(t *testing.T) {
	m := NewKeyManager()
	m.RegisterProvider("test", "", nil)

	keys := m.AllKeys("test")
	require.Len(t, keys, 1)
	assert.Equal(t, "", keys[0])

	// GetKey should return false (only key is empty)
	_, ok := m.GetKey("test")
	assert.False(t, ok)
}
