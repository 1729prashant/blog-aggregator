package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const configFileName = ".gatorconfig.json"

// Config struct represents the JSON file structure.
type Config struct {
	DbURL string `json:"db_url"`
	Name  string `json:"current_user_name"`
}

// getConfigFilePath returns the full path to the config file.
func getConfigFilePath() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("could not get home directory: %v", err)
	}
	return filepath.Join(homeDir, configFileName), nil
}

// Read reads the JSON file and returns a Config struct.
func Read() (Config, error) {
	filePath, err := getConfigFilePath()
	if err != nil {
		return Config{}, err
	}
	return readConfigFile(filePath)
}

// readConfigFile is an internal function to load the config file.
func readConfigFile(fileNameAndPath string) (Config, error) {
	var cfg Config

	data, err := os.ReadFile(fileNameAndPath)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, fmt.Errorf("config file does not exist: %v", err)
		}
		return cfg, fmt.Errorf("error loading config file: %v", err)
	}

	err = json.Unmarshal(data, &cfg)
	if err != nil {
		return cfg, fmt.Errorf("error parsing config file: %v", err)
	}

	return cfg, nil
}

// SetUser updates the current user name and writes the config back to the file.
func (cfg *Config) SetUser(name string) error {
	cfg.Name = name
	filePath, err := getConfigFilePath()
	if err != nil {
		return err
	}
	return writeConfigFile(*cfg, filePath)
}

// writeConfigFile writes the Config struct to the JSON file.
func writeConfigFile(cfg Config, fileNameAndPath string) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("error serializing config: %v", err)
	}

	err = os.MkdirAll(filepath.Dir(fileNameAndPath), 0755)
	if err != nil {
		return fmt.Errorf("error creating directory: %v", err)
	}

	err = os.WriteFile(fileNameAndPath, data, 0644)
	if err != nil {
		return fmt.Errorf("error saving config: %v", err)
	}

	return nil
}
