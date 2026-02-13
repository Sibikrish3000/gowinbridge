package wsl

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"unicode"
)

// mountEntry represents a single DrvFs mount mapping.
type mountEntry struct {
	// DriveLetter is the Windows drive letter (e.g., "C").
	DriveLetter string
	// MountPoint is the Linux mount path (e.g., "/mnt/c").
	MountPoint string
}

var (
	mountTable     []mountEntry
	mountTableOnce sync.Once

	// pathCache memoizes path translations to avoid repeated computation.
	pathCache sync.Map

	// wslDistroName is cached from the WSL_DISTRO_NAME env var.
	wslDistroName     string
	wslDistroNameOnce sync.Once
)

// mountTableReader reads mount information. Replaceable for testing.
var mountTableReader = defaultMountTableReader

func defaultMountTableReader() (string, error) {
	data, err := os.ReadFile("/proc/mounts")
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// parseMountTable extracts DrvFs mounts from /proc/mounts content.
// Lines look like: "C:\ /mnt/c 9p ..." or "drvfs /mnt/c 9p ..."
func parseMountTable(content string) []mountEntry {
	var entries []mountEntry
	scanner := bufio.NewScanner(strings.NewReader(content))
	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}

		mountPoint := fields[1]
		fsType := fields[2]

		// DrvFs / 9p mounts for Windows drives are typically under /mnt/<letter>.
		if (fsType == "9p" || fsType == "drvfs") && strings.HasPrefix(mountPoint, "/mnt/") {
			suffix := strings.TrimPrefix(mountPoint, "/mnt/")
			// Must be a single letter (the drive letter).
			if len(suffix) == 1 && unicode.IsLetter(rune(suffix[0])) {
				entries = append(entries, mountEntry{
					DriveLetter: strings.ToUpper(suffix),
					MountPoint:  mountPoint,
				})
			}
		}
	}
	return entries
}

// getMountTable returns the cached mount table, parsing /proc/mounts on first call.
func getMountTable() []mountEntry {
	mountTableOnce.Do(func() {
		content, err := mountTableReader()
		if err != nil {
			mountTable = nil
			return
		}
		mountTable = parseMountTable(content)
	})
	return mountTable
}

// getDistroName returns the cached WSL distro name.
func getDistroName() string {
	wslDistroNameOnce.Do(func() {
		wslDistroName = os.Getenv("WSL_DISTRO_NAME")
		if wslDistroName == "" {
			wslDistroName = "Ubuntu" // Reasonable default.
		}
	})
	return wslDistroName
}

// cacheKey creates a unique key for the path cache.
func cacheKey(direction, path string) string {
	return direction + ":" + path
}

// ToWindowsPath translates a Linux path to a Windows path using pure Go.
//
// Algorithm:
//  1. Check if path is under a known /mnt/<letter> mount → "X:\rest\of\path"
//  2. Otherwise, generate UNC path → "\\wsl.localhost\<distro>\path"
//
// Results are memoized.
func ToWindowsPath(linuxPath string) (string, error) {
	if linuxPath == "" {
		return "", fmt.Errorf("empty path provided")
	}

	key := cacheKey("w", linuxPath)
	if cached, ok := pathCache.Load(key); ok {
		return cached.(string), nil
	}

	// Clean the path to resolve . and .. components.
	cleaned := filepath.Clean(linuxPath)

	result := toWindowsPathInternal(cleaned)

	pathCache.Store(key, result)
	return result, nil
}

// toWindowsPathInternal performs the actual conversion without caching.
func toWindowsPathInternal(linuxPath string) string {
	mounts := getMountTable()

	// Check each mount point, longest match first isn't needed since
	// /mnt/<letter> are all the same depth.
	for _, m := range mounts {
		if linuxPath == m.MountPoint {
			// Exact match: /mnt/c → C:\
			return m.DriveLetter + ":\\"
		}
		prefix := m.MountPoint + "/"
		if strings.HasPrefix(linuxPath, prefix) {
			rest := strings.TrimPrefix(linuxPath, prefix)
			winRest := strings.ReplaceAll(rest, "/", "\\")
			return m.DriveLetter + ":\\" + winRest
		}
	}

	// Not a Windows drive mount — generate UNC path.
	distro := getDistroName()
	winPath := strings.ReplaceAll(linuxPath, "/", "\\")
	return `\\wsl.localhost\` + distro + winPath
}

// ToLinuxPath translates a Windows path to a Linux path using pure Go.
//
// Algorithm:
//  1. "X:\..." → "/mnt/x/..."
//  2. "\\wsl.localhost\<distro>\..." → "/..."
//
// Results are memoized.
func ToLinuxPath(windowsPath string) (string, error) {
	if windowsPath == "" {
		return "", fmt.Errorf("empty path provided")
	}

	key := cacheKey("u", windowsPath)
	if cached, ok := pathCache.Load(key); ok {
		return cached.(string), nil
	}

	result, err := toLinuxPathInternal(windowsPath)
	if err != nil {
		return "", err
	}

	pathCache.Store(key, result)
	return result, nil
}

// toLinuxPathInternal performs the actual conversion without caching.
func toLinuxPathInternal(windowsPath string) (string, error) {
	// Handle UNC paths: \\wsl.localhost\distro\path or \\wsl$\distro\path
	if strings.HasPrefix(windowsPath, `\\wsl.localhost\`) || strings.HasPrefix(windowsPath, `\\wsl$\`) {
		var rest string
		if strings.HasPrefix(windowsPath, `\\wsl.localhost\`) {
			rest = strings.TrimPrefix(windowsPath, `\\wsl.localhost\`)
		} else {
			rest = strings.TrimPrefix(windowsPath, `\\wsl$\`)
		}
		// Skip distro name.
		idx := strings.Index(rest, `\`)
		if idx >= 0 {
			linuxPath := rest[idx:]
			linuxPath = strings.ReplaceAll(linuxPath, `\`, "/")
			return filepath.Clean(linuxPath), nil
		}
		return "/", nil
	}

	// Handle drive letter paths: C:\Users\... → /mnt/c/Users/...
	if len(windowsPath) >= 2 && windowsPath[1] == ':' && unicode.IsLetter(rune(windowsPath[0])) {
		driveLetter := strings.ToLower(string(windowsPath[0]))
		rest := ""
		if len(windowsPath) > 2 {
			rest = windowsPath[2:]
			if strings.HasPrefix(rest, `\`) {
				rest = rest[1:]
			}
			rest = strings.ReplaceAll(rest, `\`, "/")
		}
		if rest == "" {
			return "/mnt/" + driveLetter, nil
		}
		return filepath.Clean("/mnt/" + driveLetter + "/" + rest), nil
	}

	return "", fmt.Errorf("unrecognized Windows path format: %q", windowsPath)
}

// ClearPathCache clears the memoized path cache.
func ClearPathCache() {
	pathCache = sync.Map{}
}

// resetMountTable resets mount table state for testing.
func resetMountTable() {
	mountTableOnce = sync.Once{}
	mountTable = nil
	wslDistroNameOnce = sync.Once{}
	wslDistroName = ""
}
