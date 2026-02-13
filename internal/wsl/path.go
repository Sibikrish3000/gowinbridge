package wsl

import (
	"fmt"
	"os/exec"
	"strings"
	"sync"
)

// pathCache memoizes wslpath lookups to avoid redundant shellouts.
var pathCache sync.Map

// commandRunner abstracts exec.Command for testability.
// In production this calls the real wslpath binary; in tests it can be replaced.
var commandRunner = defaultCommandRunner

func defaultCommandRunner(name string, args ...string) (string, error) {
	out, err := exec.Command(name, args...).Output()
	if err != nil {
		return "", fmt.Errorf("wslpath command failed: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

// cacheKey creates a unique key for the path cache.
func cacheKey(direction, path string) string {
	return direction + ":" + path
}

// ToWindowsPath translates a Linux path to a Windows path using wslpath -w.
// Results are memoized to reduce shellout overhead.
func ToWindowsPath(linuxPath string) (string, error) {
	if linuxPath == "" {
		return "", fmt.Errorf("empty path provided")
	}

	key := cacheKey("w", linuxPath)
	if cached, ok := pathCache.Load(key); ok {
		return cached.(string), nil
	}

	result, err := commandRunner("wslpath", "-w", linuxPath)
	if err != nil {
		return "", fmt.Errorf("failed to convert path %q to Windows format: %w", linuxPath, err)
	}

	pathCache.Store(key, result)
	return result, nil
}

// ToLinuxPath translates a Windows path to a Linux path using wslpath -u.
// Results are memoized to reduce shellout overhead.
func ToLinuxPath(windowsPath string) (string, error) {
	if windowsPath == "" {
		return "", fmt.Errorf("empty path provided")
	}

	key := cacheKey("u", windowsPath)
	if cached, ok := pathCache.Load(key); ok {
		return cached.(string), nil
	}

	result, err := commandRunner("wslpath", "-u", windowsPath)
	if err != nil {
		return "", fmt.Errorf("failed to convert path %q to Linux format: %w", windowsPath, err)
	}

	pathCache.Store(key, result)
	return result, nil
}

// ClearPathCache clears the memoized path cache.
// Primarily useful for testing.
func ClearPathCache() {
	pathCache = sync.Map{}
}
