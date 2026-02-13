package bridge

import (
	"testing"
)

func TestBuildWSLENV(t *testing.T) {
	tests := []struct {
		name string
		vars map[string]string
		want string
	}{
		{
			name: "empty map",
			vars: map[string]string{},
			want: "",
		},
		{
			name: "nil map",
			vars: nil,
			want: "",
		},
		{
			name: "single custom var",
			vars: map[string]string{"MY_VAR": "hello"},
			want: "MY_VAR/u",
		},
		{
			name: "path-like var",
			vars: map[string]string{"GOPATH": "/home/user/go"},
			want: "GOPATH/p",
		},
		{
			name: "mixed vars sorted",
			vars: map[string]string{
				"GOPATH": "/home/user/go",
				"MY_VAR": "hello",
				"PATH":   "/usr/bin",
			},
			want: "GOPATH/p:MY_VAR/u:PATH/p",
		},
		{
			name: "multiple custom vars",
			vars: map[string]string{
				"FOO": "1",
				"BAR": "2",
			},
			want: "BAR/u:FOO/u",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BuildWSLENV(tt.vars)
			if got != tt.want {
				t.Errorf("BuildWSLENV() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestPrepareEnv_NilWhenEmpty(t *testing.T) {
	config := CommandConfig{}
	env := PrepareEnv(config)
	if env != nil {
		t.Errorf("PrepareEnv(empty config) should return nil, got %d vars", len(env))
	}
}

func TestPrepareEnv_IncludesUserVars(t *testing.T) {
	config := CommandConfig{
		Env: map[string]string{"TEST_KEY": "test_value"},
	}
	env := PrepareEnv(config)
	if env == nil {
		t.Fatal("PrepareEnv should not return nil when Env is set")
	}

	found := false
	for _, e := range env {
		if e == "TEST_KEY=test_value" {
			found = true
			break
		}
	}
	if !found {
		t.Error("PrepareEnv did not include TEST_KEY=test_value")
	}
}

func TestPrepareEnv_WithTunneling(t *testing.T) {
	config := CommandConfig{
		Env:          map[string]string{"MY_VAR": "hello"},
		EnvTunneling: true,
	}
	env := PrepareEnv(config)
	if env == nil {
		t.Fatal("PrepareEnv should not return nil when EnvTunneling is true")
	}

	foundWSLENV := false
	for _, e := range env {
		if len(e) >= 7 && e[:7] == "WSLENV=" {
			foundWSLENV = true
			val := e[7:]
			if val == "" {
				t.Error("WSLENV should not be empty")
			}
			// Should contain MY_VAR/u
			if val != "MY_VAR/u" && !contains(val, "MY_VAR/u") {
				t.Errorf("WSLENV = %q, should contain MY_VAR/u", val)
			}
			break
		}
	}
	if !foundWSLENV {
		t.Error("WSLENV variable not found in environment")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && searchSubstr(s, substr))
}

func searchSubstr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestResolveCommand(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantExe bool // Whether the result should end in .exe
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
			wantExe: false, // Falls back to original
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
		{"C:\\Windows", false}, // Already a Windows path
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := looksLikePath(tt.input); got != tt.want {
				t.Errorf("looksLikePath(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}
