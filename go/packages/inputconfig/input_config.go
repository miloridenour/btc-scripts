package inputconfig

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

var ErrConfigNotFound = fmt.Errorf("config file not found")

const configFile = "data/inputs/scriptInputs.json"

// SaveConfig serializes the struct and writes it to config.json
func SaveConfig(config any) error {
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	dir := filepath.Dir(configFile)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	err = os.WriteFile(configFile, data, 0644)
	if err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

// LoadConfig reads and deserializes config.json into a struct
// If the file doesn't exist, it creates a blank one with default values
func LoadConfig(config any) error {
	// Check if file exists
	if _, err := os.Stat(configFile); os.IsNotExist(err) {
		// File doesn't exist, create it with default values from the struct
		err := SaveConfig(config)
		if err != nil {
			return fmt.Errorf("failed to create default config: %w", err)
		}
		return ErrConfigNotFound
	}

	// File exists, read it
	data, err := os.ReadFile(configFile)
	if err != nil {
		return fmt.Errorf("failed to read config file: %w", err)
	}

	err = json.Unmarshal(data, config)
	if err != nil {
		return fmt.Errorf("failed to unmarshal config: %w", err)
	}

	return nil
}
