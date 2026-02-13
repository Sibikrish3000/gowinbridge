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

// BuildWSLENV generates a WSLENV string from a map of environment variables.
// Keys that look like paths get the /p flag; all others get /u.
//
// Example output: "GOPATH/p:MY_VAR/u:PATH/p"
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
		flag := WSLEnvFlagUnixToWin
		if pathLikeKeys[strings.ToUpper(k)] {
			flag = WSLEnvFlagTranslatePath
		}
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
