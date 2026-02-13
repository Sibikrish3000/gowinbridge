// Package bridge provides the core executor for running Windows binaries
// from a WSL environment with path translation, environment bridging,
// and real-time stdio streaming.
package bridge

import "time"

// CommandConfig defines the configuration for executing a Windows binary.
type CommandConfig struct {
	// Command is the binary to execute (e.g., "cmd.exe", "notepad.exe").
	Command string

	// Args are the arguments to pass to the command.
	Args []string

	// Env is a map of environment variables to set for the process.
	Env map[string]string

	// EnvTunneling enables automatic WSLENV formatting for the provided Env keys.
	EnvTunneling bool

	// WorkDir is the working directory for the command.
	// If empty, the current working directory is used.
	WorkDir string

	// Timeout is the maximum duration the command is allowed to run.
	// Zero means no timeout.
	Timeout time.Duration

	// ConvertPaths, when true, translates file-like arguments from Linux
	// to Windows format before execution.
	ConvertPaths bool
}

// Output holds the result of a command execution.
type Output struct {
	// Stdout is the captured standard output.
	Stdout string

	// Stderr is the captured standard error.
	Stderr string

	// ExitCode is the process exit code.
	ExitCode int

	// Duration is the wall-clock time the command took to run.
	Duration time.Duration
}
