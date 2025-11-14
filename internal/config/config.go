package config

import (
	"fmt"
	"os"
	"strconv"

	"gopkg.in/yaml.v3"
)

// Config holds the application configuration
type Config struct {
	ForwardAll          bool              `yaml:"forward_all"`
	Capcodes            []string          `yaml:"capcodes"`
	CapcodeTranslations map[string]string `yaml:"capcode_translations"`
	CapcodeCSVPath      string            `yaml:"capcode_csv_path"`
	Ntfy                NtfyConfig        `yaml:"ntfy"`
	Server              ServerConfig
}

// NtfyConfig holds ntfy.sh configuration
type NtfyConfig struct {
	Server   string `yaml:"server"`
	Topic    string `yaml:"topic"`
	Token    string `yaml:"token"`    // Optional authentication token (Bearer)
	Username string `yaml:"username"` // Optional username for Basic Auth
	Password string `yaml:"password"` // Optional password for Basic Auth
}

// ServerConfig holds HTTP server configuration
type ServerConfig struct {
	Port         int
	HealthPath   string
	MetricsPath  string
	ReadTimeout  int // seconds
	WriteTimeout int // seconds
}

// Load reads configuration from file and environment variables
func Load(configPath string) (*Config, error) {
	cfg := &Config{
		ForwardAll:     true,              // Default to forwarding all messages
		CapcodeCSVPath: "capcodelijst.csv", // Default CSV path
		Server: ServerConfig{
			Port:         8080,
			HealthPath:   "/health",
			MetricsPath:  "/metrics",
			ReadTimeout:  10,
			WriteTimeout: 10,
		},
	}

	// Read config file
	if configPath != "" {
		data, err := os.ReadFile(configPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read config file: %w", err)
		}

		if err := yaml.Unmarshal(data, cfg); err != nil {
			return nil, fmt.Errorf("failed to parse config file: %w", err)
		}
	}

	// Environment variable overrides
	if forwardAll := os.Getenv("FORWARD_ALL"); forwardAll != "" {
		if fa, err := strconv.ParseBool(forwardAll); err == nil {
			cfg.ForwardAll = fa
		}
	}
	if server := os.Getenv("NTFY_SERVER"); server != "" {
		cfg.Ntfy.Server = server
	}
	if topic := os.Getenv("NTFY_TOPIC"); topic != "" {
		cfg.Ntfy.Topic = topic
	}
	if token := os.Getenv("NTFY_TOKEN"); token != "" {
		cfg.Ntfy.Token = token
	}
	if username := os.Getenv("NTFY_USERNAME"); username != "" {
		cfg.Ntfy.Username = username
	}
	if password := os.Getenv("NTFY_PASSWORD"); password != "" {
		cfg.Ntfy.Password = password
	}
	if port := os.Getenv("SERVER_PORT"); port != "" {
		if p, err := strconv.Atoi(port); err == nil {
			cfg.Server.Port = p
		}
	}
	if csvPath := os.Getenv("CAPCODE_CSV_PATH"); csvPath != "" {
		cfg.CapcodeCSVPath = csvPath
	}

	// Validate required fields
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return cfg, nil
}

// Validate checks if all required configuration fields are set
func (c *Config) Validate() error {
	// If ForwardAll is false, we need at least one capcode for filtering
	if !c.ForwardAll && len(c.Capcodes) == 0 {
		return fmt.Errorf("at least one capcode must be configured when forward_all is false")
	}
	if c.Ntfy.Server == "" {
		return fmt.Errorf("ntfy server must be configured")
	}
	if c.Ntfy.Topic == "" {
		return fmt.Errorf("ntfy topic must be configured")
	}
	return nil
}
