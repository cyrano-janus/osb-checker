package config

import (
	"fmt"
	"io/ioutil"

	"gopkg.in/yaml.v2"
)

// Config represents the test configuration
type Config struct {
	BrokerURL     string `yaml:"broker_url"`
	Username      string `yaml:"username"`
	Password      string `yaml:"password"`
	APIVersion    string `yaml:"api_version"`
	AcceptsAsync  bool   `yaml:"accepts_async"`
	TestCatalog   bool   `yaml:"test_catalog"`
	TestProvision bool   `yaml:"test_provision"`
	TestBind      bool   `yaml:"test_bind"`
	TestUpdate    bool   `yaml:"test_update"`
	TestFetch     bool   `yaml:"test_fetch"`
}

// LoadConfig loads configuration from a YAML file
func LoadConfig(path string) (*Config, error) {
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	// Set defaults
	if cfg.APIVersion == "" {
		cfg.APIVersion = "2.17"
	}

	if cfg.BrokerURL == "" {
		return nil, fmt.Errorf("broker_url is required")
	}

	return &cfg, nil
}

// DefaultConfig returns a default configuration
func DefaultConfig() *Config {
	return &Config{
		APIVersion:    "2.17",
		AcceptsAsync:  true,
		TestCatalog:   true,
		TestProvision: true,
		TestBind:      true,
		TestUpdate:    true,
		TestFetch:     true,
	}
}