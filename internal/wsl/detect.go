// Package wsl provides utilities for detecting and operating within a
// Windows Subsystem for Linux (WSL) environment.
package wsl

import (
	"os"
	"strings"
	"sync"
)

// WSL version constants.
const (
	WSLVersionNone = 0
	WSLVersion1    = 1
	WSLVersion2    = 2
)

var (
	wslDetectOnce sync.Once
	wslDetected   bool
	wslVersion    int
)

// procVersionReader is the function used to read /proc/version content.
// It can be overridden in tests for injection.
var procVersionReader = defaultProcVersionReader

func defaultProcVersionReader() (string, error) {
	data, err := os.ReadFile("/proc/version")
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// detect performs the actual WSL detection. It is called once via sync.Once.
func detect() {
	content, err := procVersionReader()
	if err != nil {
		wslDetected = false
		wslVersion = WSLVersionNone
		return
	}

	lower := strings.ToLower(content)

	// Check for WSL indicators in /proc/version.
	if !strings.Contains(lower, "microsoft") {
		wslDetected = false
		wslVersion = WSLVersionNone
		return
	}

	wslDetected = true

	// WSL2 kernels contain "microsoft-standard-wsl2" or similar patterns.
	if strings.Contains(lower, "microsoft-standard-wsl2") ||
		strings.Contains(lower, "microsoft-standard-wsl") {
		wslVersion = WSLVersion2
	} else {
		// Older "Microsoft" references in /proc/version indicate WSL1.
		wslVersion = WSLVersion1
	}
}

// IsWSL returns true if the current environment is a WSL instance.
// The result is cached after the first call (singleton pattern).
func IsWSL() bool {
	wslDetectOnce.Do(detect)
	return wslDetected
}

// DetectWSLVersion returns the WSL version (1 or 2) or WSLVersionNone (0)
// if the environment is not WSL.
func DetectWSLVersion() int {
	wslDetectOnce.Do(detect)
	return wslVersion
}

// resetDetection resets the detection state for testing purposes.
// This is NOT exported and should only be used in tests within this package.
func resetDetection() {
	wslDetectOnce = sync.Once{}
	wslDetected = false
	wslVersion = WSLVersionNone
}
