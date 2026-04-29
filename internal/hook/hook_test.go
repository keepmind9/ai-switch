package hook

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestManagerFire_NoHooks(t *testing.T) {
	m := NewManager()
	ctx := &Context{RequestID: "test-123"}

	err := m.Fire(ctx, BeforeRoute)
	assert.NoError(t, err)
}

func TestManagerFire_OptionalHook(t *testing.T) {
	m := NewManager()
	called := false

	m.Register(Hook{
		Name:  "test-hook",
		Point: BeforeRoute,
		Level: Optional,
		Fn: func(ctx *Context) error {
			called = true
			assert.Equal(t, "test-123", ctx.RequestID)
			return nil
		},
	})

	ctx := &Context{RequestID: "test-123"}
	err := m.Fire(ctx, BeforeRoute)
	assert.NoError(t, err)
	assert.True(t, called)
}

func TestManagerFire_OptionalHookError_Continues(t *testing.T) {
	m := NewManager()
	firstCalled := false
	secondCalled := false

	m.Register(Hook{
		Name:  "failing-hook",
		Point: BeforeUpstream,
		Level: Optional,
		Fn: func(ctx *Context) error {
			firstCalled = true
			return errors.New("optional error")
		},
	})
	m.Register(Hook{
		Name:  "next-hook",
		Point: BeforeUpstream,
		Level: Optional,
		Fn: func(ctx *Context) error {
			secondCalled = true
			return nil
		},
	})

	ctx := &Context{RequestID: "test-456"}
	err := m.Fire(ctx, BeforeUpstream)
	assert.NoError(t, err)
	assert.True(t, firstCalled)
	assert.True(t, secondCalled)
}

func TestManagerFire_CriticalHookError_Aborts(t *testing.T) {
	m := NewManager()
	secondCalled := false

	m.Register(Hook{
		Name:  "critical-fail",
		Point: AfterRoute,
		Level: Critical,
		Fn: func(ctx *Context) error {
			return errors.New("critical error")
		},
	})
	m.Register(Hook{
		Name:  "should-not-run",
		Point: AfterRoute,
		Level: Optional,
		Fn: func(ctx *Context) error {
			secondCalled = true
			return nil
		},
	})

	ctx := &Context{RequestID: "test-789"}
	err := m.Fire(ctx, AfterRoute)
	assert.Error(t, err)
	assert.Equal(t, "critical error", err.Error())
	assert.False(t, secondCalled, "hooks after a critical error should not be called")
}

func TestManagerFire_MultiplePoints(t *testing.T) {
	m := NewManager()
	var called []string

	m.Register(Hook{
		Name:  "before-route",
		Point: BeforeRoute,
		Level: Optional,
		Fn: func(ctx *Context) error {
			called = append(called, "before-route")
			return nil
		},
	})
	m.Register(Hook{
		Name:  "after-route",
		Point: AfterRoute,
		Level: Optional,
		Fn: func(ctx *Context) error {
			called = append(called, "after-route")
			return nil
		},
	})

	ctx := &Context{RequestID: "test-multi"}

	err := m.Fire(ctx, BeforeRoute)
	assert.NoError(t, err)
	assert.Equal(t, []string{"before-route"}, called)

	err = m.Fire(ctx, AfterRoute)
	assert.NoError(t, err)
	assert.Equal(t, []string{"before-route", "after-route"}, called)
}
