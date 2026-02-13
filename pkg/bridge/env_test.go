package bridge

import (
	"strings"
	"testing"
)

func TestInferWSLEnvFlag(t *testing.T) {
	tests := []struct {
		name  string
		key   string
		value string
		want  string
	}{
		{
			name:  "value is a path",
			key:   "MY_DIR",
			value: "/home/user/project",
			want:  WSLEnvFlagTranslatePath,
		},
		{
			name:  "value is a relative path",
			key:   "CONF",
			value: "./config/app.yaml",
			want:  WSLEnvFlagTranslatePath,
		},
		{
			name:  "value is a parent-relative path",
			key:   "DATA",
			value: "../data/file.csv",
			want:  WSLEnvFlagTranslatePath,
		},
		{
			name:  "value is a path list",
			key:   "SEARCH_DIRS",
			value: "/usr/bin:/usr/local/bin:/home/user/bin",
			want:  WSLEnvFlagTranslatePathList,
		},
		{
			name:  "value is a mixed path list",
			key:   "LIB_PATH",
			value: "/usr/lib:./local/lib",
			want:  WSLEnvFlagTranslatePathList,
		},
		{
			name:  "known path key with non-path value",
			key:   "GOPATH",
			value: "",
			want:  WSLEnvFlagTranslatePath, // Key-based lookup.
		},
		{
			name:  "plain string value",
			key:   "MY_VAR",
			value: "hello",
			want:  WSLEnvFlagUnixToWin,
		},
		{
			name:  "colon in non-path value",
			key:   "CONN_STR",
			value: "host:port",
			want:  WSLEnvFlagUnixToWin, // "host" and "port" don't look like paths.
		},
		{
			name:  "empty value unknown key",
			key:   "EMPTY",
			value: "",
			want:  WSLEnvFlagUnixToWin,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := inferWSLEnvFlag(tt.key, tt.value)
			if got != tt.want {
				t.Errorf("inferWSLEnvFlag(%q, %q) = %q, want %q", tt.key, tt.value, got, tt.want)
			}
		})
	}
}

func TestBuildWSLENV_WithHeuristics(t *testing.T) {
	vars := map[string]string{
		"MY_VAR":   "hello",
		"MY_PATH":  "/home/user/data",
		"LIB_DIRS": "/usr/lib:/usr/local/lib",
	}

	got := BuildWSLENV(vars)
	// Sorted: LIB_DIRS/l:MY_PATH/p:MY_VAR/u
	want := "LIB_DIRS/l:MY_PATH/p:MY_VAR/u"
	if got != want {
		t.Errorf("BuildWSLENV() = %q, want %q", got, want)
	}
}

func TestBuildWSLENV_Empty(t *testing.T) {
	if got := BuildWSLENV(nil); got != "" {
		t.Errorf("BuildWSLENV(nil) = %q, want empty", got)
	}
	if got := BuildWSLENV(map[string]string{}); got != "" {
		t.Errorf("BuildWSLENV({}) = %q, want empty", got)
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
			if !strings.Contains(val, "MY_VAR/u") {
				t.Errorf("WSLENV = %q, should contain MY_VAR/u", val)
			}
			break
		}
	}
	if !foundWSLENV {
		t.Error("WSLENV variable not found in environment")
	}
}

func strings_Contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
