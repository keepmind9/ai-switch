package main

import (
	"bytes"
	"errors"
	"fmt"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- helpers ---

func findCommand(cmd *cobra.Command, name string) *cobra.Command {
	for _, child := range cmd.Commands() {
		if child.Name() == name {
			return child
		}
	}
	return nil
}

func executeCommand(root *cobra.Command, args ...string) (string, error) {
	buf := new(bytes.Buffer)
	root.SetOut(buf)
	root.SetErr(buf)
	root.SetArgs(args)
	err := root.Execute()
	return buf.String(), err
}

// --- structural tests ---

func TestNewRootCmdConfiguresCommands(t *testing.T) {
	cmd := newRootCmd()

	assert.Equal(t, "ai-switch", cmd.Use)
	require.NotNil(t, cmd.PersistentFlags().Lookup("config"))
	assert.NotNil(t, findCommand(cmd, "serve"))
	assert.NotNil(t, findCommand(cmd, "stop"))
	assert.NotNil(t, findCommand(cmd, "check"))
	assert.NotNil(t, findCommand(cmd, "version"))
}

func TestNewServeCmdConfiguresDaemonFlag(t *testing.T) {
	cmd := newServeCmd("")

	assert.Equal(t, "serve", cmd.Use)
	require.NotNil(t, cmd.Flags().Lookup("daemon"))
}

func TestAllCommandsUseRunE(t *testing.T) {
	root := newRootCmd()
	for _, child := range root.Commands() {
		assert.NotNilf(t, child.RunE, "%q should use RunE, not Run", child.Name())
	}
}

func TestRootConfigFlagIsInheritedBySubcommands(t *testing.T) {
	cmd := newRootCmd()
	configFlag := cmd.PersistentFlags().Lookup("config")
	require.NotNil(t, configFlag)
	assert.Equal(t, "c", configFlag.Shorthand)

	serveCmd := findCommand(cmd, "serve")
	require.NotNil(t, serveCmd)
	// Inherited persistent flags are accessible via Flag(), not PersistentFlags()
	assert.NotNil(t, serveCmd.Flag("config"))
}

func TestServeDaemonFlagDefaults(t *testing.T) {
	cmd := newServeCmd("")
	daemonFlag := cmd.Flags().Lookup("daemon")
	require.NotNil(t, daemonFlag)
	assert.Equal(t, "false", daemonFlag.DefValue)
	assert.Equal(t, "d", daemonFlag.Shorthand)
}

// --- behavioral tests ---

func TestVersionCommandOutput(t *testing.T) {
	output, err := executeCommand(newRootCmd(), "version")
	require.NoError(t, err)
	assert.Contains(t, output, "Version:")
	assert.Contains(t, output, "Go version:")
}

func TestVersionCommandFormat(t *testing.T) {
	output, err := executeCommand(newRootCmd(), "version")
	require.NoError(t, err)
	assert.Contains(t, output, fmt.Sprintf("Go version: %s", "go"))
	assert.Contains(t, output, "OS/Arch:")
}

func TestStopCommandReturnsRunEError(t *testing.T) {
	// stop should use RunE so errors propagate through Cobra
	cmd := newStopCmd()
	require.NotNil(t, cmd.RunE)
}

func TestCheckCommandReturnsRunEError(t *testing.T) {
	cmd := newCheckCmd("")
	require.NotNil(t, cmd.RunE)
}

func TestErrExitCode(t *testing.T) {
	e := errExitCode{code: 2}
	assert.Equal(t, "exit code 2", e.Error())
}

func TestIsAddrInUse(t *testing.T) {
	assert.True(t, isAddrInUse(errors.New("bind: address already in use")))
	assert.False(t, isAddrInUse(errors.New("connection refused")))
}
