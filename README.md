# GoWinBridge

[![Go](https://img.shields.io/badge/Go-1.21+-00ADD8?logo=go&logoColor=white)](https://go.dev)
[![License: GPL](https://img.shields.io/badge/License-GPL-yellow.svg)](LICENSE)

A Go library and CLI for executing Windows binaries from WSL with **path translation**, **environment bridging**, **real-time stdio streaming**, and **bounded concurrency**.

## Why?

Running Windows binaries from WSL via `os/exec` works, but lacks:

- **Path translation** — Linux paths like `./file.txt` don't resolve on the Windows side
- **Environment bridging** — Windows processes don't inherit Linux env vars without `WSLENV`
- **Signal propagation** — Ctrl+C in your terminal won't kill the Windows child process
- **Concurrency control** — Spawning dozens of Windows processes simultaneously is expensive

GoWinBridge solves all of these behind a clean Go API and a ready-to-use CLI.

## Architecture

```
cmd/winrun/          CLI entry point (flags, signal handling)
  │
  ├── pkg/workerpool/   Bounded worker pool (configurable concurrency)
  │     │
  │     └── pkg/bridge/     Core executor
  │           ├── exec.go       CommandContext, .exe resolution, stdio streaming
  │           ├── env.go        WSLENV formatting & tunneling
  │           └── config.go     CommandConfig / Output types
  │
  └── internal/wsl/     WSL detection & path translation
        ├── detect.go       WSL1 vs WSL2 detection (cached singleton)
        └── path.go         wslpath wrapper with memoization
```

## Installation

```bash
# Clone
git clone https://github.com/sibikrish3000/gowinbridge.git
cd gowinbridge

# Build the CLI
go build -o winrun ./cmd/winrun

# Or install directly
go install github.com/sibikrish3000/gowinbridge/cmd/winrun@latest
```

## CLI Usage

```bash
# Basic execution
winrun -- cmd.exe /c echo hello

# Auto-convert Linux paths to Windows paths
winrun --convert-paths -- cmd.exe /c type ./go.mod

# Environment variable tunneling
winrun --env MY_VAR=hello --tunnel-env -- cmd.exe /c echo %MY_VAR%

# Concurrent execution with timeout
winrun --concurrency 4 --timeout 30s -- powershell.exe -Command Get-Process
```

### Flags

| Flag | Default | Description |
|---|---|---|
| `--concurrency N` | `NumCPU` | Max concurrent Windows process executions |
| `--convert-paths` | `false` | Auto-detect and convert file path arguments to Windows format |
| `--env KEY=VAL` | — | Set environment variable (repeatable) |
| `--tunnel-env` | `false` | Enable WSLENV tunneling for `--env` vars |
| `--timeout DURATION` | `0` (none) | Max execution time (e.g., `30s`, `5m`) |

## Library Usage

### Basic Execution

```go
package main

import (
    "context"
    "fmt"
    "log"

    "github.com/sibikrish/gowinbridge/pkg/bridge"
)

func main() {
    output, err := bridge.Execute(context.Background(), bridge.CommandConfig{
        Command:      "cmd.exe",
        Args:         []string{"/c", "echo", "hello from Windows"},
        ConvertPaths: false,
    })
    if err != nil {
        log.Fatal(err)
    }
    fmt.Println(output.Stdout)
    // Output: hello from Windows
}
```

### With Path Translation & Environment Tunneling

```go
output, err := bridge.Execute(ctx, bridge.CommandConfig{
    Command:      "cmd.exe",
    Args:         []string{"/c", "type", "./myfile.txt"},
    ConvertPaths: true,  // ./myfile.txt → C:\Users\...\myfile.txt
    Env:          map[string]string{"MY_VAR": "value"},
    EnvTunneling: true,  // Generates WSLENV=MY_VAR/u
    Timeout:      30 * time.Second,
})
```

### Concurrent Execution via Worker Pool

```go
import "github.com/sibikrish3000/gowinbridge/pkg/workerpool"

pool := workerpool.NewPool(4, bridge.Execute)

pool.Submit(bridge.CommandConfig{Command: "build1.exe"})
pool.Submit(bridge.CommandConfig{Command: "build2.exe"})
pool.Submit(bridge.CommandConfig{Command: "build3.exe"})

go pool.Shutdown()

for result := range pool.Results() {
    if result.Err != nil {
        log.Printf("Error: %v", result.Err)
        continue
    }
    fmt.Printf("[%s] %s\n", result.Config.Command, result.Output.Stdout)
}
```

### WSL Detection

```go
import "github.com/sibikrish3000/gowinbridge/internal/wsl"

if !wsl.IsWSL() {
    log.Fatal("This tool requires WSL")
}
fmt.Printf("Running on WSL%d\n", wsl.DetectWSLVersion())
```

## Development

### Prerequisites

- Go 1.21+
- WSL (for integration tests; unit tests run anywhere)

### Project Structure

```
.
├── cmd/winrun/              CLI tool
├── internal/wsl/            WSL detection & path translation (private)
│   ├── detect.go
│   ├── detect_test.go
│   ├── path.go
│   └── path_test.go
├── pkg/bridge/              Core executor (public API)
│   ├── config.go
│   ├── env.go
│   ├── exec.go
│   └── exec_test.go
├── pkg/workerpool/          Bounded concurrency pool (public API)
│   ├── pool.go
│   └── pool_test.go
├── go.mod
└── README.md
```

### Running Tests

```bash
# All unit tests (works on any OS — WSL integration tests auto-skip)
go test ./... -v

# With race detector
go test ./... -race

# Short / unit tests only
go test ./... -short
```

### Key Design Decisions

| Decision | Rationale |
|---|---|
| **`sync.Once` for WSL detection** | Avoids repeated `/proc/version` reads; cached after first call |
| **`sync.Map` for path cache** | Memoizes `wslpath` shellouts; concurrent-safe without locks |
| **Injectable `commandRunner`** | Allows mocking `wslpath` in tests without needing the real binary |
| **`exec.CommandContext`** | Ensures context cancellation (timeout / SIGINT) kills the Windows process |
| **`bufio.Scanner` goroutines** | Streams stdout/stderr in real-time instead of buffering everything in memory |
| **`.exe` auto-resolution** | Appends `.exe` and checks `PATH` if the user passes `cmd` instead of `cmd.exe` |
| **Worker pool with injectable executor** | Testable concurrency engine; mock executor eliminates WSL dependency in tests |

### Known Gotchas

- **Binary names**: Always use `.exe` suffix (e.g., `cmd.exe`, not `cmd`). The library attempts auto-resolution but explicit is better.
- **Path separators**: Windows uses `\`. The library handles this via `wslpath`, but be careful with manual string building.
- **Zombie processes**: The CLI registers `SIGINT`/`SIGTERM` handlers to cancel all in-flight Windows processes on exit.
- **WSLENV**: Only works for environment variables you explicitly pass — it does not auto-export your entire shell environment.

## License

GNU GENERAL PUBLIC LICENSE

