package config

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
)

// GUIConfig stores desktop GUI-specific settings that should not live in the
// main server config.
type GUIConfig struct {
	Auth GUIAuthConfig `json:"auth"`
}

// GUIAuthConfig stores desktop GUI authentication settings.
type GUIAuthConfig struct {
	CountryCode string `json:"country_code"`
	PhoneNumber string `json:"phone_number"`
}

// GUIConfigFilePath returns the GUI-specific config file path.
func GUIConfigFilePath() string {
	return filepath.Join(XbotHome(), "gui-config.json")
}

// LoadGUIConfig loads the GUI-specific config, returning defaults when missing.
func LoadGUIConfig() *GUIConfig {
	path := GUIConfigFilePath()
	data, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			slog.Warn("failed to read GUI config, using defaults", "path", path, "error", err)
		}
		return &GUIConfig{}
	}

	var cfg GUIConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		slog.Warn("failed to parse GUI config, using defaults", "path", path, "error", err)
		return &GUIConfig{}
	}
	return &cfg
}

// SaveGUIConfig saves the GUI-specific config to disk.
func SaveGUIConfig(cfg *GUIConfig) error {
	if cfg == nil {
		cfg = &GUIConfig{}
	}
	return saveJSONFile(GUIConfigFilePath(), cfg)
}

// EnsureGUIConfig creates the GUI config file if it does not exist yet.
func EnsureGUIConfig() (*GUIConfig, error) {
	path := GUIConfigFilePath()
	cfg := LoadGUIConfig()
	if _, err := os.Stat(path); err == nil {
		return cfg, nil
	} else if !os.IsNotExist(err) {
		return cfg, err
	}
	if err := SaveGUIConfig(cfg); err != nil {
		return cfg, err
	}
	return cfg, nil
}

func saveJSONFile(path string, value interface{}) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("write temp config: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("rename config: %w", err)
	}
	return nil
}
