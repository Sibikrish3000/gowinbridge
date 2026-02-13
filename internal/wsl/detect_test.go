package wsl

import (
	"fmt"
	"testing"
)

func TestIsWSL(t *testing.T) {
	tests := []struct {
		name       string
		procInput  string
		procErr    error
		wantIsWSL  bool
		wantVer    int
	}{
		{
			name:      "WSL2 kernel",
			procInput: "Linux version 5.15.90.1-microsoft-standard-WSL2 (root@1234) (gcc) #1 SMP",
			wantIsWSL: true,
			wantVer:   WSLVersion2,
		},
		{
			name:      "WSL2 lowercase variant",
			procInput: "Linux version 5.15.146.1-microsoft-standard-WSL2 (gcc version 12) #1 SMP",
			wantIsWSL: true,
			wantVer:   WSLVersion2,
		},
		{
			name:      "WSL1 kernel",
			procInput: "Linux version 4.4.0-19041-Microsoft (Microsoft@Microsoft.com) (gcc version 5.4.0) #1 SMP",
			wantIsWSL: true,
			wantVer:   WSLVersion1,
		},
		{
			name:      "native Linux",
			procInput: "Linux version 6.1.0-13-amd64 (debian-kernel@lists.debian.org) (gcc-12 (Debian 12.2.0-14)) #1 SMP PREEMPT_DYNAMIC",
			wantIsWSL: false,
			wantVer:   WSLVersionNone,
		},
		{
			name:      "empty proc version",
			procInput: "",
			wantIsWSL: false,
			wantVer:   WSLVersionNone,
		},
		{
			name:      "read error",
			procInput: "",
			procErr:   fmt.Errorf("permission denied"),
			wantIsWSL: false,
			wantVer:   WSLVersionNone,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset detection state and inject test reader.
			resetDetection()
			procVersionReader = func() (string, error) {
				return tt.procInput, tt.procErr
			}
			t.Cleanup(func() {
				procVersionReader = defaultProcVersionReader
				resetDetection()
			})

			gotWSL := IsWSL()
			if gotWSL != tt.wantIsWSL {
				t.Errorf("IsWSL() = %v, want %v", gotWSL, tt.wantIsWSL)
			}

			gotVer := DetectWSLVersion()
			if gotVer != tt.wantVer {
				t.Errorf("DetectWSLVersion() = %d, want %d", gotVer, tt.wantVer)
			}
		})
	}
}

func TestDetectionIsCached(t *testing.T) {
	resetDetection()
	callCount := 0
	procVersionReader = func() (string, error) {
		callCount++
		return "Linux version 5.15.90.1-microsoft-standard-WSL2", nil
	}
	t.Cleanup(func() {
		procVersionReader = defaultProcVersionReader
		resetDetection()
	})

	_ = IsWSL()
	_ = IsWSL()
	_ = DetectWSLVersion()

	if callCount != 1 {
		t.Errorf("procVersionReader called %d times, want 1 (should be cached)", callCount)
	}
}
