// winrun is a CLI tool that uses the gowinbridge library to execute
// Windows binaries from WSL with path translation, environment bridging,
// concurrent execution, and graceful signal handling.
//
// Usage:
//
//	winrun [flags] -- <command> [args...]
//	winrun shim <install|list|remove> [options]
//
// Flags:
//
//	--concurrency N    Max concurrent executions (default: NumCPU)
//	--convert-paths    Auto-detect and convert file path arguments
//	--encoding ENC     Output encoding: utf8, cp1252, utf16le, utf16be, auto
//	--env KEY=VAL      Set environment variable (repeatable)
//	--tunnel-env       Enable WSLENV tunneling for --env vars
//	--interactive      Run in interactive/PTY mode (auto-detected)
//	--timeout DURATION Max execution time (e.g., 30s, 5m)
//	--version          Print version and exit
//	--help             Show usage
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/sibikrish3000/gowinbridge/internal/wsl"
	"github.com/sibikrish3000/gowinbridge/pkg/bridge"
	"github.com/sibikrish3000/gowinbridge/pkg/workerpool"
)

// Build-time variables, injected via -ldflags.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

// envFlags collects repeatable --env KEY=VAL flags.
type envFlags []string

func (e *envFlags) String() string { return strings.Join(*e, ", ") }
func (e *envFlags) Set(val string) error {
	*e = append(*e, val)
	return nil
}

func main() {
	// Handle "shim" subcommand before flag parsing.
	if len(os.Args) > 1 && os.Args[1] == "shim" {
		handleShim(os.Args[2:])
		return
	}

	var (
		concurrency  int
		convertPaths bool
		envVars      envFlags
		tunnelEnv    bool
		timeout      time.Duration
		showVersion  bool
		encoding     string
		interactive  bool
	)

	flag.IntVar(&concurrency, "concurrency", runtime.NumCPU(), "Max concurrent executions")
	flag.BoolVar(&convertPaths, "convert-paths", false, "Auto-convert file path arguments to Windows format")
	flag.Var(&envVars, "env", "Set environment variable as KEY=VAL (repeatable)")
	flag.BoolVar(&tunnelEnv, "tunnel-env", false, "Enable WSLENV tunneling for specified env vars")
	flag.DurationVar(&timeout, "timeout", 0, "Max execution time (e.g., 30s, 5m)")
	flag.BoolVar(&showVersion, "version", false, "Print version information and exit")
	flag.StringVar(&encoding, "encoding", "", "Output encoding: utf8, cp1252, utf16le, utf16be, auto")
	flag.BoolVar(&interactive, "interactive", false, "Run in interactive/PTY mode (bypasses output capture)")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: winrun [flags] -- <command> [args...]\n")
		fmt.Fprintf(os.Stderr, "       winrun shim <install|list|remove> [options]\n\n")
		fmt.Fprintf(os.Stderr, "Execute Windows binaries from WSL with path translation and env bridging.\n\n")
		fmt.Fprintf(os.Stderr, "Flags:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nExamples:\n")
		fmt.Fprintf(os.Stderr, "  winrun -- cmd.exe /c echo hello\n")
		fmt.Fprintf(os.Stderr, "  winrun --convert-paths -- cmd.exe /c type ./myfile.txt\n")
		fmt.Fprintf(os.Stderr, "  winrun --encoding cp1252 -- cmd.exe /c chcp\n")
		fmt.Fprintf(os.Stderr, "  winrun -interactive -- python.exe\n")
		fmt.Fprintf(os.Stderr, "  winrun --env MY_VAR=hello --tunnel-env -- cmd.exe /c echo %%MY_VAR%%\n")
		fmt.Fprintf(os.Stderr, "  winrun --concurrency 4 --timeout 30s -- powershell.exe -Command Get-Process\n")
		fmt.Fprintf(os.Stderr, "  winrun shim install docker.exe --as docker\n")
	}

	flag.Parse()

	if showVersion {
		fmt.Printf("winrun %s\n  commit: %s\n  built:  %s\n  go:     %s\n", version, commit, date, runtime.Version())
		os.Exit(0)
	}

	// Find the command after "--" separator.
	args := flag.Args()
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "Error: no command specified. Use '--' to separate flags from the command.")
		fmt.Fprintln(os.Stderr, "Run 'winrun --help' for usage.")
		os.Exit(1)
	}

	// Validate WSL environment.
	if !wsl.IsWSL() {
		fmt.Fprintln(os.Stderr, "Error: winrun must be run inside a WSL environment.")
		wslVer := wsl.DetectWSLVersion()
		if wslVer == 0 {
			fmt.Fprintln(os.Stderr, "  This does not appear to be a WSL instance.")
		}
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "[winrun] WSL%d environment detected\n", wsl.DetectWSLVersion())

	// Auto-detect interactive mode if stdin is a terminal.
	if !interactive && bridge.IsTerminal(int(os.Stdin.Fd())) {
		// Only auto-enable for known interactive binaries.
		cmd := strings.ToLower(args[0])
		if strings.Contains(cmd, "python") || strings.Contains(cmd, "node") ||
			strings.Contains(cmd, "mysql") || strings.Contains(cmd, "psql") ||
			strings.Contains(cmd, "irb") || strings.Contains(cmd, "bash") {
			interactive = true
			fmt.Fprintln(os.Stderr, "[winrun] Auto-detected interactive mode")
		}
	}

	// Parse environment variables.
	envMap := make(map[string]string)
	for _, e := range envVars {
		parts := strings.SplitN(e, "=", 2)
		if len(parts) != 2 {
			fmt.Fprintf(os.Stderr, "Error: invalid env format %q, expected KEY=VAL\n", e)
			os.Exit(1)
		}
		envMap[parts[0]] = parts[1]
	}

	// Build the command config.
	command := args[0]
	cmdArgs := args[1:]

	config := bridge.CommandConfig{
		Command:      command,
		Args:         cmdArgs,
		Env:          envMap,
		EnvTunneling: tunnelEnv,
		Timeout:      timeout,
		ConvertPaths: convertPaths,
		Encoding:     encoding,
		Interactive:  interactive,
	}

	// Always make stdin available to the command.
	// In interactive mode this goes through direct copy;
	// in buffered mode it's piped via a goroutine.
	if !interactive && !bridge.IsTerminal(int(os.Stdin.Fd())) {
		// Stdin is a pipe (e.g., echo "data" | winrun ...) — forward it.
		config.Stdin = os.Stdin
	} else if interactive {
		config.Stdin = os.Stdin
	}

	// Set up signal handling for graceful shutdown.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 2)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigCh
		fmt.Fprintf(os.Stderr, "\n[winrun] Received %s, requesting graceful shutdown...\n", sig)
		cancel() // Cancel context → sends SIGTERM to child via exec.CommandContext.

		// Wait for a second signal or timeout for force kill.
		select {
		case sig2 := <-sigCh:
			fmt.Fprintf(os.Stderr, "[winrun] Received %s again, force exiting.\n", sig2)
			os.Exit(130)
		case <-time.After(5 * time.Second):
			fmt.Fprintln(os.Stderr, "[winrun] Grace period expired, force exiting.")
			os.Exit(130)
		}
	}()

	// Create an executor that uses our signal-aware context.
	executor := func(_ context.Context, cfg bridge.CommandConfig) (bridge.Output, error) {
		return bridge.Execute(ctx, cfg)
	}

	// Execute using the worker pool (even for a single command, for consistency).
	pool := workerpool.NewPool(concurrency, executor)
	pool.Submit(config)
	pool.Shutdown()

	exitCode := 0
	for result := range pool.Results() {
		if result.Err != nil {
			fmt.Fprintf(os.Stderr, "[winrun] Error: %v\n", result.Err)
			exitCode = 1
			continue
		}

		if result.Output.Stdout != "" {
			fmt.Println(result.Output.Stdout)
		}
		if result.Output.Stderr != "" {
			fmt.Fprintln(os.Stderr, result.Output.Stderr)
		}

		fmt.Fprintf(os.Stderr, "[winrun] Command %q completed in %s (exit code: %d)\n",
			result.Config.Command, result.Output.Duration.Round(time.Millisecond), result.Output.ExitCode)

		if result.Output.ExitCode != 0 {
			exitCode = result.Output.ExitCode
		}
	}

	os.Exit(exitCode)
}
