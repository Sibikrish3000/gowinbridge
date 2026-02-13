package bridge

import (
	"fmt"
	"os"
	"sort"
	"strings"
)

// WSLENV flag constants.
// See: https://devblogs.microsoft.com/commandline/share-environment-vars-between-wsl-and-windows/
const (
	// WSLEnvFlagTranslatePath translates the value between WSL/Windows path formats.
	WSLEnvFlagTranslatePath = "/p"
	// WSLEnvFlagTranslatePathList translates a colon-delimited list of paths.
	WSLEnvFlagTranslatePathList = "/l"
	// WSLEnvFlagUnixToWin makes the value available only when invoking Win32 from WSL.
	WSLEnvFlagUnixToWin = "/u"
	// WSLEnvFlagWinToUnix makes the value available only when invoking WSL from Win32.
	WSLEnvFlagWinToUnix = "/w"
)

// pathLikeKeys are environment variable names that typically contain paths
// and should get the /p flag in WSLENV.
var pathLikeKeys = map[string]bool{
	"PATH":         true,
	"HOME":         true,
	"GOPATH":       true,
	"GOROOT":       true,
	"TMPDIR":       true,
	"TEMP":         true,
	"TMP":          true,
	"USERPROFILE":  true,
	"APPDATA":      true,
	"LOCALAPPDATA": true,
}

// inferWSLEnvFlag intelligently selects the WSLENV flag based on both the
// key name and the actual value content.
//
// Heuristics (in order of priority):
//  1. Value contains ":" with path-like segments → /l (path list)
//  2. Value starts with "/" or "./" or "../" → /p (single path)
//  3. Key name is in the well-known pathLikeKeys set → /p
//  4. Default → /u (pass unmodified)
func inferWSLEnvFlag(key, value string) string {
	// Check value for path list pattern: segments separated by ":" that look like paths.
	if strings.Contains(value, ":") {
		segments := strings.Split(value, ":")
		pathCount := 0
		for _, seg := range segments {
			seg = strings.TrimSpace(seg)
			if strings.HasPrefix(seg, "/") || strings.HasPrefix(seg, "./") {
				pathCount++
			}
		}
		// If most segments look like paths, treat as path list.
		if pathCount > 0 && pathCount >= len(segments)/2 {
			return WSLEnvFlagTranslatePathList
		}
	}

	// Check value for single path pattern.
	if strings.HasPrefix(value, "/") || strings.HasPrefix(value, "./") || strings.HasPrefix(value, "../") {
		return WSLEnvFlagTranslatePath
	}

	// Check well-known key names.
	if pathLikeKeys[strings.ToUpper(key)] {
		return WSLEnvFlagTranslatePath
	}

	return WSLEnvFlagUnixToWin
}

// BuildWSLENV generates a WSLENV string from a map of environment variables.
// Uses intelligent heuristics to auto-select /p, /l, or /u flags based on
// both key names and actual values.
//
// Example output: "GOPATH/p:MY_LIST/l:MY_VAR/u"
func BuildWSLENV(vars map[string]string) string {
	if len(vars) == 0 {
		return ""
	}

	// Sort keys for deterministic output.
	keys := make([]string, 0, len(vars))
	for k := range vars {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		flag := inferWSLEnvFlag(k, vars[k])
		parts = append(parts, fmt.Sprintf("%s%s", k, flag))
	}

	return strings.Join(parts, ":")
}

// PrepareEnv builds the full environment slice for a command.
// It starts from the current process environment, adds user-specified vars,
// and optionally appends the WSLENV tunneling variable.
func PrepareEnv(config CommandConfig) []string {
	if len(config.Env) == 0 && !config.EnvTunneling {
		return nil // Inherit parent environment.
	}

	// Start with current environment.
	env := os.Environ()

	// Add user-specified variables.
	for k, v := range config.Env {
		env = append(env, fmt.Sprintf("%s=%s", k, v))
	}

	// If tunneling is enabled, add WSLENV.
	if config.EnvTunneling && len(config.Env) > 0 {
		wslenv := BuildWSLENV(config.Env)

		// Check if WSLENV already exists and append.
		existingIdx := -1
		for i, e := range env {
			if strings.HasPrefix(e, "WSLENV=") {
				existingIdx = i
				break
			}
		}

		if existingIdx >= 0 {
			existing := strings.TrimPrefix(env[existingIdx], "WSLENV=")
			if existing != "" {
				wslenv = existing + ":" + wslenv
			}
			env[existingIdx] = "WSLENV=" + wslenv
		} else {
			env = append(env, "WSLENV="+wslenv)
		}
	}

	return env
}
