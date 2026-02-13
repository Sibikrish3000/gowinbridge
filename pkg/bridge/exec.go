package bridge

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/sibikrish3000/gowinbridge/internal/wsl"
	"golang.org/x/term"
)

// wslChecked guards the one-time WSL environment validation.
var (
	wslCheckOnce sync.Once
	wslCheckErr  error
)

// validateWSL ensures we are running inside WSL. It is called once.
func validateWSL() error {
	wslCheckOnce.Do(func() {
		if !wsl.IsWSL() {
			wslCheckErr = fmt.Errorf("gowinbridge: not running in a WSL environment")
		}
	})
	return wslCheckErr
}

// resolveCommand ensures the command has a .exe extension.
// If the command does not end in .exe, it attempts to find the .exe variant on PATH.
func resolveCommand(command string) string {
	if strings.HasSuffix(strings.ToLower(command), ".exe") {
		return command
	}

	// Try appending .exe and check if it exists on PATH.
	withExe := command + ".exe"
	if _, err := exec.LookPath(withExe); err == nil {
		return withExe
	}

	// Fall back to the original command; let exec handle the error.
	return command
}

// convertPathArgs translates arguments that look like file paths from Linux to Windows format.
func convertPathArgs(args []string) ([]string, error) {
	converted := make([]string, len(args))
	for i, arg := range args {
		if looksLikePath(arg) {
			winPath, err := wsl.ToWindowsPath(arg)
			if err != nil {
				return nil, fmt.Errorf("failed to convert argument %q: %w", arg, err)
			}
			converted[i] = winPath
		} else {
			converted[i] = arg
		}
	}
	return converted, nil
}

// looksLikePath returns true if the string might be a file path.
func looksLikePath(s string) bool {
	if s == "" {
		return false
	}
	// Starts with / or ./ or ../ — likely a Linux path.
	return strings.HasPrefix(s, "/") ||
		strings.HasPrefix(s, "./") ||
		strings.HasPrefix(s, "../")
}

// IsTerminal reports whether the given file descriptor is a terminal.
// Exported for use in CLI auto-detection.
func IsTerminal(fd int) bool {
	return term.IsTerminal(fd)
}

// Execute runs a Windows binary from WSL with full lifecycle management.
// It uses exec.CommandContext for signal propagation and supports both
// buffered (Scanner) and interactive (raw copy) stdio modes.
func Execute(ctx context.Context, config CommandConfig) (Output, error) {
	// Validate WSL environment (fail fast).
	if err := validateWSL(); err != nil {
		return Output{}, err
	}

	// Resolve the command to its .exe variant if needed.
	resolvedCmd := resolveCommand(config.Command)

	// Optionally convert path-like arguments.
	args := config.Args
	if config.ConvertPaths {
		var err error
		args, err = convertPathArgs(args)
		if err != nil {
			return Output{}, fmt.Errorf("path conversion failed: %w", err)
		}
	}

	// Apply timeout if configured.
	execCtx := ctx
	if config.Timeout > 0 {
		var cancel context.CancelFunc
		execCtx, cancel = context.WithTimeout(ctx, config.Timeout)
		defer cancel()
	}

	// Build the command.
	cmd := exec.CommandContext(execCtx, resolvedCmd, args...)

	// Set working directory.
	if config.WorkDir != "" {
		cmd.Dir = config.WorkDir
	}

	// Prepare environment.
	cmd.Env = PrepareEnv(config)

	// Interactive mode: direct stdio copy, no buffering.
	if config.Interactive {
		return executeInteractive(cmd, config)
	}

	// Buffered mode: capture output with optional encoding.
	return executeBuffered(cmd, config)
}

// executeInteractive runs the command with direct stdin/stdout/stderr piping.
// This supports REPLs, TUI apps, and progress bars.
func executeInteractive(cmd *exec.Cmd, config CommandConfig) (Output, error) {
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if config.Stdin != nil {
		cmd.Stdin = config.Stdin
	} else {
		cmd.Stdin = os.Stdin
	}

	start := time.Now()

	if err := cmd.Start(); err != nil {
		return Output{}, fmt.Errorf("failed to start command %q: %w", config.Command, err)
	}

	waitErr := cmd.Wait()
	duration := time.Since(start)

	output := Output{Duration: duration}

	if waitErr != nil {
		if exitErr, ok := waitErr.(*exec.ExitError); ok {
			output.ExitCode = exitErr.ExitCode()
		} else {
			return output, fmt.Errorf("command execution failed: %w", waitErr)
		}
	}

	return output, nil
}

// executeBuffered runs the command with buffered stdio capture and optional encoding.
func executeBuffered(cmd *exec.Cmd, config CommandConfig) (Output, error) {
	// Set up pipes.
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return Output{}, fmt.Errorf("failed to create stdout pipe: %w", err)
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return Output{}, fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	// If stdin is provided in non-interactive mode, pipe it.
	if config.Stdin != nil {
		stdinPipe, err := cmd.StdinPipe()
		if err != nil {
			return Output{}, fmt.Errorf("failed to create stdin pipe: %w", err)
		}
		go func() {
			defer stdinPipe.Close()
			io.Copy(stdinPipe, config.Stdin)
		}()
	}

	// Wrap pipes in encoding decoder if specified.
	var stdoutReader, stderrReader io.Reader
	stdoutReader = stdoutPipe
	stderrReader = stderrPipe

	if config.Encoding != "" {
		stdoutReader, err = NewDecodingReader(stdoutPipe, config.Encoding)
		if err != nil {
			return Output{}, fmt.Errorf("failed to create stdout decoder: %w", err)
		}
		stderrReader, err = NewDecodingReader(stderrPipe, config.Encoding)
		if err != nil {
			return Output{}, fmt.Errorf("failed to create stderr decoder: %w", err)
		}
	}

	start := time.Now()

	if err := cmd.Start(); err != nil {
		return Output{}, fmt.Errorf("failed to start command %q: %w", config.Command, err)
	}

	// Stream stdout and stderr concurrently.
	var stdoutBuf, stderrBuf strings.Builder
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		scanner := bufio.NewScanner(stdoutReader)
		for scanner.Scan() {
			line := scanner.Text()
			stdoutBuf.WriteString(line)
			stdoutBuf.WriteString("\n")
		}
	}()

	go func() {
		defer wg.Done()
		scanner := bufio.NewScanner(stderrReader)
		for scanner.Scan() {
			line := scanner.Text()
			stderrBuf.WriteString(line)
			stderrBuf.WriteString("\n")
		}
	}()

	// Wait for streaming goroutines to finish reading.
	wg.Wait()

	// Wait for the process to exit.
	waitErr := cmd.Wait()
	duration := time.Since(start)

	output := Output{
		Stdout:   strings.TrimRight(stdoutBuf.String(), "\n"),
		Stderr:   strings.TrimRight(stderrBuf.String(), "\n"),
		Duration: duration,
	}

	if waitErr != nil {
		if exitErr, ok := waitErr.(*exec.ExitError); ok {
			output.ExitCode = exitErr.ExitCode()
		} else {
			return output, fmt.Errorf("command execution failed: %w", waitErr)
		}
	}

	return output, nil
}

// ResetWSLCheck resets the WSL validation state (for testing only).
func ResetWSLCheck() {
	wslCheckOnce = sync.Once{}
	wslCheckErr = nil
}
