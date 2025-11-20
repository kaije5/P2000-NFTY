package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadWithValidConfig(t *testing.T) {
	// Create a temporary config file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	configContent := `
forward_all: false
capcodes:
  - "0123456"
  - "0234567"
capcode_translations:
  "0123456": "Fire Department"
  "0234567": "Ambulance"
capcode_csv_path: "/custom/path/capcodes.csv"
ntfy:
  server: "https://ntfy.example.com"
  topic: "test-topic"
  token: "test-token"
  username: "test-user"
  password: "test-pass"
`

	err := os.WriteFile(configPath, []byte(configContent), 0644)
	require.NoError(t, err)

	cfg, err := Load(configPath)
	require.NoError(t, err)
	require.NotNil(t, cfg)

	// Verify all fields are loaded correctly
	assert.False(t, cfg.ForwardAll)
	assert.Equal(t, []string{"0123456", "0234567"}, cfg.Capcodes)
	assert.Equal(t, "Fire Department", cfg.CapcodeTranslations["0123456"])
	assert.Equal(t, "Ambulance", cfg.CapcodeTranslations["0234567"])
	assert.Equal(t, "/custom/path/capcodes.csv", cfg.CapcodeCSVPath)
	assert.Equal(t, "https://ntfy.example.com", cfg.Ntfy.Server)
	assert.Equal(t, "test-topic", cfg.Ntfy.Topic)
	assert.Equal(t, "test-token", cfg.Ntfy.Token)
	assert.Equal(t, "test-user", cfg.Ntfy.Username)
	assert.Equal(t, "test-pass", cfg.Ntfy.Password)

	// Verify default server config
	assert.Equal(t, 8080, cfg.Server.Port)
	assert.Equal(t, "/health", cfg.Server.HealthPath)
	assert.Equal(t, "/metrics", cfg.Server.MetricsPath)
	assert.Equal(t, 10, cfg.Server.ReadTimeout)
	assert.Equal(t, 10, cfg.Server.WriteTimeout)
}

func TestLoadWithDefaults(t *testing.T) {
	// Load with empty config path to test defaults
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "minimal.yaml")

	minimalConfig := `
ntfy:
  server: "https://ntfy.sh"
  topic: "alerts"
`

	err := os.WriteFile(configPath, []byte(minimalConfig), 0644)
	require.NoError(t, err)

	cfg, err := Load(configPath)
	require.NoError(t, err)
	require.NotNil(t, cfg)

	// Verify defaults
	assert.True(t, cfg.ForwardAll)
	assert.Empty(t, cfg.Capcodes)
	assert.Equal(t, "capcodelijst.csv", cfg.CapcodeCSVPath)
	assert.Equal(t, 8080, cfg.Server.Port)
}

func TestLoadWithEmptyPath(t *testing.T) {
	// Set required env vars to make config valid
	os.Setenv("NTFY_SERVER", "https://ntfy.sh")
	os.Setenv("NTFY_TOPIC", "test")
	defer func() {
		os.Unsetenv("NTFY_SERVER")
		os.Unsetenv("NTFY_TOPIC")
	}()

	cfg, err := Load("")
	require.NoError(t, err)
	require.NotNil(t, cfg)

	// Should use defaults when no config file
	assert.True(t, cfg.ForwardAll)
	assert.Equal(t, "https://ntfy.sh", cfg.Ntfy.Server)
	assert.Equal(t, "test", cfg.Ntfy.Topic)
}

func TestLoadWithInvalidPath(t *testing.T) {
	cfg, err := Load("/nonexistent/path/config.yaml")
	assert.Error(t, err)
	assert.Nil(t, cfg)
	assert.Contains(t, err.Error(), "failed to read config file")
}

func TestLoadWithInvalidYAML(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "invalid.yaml")

	invalidYAML := `
forward_all: true
capcodes: [
  incomplete yaml
`

	err := os.WriteFile(configPath, []byte(invalidYAML), 0644)
	require.NoError(t, err)

	cfg, err := Load(configPath)
	assert.Error(t, err)
	assert.Nil(t, cfg)
	assert.Contains(t, err.Error(), "failed to parse config file")
}

func TestEnvironmentVariableOverrides(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	configContent := `
forward_all: true
capcodes:
  - "0101001"
ntfy:
  server: "https://ntfy.sh"
  topic: "default-topic"
  token: "default-token"
`

	err := os.WriteFile(configPath, []byte(configContent), 0644)
	require.NoError(t, err)

	// Set environment variables
	os.Setenv("FORWARD_ALL", "false")
	os.Setenv("NTFY_SERVER", "https://custom.ntfy.sh")
	os.Setenv("NTFY_TOPIC", "custom-topic")
	os.Setenv("NTFY_TOKEN", "custom-token")
	os.Setenv("NTFY_USERNAME", "env-user")
	os.Setenv("NTFY_PASSWORD", "env-pass")
	os.Setenv("SERVER_PORT", "9090")
	os.Setenv("CAPCODE_CSV_PATH", "/env/path/capcodes.csv")

	defer func() {
		os.Unsetenv("FORWARD_ALL")
		os.Unsetenv("NTFY_SERVER")
		os.Unsetenv("NTFY_TOPIC")
		os.Unsetenv("NTFY_TOKEN")
		os.Unsetenv("NTFY_USERNAME")
		os.Unsetenv("NTFY_PASSWORD")
		os.Unsetenv("SERVER_PORT")
		os.Unsetenv("CAPCODE_CSV_PATH")
	}()

	cfg, err := Load(configPath)
	require.NoError(t, err)
	require.NotNil(t, cfg)

	// Verify environment variables override config file
	assert.False(t, cfg.ForwardAll)
	assert.Equal(t, "https://custom.ntfy.sh", cfg.Ntfy.Server)
	assert.Equal(t, "custom-topic", cfg.Ntfy.Topic)
	assert.Equal(t, "custom-token", cfg.Ntfy.Token)
	assert.Equal(t, "env-user", cfg.Ntfy.Username)
	assert.Equal(t, "env-pass", cfg.Ntfy.Password)
	assert.Equal(t, 9090, cfg.Server.Port)
	assert.Equal(t, "/env/path/capcodes.csv", cfg.CapcodeCSVPath)
}

func TestEnvironmentVariableInvalidValues(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	configContent := `
ntfy:
  server: "https://ntfy.sh"
  topic: "test"
`

	err := os.WriteFile(configPath, []byte(configContent), 0644)
	require.NoError(t, err)

	// Set invalid environment variables (should be ignored)
	os.Setenv("FORWARD_ALL", "not-a-bool")
	os.Setenv("SERVER_PORT", "not-a-number")

	defer func() {
		os.Unsetenv("FORWARD_ALL")
		os.Unsetenv("SERVER_PORT")
	}()

	cfg, err := Load(configPath)
	require.NoError(t, err)
	require.NotNil(t, cfg)

	// Should use defaults when env vars are invalid
	assert.True(t, cfg.ForwardAll) // default
	assert.Equal(t, 8080, cfg.Server.Port) // default
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name        string
		config      Config
		expectError bool
		errorMsg    string
	}{
		{
			name: "Valid config with ForwardAll true",
			config: Config{
				ForwardAll: true,
				Ntfy: NtfyConfig{
					Server: "https://ntfy.sh",
					Topic:  "test",
				},
			},
			expectError: false,
		},
		{
			name: "Valid config with ForwardAll false and capcodes",
			config: Config{
				ForwardAll: false,
				Capcodes:   []string{"0123456"},
				Ntfy: NtfyConfig{
					Server: "https://ntfy.sh",
					Topic:  "test",
				},
			},
			expectError: false,
		},
		{
			name: "Invalid: ForwardAll false without capcodes",
			config: Config{
				ForwardAll: false,
				Capcodes:   []string{},
				Ntfy: NtfyConfig{
					Server: "https://ntfy.sh",
					Topic:  "test",
				},
			},
			expectError: true,
			errorMsg:    "at least one capcode must be configured",
		},
		{
			name: "Invalid: Missing ntfy server",
			config: Config{
				ForwardAll: true,
				Ntfy: NtfyConfig{
					Topic: "test",
				},
			},
			expectError: true,
			errorMsg:    "ntfy server must be configured",
		},
		{
			name: "Invalid: Missing ntfy topic",
			config: Config{
				ForwardAll: true,
				Ntfy: NtfyConfig{
					Server: "https://ntfy.sh",
				},
			},
			expectError: true,
			errorMsg:    "ntfy topic must be configured",
		},
		{
			name: "Valid: Empty capcodes with ForwardAll true",
			config: Config{
				ForwardAll: true,
				Capcodes:   []string{},
				Ntfy: NtfyConfig{
					Server: "https://ntfy.sh",
					Topic:  "test",
				},
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()

			if tt.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestLoadWithValidationFailure(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "invalid-config.yaml")

	// Config that will fail validation (missing ntfy server)
	invalidConfig := `
forward_all: true
ntfy:
  topic: "test"
`

	err := os.WriteFile(configPath, []byte(invalidConfig), 0644)
	require.NoError(t, err)

	cfg, err := Load(configPath)
	assert.Error(t, err)
	assert.Nil(t, cfg)
	assert.Contains(t, err.Error(), "invalid configuration")
}

func TestConfigStructDefaults(t *testing.T) {
	cfg := &Config{
		ForwardAll:     true,
		CapcodeCSVPath: "capcodelijst.csv",
		Server: ServerConfig{
			Port:         8080,
			HealthPath:   "/health",
			MetricsPath:  "/metrics",
			ReadTimeout:  10,
			WriteTimeout: 10,
		},
	}

	assert.True(t, cfg.ForwardAll)
	assert.Equal(t, "capcodelijst.csv", cfg.CapcodeCSVPath)
	assert.Equal(t, 8080, cfg.Server.Port)
	assert.Equal(t, "/health", cfg.Server.HealthPath)
	assert.Equal(t, "/metrics", cfg.Server.MetricsPath)
	assert.Equal(t, 10, cfg.Server.ReadTimeout)
	assert.Equal(t, 10, cfg.Server.WriteTimeout)
}

func TestComplexCapcodeTranslations(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	configContent := `
forward_all: false
capcodes:
  - "0101001"
  - "0101002"
  - "0101003"
capcode_translations:
  "0101001": "Brandweer Utrecht"
  "0101002": "Ambulance Utrecht"
  "0101003": "Politie Utrecht"
ntfy:
  server: "https://ntfy.sh"
  topic: "utrecht-alerts"
`

	err := os.WriteFile(configPath, []byte(configContent), 0644)
	require.NoError(t, err)

	cfg, err := Load(configPath)
	require.NoError(t, err)
	require.NotNil(t, cfg)

	assert.Equal(t, 3, len(cfg.Capcodes))
	assert.Equal(t, 3, len(cfg.CapcodeTranslations))
	assert.Equal(t, "Brandweer Utrecht", cfg.CapcodeTranslations["0101001"])
	assert.Equal(t, "Ambulance Utrecht", cfg.CapcodeTranslations["0101002"])
	assert.Equal(t, "Politie Utrecht", cfg.CapcodeTranslations["0101003"])
}
