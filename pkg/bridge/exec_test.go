package bridge

import (
	"testing"
)

func TestResolveCommand(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantExe bool
	}{
		{
			name:    "already has .exe",
			input:   "cmd.exe",
			wantExe: true,
		},
		{
			name:    "uppercase .EXE",
			input:   "CMD.EXE",
			wantExe: true,
		},
		{
			name:    "no extension — not on PATH",
			input:   "nonexistent_binary_xyz",
			wantExe: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveCommand(tt.input)
			hasExe := len(got) >= 4 && (got[len(got)-4:] == ".exe" || got[len(got)-4:] == ".EXE")
			if tt.wantExe && !hasExe {
				t.Errorf("resolveCommand(%q) = %q, expected .exe suffix", tt.input, got)
			}
		})
	}
}

func TestLooksLikePath(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"/home/user/file.txt", true},
		{"./relative/file", true},
		{"../parent/file", true},
		{"just-a-flag", false},
		{"-v", false},
		{"", false},
		{"C:\\Windows", false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := looksLikePath(tt.input); got != tt.want {
				t.Errorf("looksLikePath(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}
