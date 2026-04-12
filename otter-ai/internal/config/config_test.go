package config

import (
	"os"
	"testing"
	"time"
)

func clearEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{
		"OTTER_RAFT_ID", "OTTER_ENV", "OTTER_PORT", "OTTER_DB_PATH",
		"OTTER_VECTOR_BACKEND", "OTTER_RAFT_TYPE", "OTTER_RAFT_BIND_ADDR",
		"OTTER_RAFT_ADVERTISE_ADDR", "OTTER_RAFT_DATA_DIR", "OTTER_LLM_PROVIDER",
		"OTTER_LLM_ENDPOINT", "OTTER_LLM_MODEL", "OTTER_LLM_API_KEY",
		"OTTER_HOST", "OTTER_HOST_PASSPHRASE", "OTTER_JWT_SECRET",
		"OTTER_RATE_LIMIT", "OTTER_RATE_LIMIT_WINDOW",
	} {
		os.Unsetenv(k)
	}
}

func TestLoad_Success(t *testing.T) {
	clearEnv(t)
	os.Setenv("OTTER_RAFT_ID", "test-raft")
	t.Cleanup(func() { clearEnv(t) })

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Raft.ID != "test-raft" {
		t.Errorf("Raft.ID = %q; want test-raft", cfg.Raft.ID)
	}
}

func TestLoad_MissingRaftID(t *testing.T) {
	clearEnv(t)
	_, err := Load()
	if err == nil {
		t.Error("expected error for missing OTTER_RAFT_ID")
	}
}

func TestLoad_Defaults(t *testing.T) {
	clearEnv(t)
	os.Setenv("OTTER_RAFT_ID", "raft1")
	t.Cleanup(func() { clearEnv(t) })

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Env != "development" {
		t.Errorf("Env = %q; want development", cfg.Env)
	}
	if cfg.Port != 8080 {
		t.Errorf("Port = %d; want 8080", cfg.Port)
	}
	if cfg.VectorBackend != "sqlite" {
		t.Errorf("VectorBackend = %q; want sqlite", cfg.VectorBackend)
	}
	if cfg.LLM.Provider != "openwebui" {
		t.Errorf("LLM.Provider = %q; want openwebui", cfg.LLM.Provider)
	}
	if cfg.API.RateLimit != 100 {
		t.Errorf("API.RateLimit = %d; want 100", cfg.API.RateLimit)
	}
}

func TestLoad_CustomValues(t *testing.T) {
	clearEnv(t)
	os.Setenv("OTTER_RAFT_ID", "r1")
	os.Setenv("OTTER_ENV", "production")
	os.Setenv("OTTER_PORT", "9090")
	os.Setenv("OTTER_LLM_PROVIDER", "openai")
	os.Setenv("OTTER_LLM_API_KEY", "sk-test")
	os.Setenv("OTTER_RATE_LIMIT_WINDOW", "5m")
	t.Cleanup(func() { clearEnv(t) })

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Env != "production" {
		t.Errorf("Env = %q", cfg.Env)
	}
	if cfg.Port != 9090 {
		t.Errorf("Port = %d", cfg.Port)
	}
	if cfg.LLM.Provider != "openai" {
		t.Errorf("LLM.Provider = %q", cfg.LLM.Provider)
	}
	if cfg.LLM.APIKey != "sk-test" {
		t.Errorf("LLM.APIKey = %q", cfg.LLM.APIKey)
	}
	if cfg.API.RateLimitWindow != 5*time.Minute {
		t.Errorf("RateLimitWindow = %v", cfg.API.RateLimitWindow)
	}
}

func TestValidate_EmptyRaftID(t *testing.T) {
	cfg := &Config{Raft: RaftConfig{ID: ""}, Port: 8080}
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for empty raft ID")
	}
}

func TestValidate_InvalidPort(t *testing.T) {
	for _, port := range []int{0, -1, 65536} {
		cfg := &Config{Raft: RaftConfig{ID: "r"}, Port: port}
		if err := cfg.Validate(); err == nil {
			t.Errorf("expected error for port %d", port)
		}
	}
}

func TestValidate_ValidConfig(t *testing.T) {
	cfg := &Config{Raft: RaftConfig{ID: "r"}, Port: 8080}
	if err := cfg.Validate(); err != nil {
		t.Errorf("Validate: %v", err)
	}
}

func TestValidate_BoundaryPorts(t *testing.T) {
	for _, port := range []int{1, 65535} {
		cfg := &Config{Raft: RaftConfig{ID: "r"}, Port: port}
		if err := cfg.Validate(); err != nil {
			t.Errorf("port %d should be valid: %v", port, err)
		}
	}
}

func TestGetEnv_Default(t *testing.T) {
	clearEnv(t)
	if v := getEnv("OTTER_NONEXISTENT", "default"); v != "default" {
		t.Errorf("got %q; want default", v)
	}
}

func TestGetEnv_Set(t *testing.T) {
	os.Setenv("OTTER_TEST_VAR", "value")
	t.Cleanup(func() { os.Unsetenv("OTTER_TEST_VAR") })

	if v := getEnv("OTTER_TEST_VAR", "default"); v != "value" {
		t.Errorf("got %q; want value", v)
	}
}

func TestGetEnvRequired_Missing(t *testing.T) {
	clearEnv(t)
	_, err := getEnvRequired("OTTER_NONEXISTENT")
	if err == nil {
		t.Error("expected error")
	}
}

func TestGetEnvRequired_Present(t *testing.T) {
	os.Setenv("OTTER_TEST_REQ", "val")
	t.Cleanup(func() { os.Unsetenv("OTTER_TEST_REQ") })

	v, err := getEnvRequired("OTTER_TEST_REQ")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if v != "val" {
		t.Errorf("got %q; want val", v)
	}
}

func TestGetEnvAsInt_Default(t *testing.T) {
	clearEnv(t)
	if v := getEnvAsInt("OTTER_NONEXISTENT", 42); v != 42 {
		t.Errorf("got %d; want 42", v)
	}
}

func TestGetEnvAsInt_Valid(t *testing.T) {
	os.Setenv("OTTER_INT_TEST", "99")
	t.Cleanup(func() { os.Unsetenv("OTTER_INT_TEST") })

	if v := getEnvAsInt("OTTER_INT_TEST", 0); v != 99 {
		t.Errorf("got %d; want 99", v)
	}
}

func TestGetEnvAsInt_Invalid(t *testing.T) {
	os.Setenv("OTTER_INT_BAD", "not_a_number")
	t.Cleanup(func() { os.Unsetenv("OTTER_INT_BAD") })

	if v := getEnvAsInt("OTTER_INT_BAD", 42); v != 42 {
		t.Errorf("got %d; want 42 (default)", v)
	}
}

func TestGetEnvAsDuration_Default(t *testing.T) {
	clearEnv(t)
	if v := getEnvAsDuration("OTTER_NONEXISTENT", 5*time.Second); v != 5*time.Second {
		t.Errorf("got %v", v)
	}
}

func TestGetEnvAsDuration_Valid(t *testing.T) {
	os.Setenv("OTTER_DUR_TEST", "30s")
	t.Cleanup(func() { os.Unsetenv("OTTER_DUR_TEST") })

	if v := getEnvAsDuration("OTTER_DUR_TEST", 0); v != 30*time.Second {
		t.Errorf("got %v; want 30s", v)
	}
}

func TestGetEnvAsDuration_Invalid(t *testing.T) {
	os.Setenv("OTTER_DUR_BAD", "nope")
	t.Cleanup(func() { os.Unsetenv("OTTER_DUR_BAD") })

	if v := getEnvAsDuration("OTTER_DUR_BAD", 10*time.Second); v != 10*time.Second {
		t.Errorf("got %v; want 10s (default)", v)
	}
}
