package wsl

import (
	"os"
	"testing"
)

// mockMounts is realistic /proc/mounts content for testing.
const mockMounts = `none / 9p rw,relatime,dirsync,aname=drvfs;path=\,symlinkroot=/mnt/wsl 0 0
none /init 9p rw,relatime 0 0
none /dev tmpfs rw,nosuid,relatime,mode=755 0 0
C:\ /mnt/c 9p rw,noatime,dirsync,aname=drvfs;path=C:\;uid=1000;gid=1000;symlinkroot=/mnt/wsl 0 0
D:\ /mnt/d 9p rw,noatime,dirsync,aname=drvfs;path=D:\;uid=1000;gid=1000;symlinkroot=/mnt/wsl 0 0
none /run tmpfs rw,nosuid,noexec,relatime 0 0
tmpfs /sys/fs/cgroup tmpfs rw,nosuid,nodev,noexec,relatime,mode=755 0 0`

func setupMockMounts(t *testing.T) {
	t.Helper()
	resetMountTable()
	ClearPathCache()
	mountTableReader = func() (string, error) {
		return mockMounts, nil
	}
	os.Setenv("WSL_DISTRO_NAME", "Ubuntu")
	t.Cleanup(func() {
		mountTableReader = defaultMountTableReader
		resetMountTable()
		ClearPathCache()
		os.Unsetenv("WSL_DISTRO_NAME")
	})
}

func TestParseMountTable(t *testing.T) {
	entries := parseMountTable(mockMounts)
	if len(entries) != 2 {
		t.Fatalf("expected 2 mount entries, got %d", len(entries))
	}
	if entries[0].DriveLetter != "C" || entries[0].MountPoint != "/mnt/c" {
		t.Errorf("entry[0] = %+v, want C:/mnt/c", entries[0])
	}
	if entries[1].DriveLetter != "D" || entries[1].MountPoint != "/mnt/d" {
		t.Errorf("entry[1] = %+v, want D:/mnt/d", entries[1])
	}
}

func TestToWindowsPath(t *testing.T) {
	setupMockMounts(t)

	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{
			name:  "drive root",
			input: "/mnt/c",
			want:  `C:\`,
		},
		{
			name:  "drive subdir",
			input: "/mnt/c/Users/test/file.txt",
			want:  `C:\Users\test\file.txt`,
		},
		{
			name:  "D drive",
			input: "/mnt/d/projects",
			want:  `D:\projects`,
		},
		{
			name:  "non-mount path (home)",
			input: "/home/user/code",
			want:  `\\wsl.localhost\Ubuntu\home\user\code`,
		},
		{
			name:  "root path",
			input: "/",
			want:  `\\wsl.localhost\Ubuntu\`,
		},
		{
			name:  "relative dot-dot resolved",
			input: "/mnt/c/Users/../Users/test",
			want:  `C:\Users\test`,
		},
		{
			name:    "empty path",
			input:   "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ToWindowsPath(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ToWindowsPath(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("ToWindowsPath(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestToLinuxPath(t *testing.T) {
	setupMockMounts(t)

	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{
			name:  "C drive path",
			input: `C:\Users\test`,
			want:  "/mnt/c/Users/test",
		},
		{
			name:  "C drive root",
			input: `C:\`,
			want:  "/mnt/c",
		},
		{
			name:  "D drive",
			input: `D:\projects\foo`,
			want:  "/mnt/d/projects/foo",
		},
		{
			name:  "UNC wsl.localhost",
			input: `\\wsl.localhost\Ubuntu\home\user`,
			want:  "/home/user",
		},
		{
			name:  "UNC wsl$",
			input: `\\wsl$\Ubuntu\tmp\file.txt`,
			want:  "/tmp/file.txt",
		},
		{
			name:    "empty path",
			input:   "",
			wantErr: true,
		},
		{
			name:    "unrecognized format",
			input:   "not-a-path",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ToLinuxPath(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ToLinuxPath(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("ToLinuxPath(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestPathCaching(t *testing.T) {
	setupMockMounts(t)

	// First call computes.
	r1, _ := ToWindowsPath("/mnt/c/test")
	// Second call should return cached result.
	r2, _ := ToWindowsPath("/mnt/c/test")

	if r1 != r2 {
		t.Errorf("cached result mismatch: %q vs %q", r1, r2)
	}
}

func TestMountTableReadError(t *testing.T) {
	resetMountTable()
	ClearPathCache()
	mountTableReader = func() (string, error) {
		return "", os.ErrNotExist
	}
	os.Setenv("WSL_DISTRO_NAME", "TestDistro")
	t.Cleanup(func() {
		mountTableReader = defaultMountTableReader
		resetMountTable()
		ClearPathCache()
		os.Unsetenv("WSL_DISTRO_NAME")
	})

	// With no mount table, all paths become UNC.
	got, err := ToWindowsPath("/mnt/c/test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Falls through to UNC since no mounts parsed.
	want := `\\wsl.localhost\TestDistro\mnt\c\test`
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
