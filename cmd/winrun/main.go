// winrun is a CLI tool that uses the gowinbridge library to execute
// Windows binaries from WSL with path translation, environment bridging,
// concurrent execution, and graceful signal handling.
//
// Usage:
//
//	winrun [flags] -- <command> [args...]
//
// Flags:
//
//	--concurrency N    Max concurrent executions (default: NumCPU)
//	--convert-paths    Auto-detect and convert file path arguments
//	--env KEY=VAL      Set environment variable (repeatable)
//	--tunnel-env       Enable WSLENV tunneling for --env vars
//	--timeout DURATION Max execution time (e.g., 30s, 5m)
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
	var (
		concurrency  int
		convertPaths bool
		envVars      envFlags
		tunnelEnv    bool
		timeout      time.Duration
		showVersion  bool
	)

	flag.IntVar(&concurrency, "concurrency", runtime.NumCPU(), "Max concurrent executions")
	flag.BoolVar(&convertPaths, "convert-paths", false, "Auto-convert file path arguments to Windows format")
	flag.Var(&envVars, "env", "Set environment variable as KEY=VAL (repeatable)")
	flag.BoolVar(&tunnelEnv, "tunnel-env", false, "Enable WSLENV tunneling for specified env vars")
	flag.DurationVar(&timeout, "timeout", 0, "Max execution time (e.g., 30s, 5m)")
	flag.BoolVar(&showVersion, "version", false, "Print version information and exit")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: winrun [flags] -- <command> [args...]\n\n")
		fmt.Fprintf(os.Stderr, "Execute Windows binaries from WSL with path translation and env bridging.\n\n")
		fmt.Fprintf(os.Stderr, "Flags:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nExamples:\n")
		fmt.Fprintf(os.Stderr, "  winrun -- cmd.exe /c echo hello\n")
		fmt.Fprintf(os.Stderr, "  winrun --convert-paths -- cmd.exe /c type ./myfile.txt\n")
		fmt.Fprintf(os.Stderr, "  winrun --env MY_VAR=hello --tunnel-env -- cmd.exe /c echo %%MY_VAR%%\n")
		fmt.Fprintf(os.Stderr, "  winrun --concurrency 4 --timeout 30s -- powershell.exe -Command Get-Process\n")
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
	}

	// Set up signal handling for graceful shutdown.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigCh
		fmt.Fprintf(os.Stderr, "\n[winrun] Received signal %s, shutting down...\n", sig)
		cancel()
		// Give processes a moment to terminate, then force exit.
		time.Sleep(2 * time.Second)
		fmt.Fprintln(os.Stderr, "[winrun] Force exit.")
		os.Exit(130)
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
