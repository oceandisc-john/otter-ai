package config

import (
	"fmt"
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

// Config holds all application configuration
type Config struct {
	Env           string
	Port          int
	DBPath        string
	VectorBackend string
	Raft          RaftConfig
	LLM           LLMConfig
	API           APIConfig
	Plugins       PluginConfig
}

// RaftConfig holds raft-specific configuration
type RaftConfig struct {
	ID            string
	Type          string // super-raft, raft, sub-raft
	BindAddr      string
	AdvertiseAddr string
	DataDir       string
}

// LLMConfig holds LLM provider configuration
type LLMConfig struct {
	Provider string
	Endpoint string
	Model    string
	APIKey   string
}

// APIConfig holds API server configuration
type APIConfig struct {
	Port int
	Host string
}

// PluginConfig holds plugin configuration
type PluginConfig struct {
	Enabled  []string
	Discord  PluginSettings
	Signal   PluginSettings
	Telegram PluginSettings
	Slack    PluginSettings
}

// PluginSettings holds generic plugin settings
type PluginSettings struct {
	Enabled bool
	Token   string
	Config  map[string]string
}

// Load reads configuration from environment variables and .env file
func Load() (*Config, error) {
	// Load .env file if it exists (development mode)
	_ = godotenv.Load()

	cfg := &Config{
		Env:           getEnv("OTTER_ENV", "development"),
		Port:          getEnvAsInt("OTTER_PORT", 8080),
		DBPath:        getEnv("OTTER_DB_PATH", "/data/otter.db"),
		VectorBackend: getEnv("OTTER_VECTOR_BACKEND", "sqlite"),
		Raft: RaftConfig{
			ID:            getEnvRequired("OTTER_RAFT_ID"),
			Type:          getEnv("OTTER_RAFT_TYPE", "raft"),
			BindAddr:      getEnv("OTTER_RAFT_BIND_ADDR", "127.0.0.1:7000"),
			AdvertiseAddr: getEnv("OTTER_RAFT_ADVERTISE_ADDR", "127.0.0.1:7000"),
			DataDir:       getEnv("OTTER_RAFT_DATA_DIR", "/data/raft"),
		},
		LLM: LLMConfig{
			Provider: getEnv("OTTER_LLM_PROVIDER", "openwebui"),
			Endpoint: getEnv("OTTER_LLM_ENDPOINT", "http://localhost:11434"),
			Model:    getEnv("OTTER_LLM_MODEL", "llama2"),
			APIKey:   getEnv("OTTER_LLM_API_KEY", ""),
		},
		API: APIConfig{
			Port: getEnvAsInt("OTTER_PORT", 8080),
			Host: getEnv("OTTER_HOST", "0.0.0.0"),
		},
		Plugins: PluginConfig{
			Enabled: []string{},
		},
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return cfg, nil
}

// Validate ensures the configuration is valid
func (c *Config) Validate() error {
	if c.Raft.ID == "" {
		return fmt.Errorf("OTTER_RAFT_ID is required")
	}

	validRaftTypes := map[string]bool{
		"super-raft": true,
		"raft":       true,
		"sub-raft":   true,
	}
	if !validRaftTypes[c.Raft.Type] {
		return fmt.Errorf("invalid OTTER_RAFT_TYPE: %s (must be super-raft, raft, or sub-raft)", c.Raft.Type)
	}

	if c.Port < 1 || c.Port > 65535 {
		return fmt.Errorf("invalid port: %d", c.Port)
	}

	return nil
}

// getEnv retrieves an environment variable or returns a default value
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// getEnvRequired retrieves a required environment variable or fails
func getEnvRequired(key string) string {
	value := os.Getenv(key)
	if value == "" {
		panic(fmt.Sprintf("Required environment variable %s is not set", key))
	}
	return value
}

// getEnvAsInt retrieves an environment variable as an integer or returns a default value
func getEnvAsInt(key string, defaultValue int) int {
	valueStr := os.Getenv(key)
	if valueStr == "" {
		return defaultValue
	}
	value, err := strconv.Atoi(valueStr)
	if err != nil {
		return defaultValue
	}
	return value
}
