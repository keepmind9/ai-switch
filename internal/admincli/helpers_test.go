package admincli

import (
	"context"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
)

func newTestCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "test"}
	cmd.SetContext(context.Background())
	return cmd
}

func TestNeedMinArgs_Pass(t *testing.T) {
	validator := needMinArgs(1, "update <key>")
	err := validator(nil, []string{"openai"})
	assert.NoError(t, err)
}

func TestNeedMinArgs_Fail(t *testing.T) {
	validator := needMinArgs(1, "update <key>")
	err := validator(nil, []string{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "usage:")
}

func TestNeedMinArgs_TwoArgs(t *testing.T) {
	validator := needMinArgs(2, "cmd <a> <b>")
	err := validator(nil, []string{"one"})
	assert.Error(t, err)
	err = validator(nil, []string{"one", "two"})
	assert.NoError(t, err)
}

func TestInjectClient(t *testing.T) {
	client := NewAdminClient("http://test:1234/api")
	cmd := newTestCmd()
	InjectClient(cmd, client)

	got := ClientFromCmd(cmd)
	assert.Equal(t, client, got)
}

func TestJoinSlice(t *testing.T) {
	assert.Equal(t, "-", joinSlice(nil))
	assert.Equal(t, "-", joinSlice([]string{}))
	assert.Equal(t, "a, b, c", joinSlice([]string{"a", "b", "c"}))
}

func TestDashIfEmpty(t *testing.T) {
	assert.Equal(t, "-", dashIfEmpty(""))
	assert.Equal(t, "hello", dashIfEmpty("hello"))
}
