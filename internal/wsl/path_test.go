package wsl

import (
	"fmt"
	"testing"
)

func TestToWindowsPath(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		mockOut   string
		mockErr   error
		want      string
		wantErr   bool
	}{
		{
			name:    "home directory",
			input:   "/home/user",
			mockOut: `\\wsl.localhost\Ubuntu\home\user`,
			want:    `\\wsl.localhost\Ubuntu\home\user`,
		},
		{
			name:    "mnt path",
			input:   "/mnt/c/Users/test",
			mockOut: `C:\Users\test`,
			want:    `C:\Users\test`,
		},
		{
			name:    "root path",
			input:   "/",
			mockOut: `\\wsl.localhost\Ubuntu`,
			want:    `\\wsl.localhost\Ubuntu`,
		},
		{
			name:    "empty path",
			input:   "",
			wantErr: true,
		},
		{
			name:    "wslpath error",
			input:   "/nonexistent",
			mockErr: fmt.Errorf("wslpath: not found"),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ClearPathCache()
			commandRunner = func(name string, args ...string) (string, error) {
				if tt.mockErr != nil {
					return "", tt.mockErr
				}
				return tt.mockOut, nil
			}
			t.Cleanup(func() {
				commandRunner = defaultCommandRunner
				ClearPathCache()
			})

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
	tests := []struct {
		name    string
		input   string
		mockOut string
		mockErr error
		want    string
		wantErr bool
	}{
		{
			name:    "C drive",
			input:   `C:\Users\test`,
			mockOut: "/mnt/c/Users/test",
			want:    "/mnt/c/Users/test",
		},
		{
			name:    "empty path",
			input:   "",
			wantErr: true,
		},
		{
			name:    "wslpath error",
			input:   `Z:\invalid`,
			mockErr: fmt.Errorf("wslpath: error"),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ClearPathCache()
			commandRunner = func(name string, args ...string) (string, error) {
				if tt.mockErr != nil {
					return "", tt.mockErr
				}
				return tt.mockOut, nil
			}
			t.Cleanup(func() {
				commandRunner = defaultCommandRunner
				ClearPathCache()
			})

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
	ClearPathCache()
	callCount := 0
	commandRunner = func(name string, args ...string) (string, error) {
		callCount++
		return `C:\cached`, nil
	}
	t.Cleanup(func() {
		commandRunner = defaultCommandRunner
		ClearPathCache()
	})

	// First call should invoke the command runner.
	_, _ = ToWindowsPath("/some/path")
	// Second call should hit the cache.
	_, _ = ToWindowsPath("/some/path")

	if callCount != 1 {
		t.Errorf("commandRunner called %d times, want 1 (cache miss + cache hit)", callCount)
	}

	// A different path should cause another invocation.
	_, _ = ToWindowsPath("/another/path")
	if callCount != 2 {
		t.Errorf("commandRunner called %d times, want 2 after second unique path", callCount)
	}
}
