package config

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strings"
)

type Config struct {
	Database DatabaseConfig `json:"database"`
	Ollama   OllamaConfig   `json:"ollama"`
	Server   ServerConfig   `json:"server"`
	Lychee   LycheeConfig   `json:"lychee"`
	Albums   AlbumsConfig   `json:"albums,omitempty"`
}

type DatabaseConfig struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Username string `json:"username"`
	Password string `json:"password"`
	Database string `json:"database"`
}

type OllamaConfig struct {
	Endpoint                  string            `json:"endpoint"`
	ImageAnalysisModel        string            `json:"image_analysis_model"`
	DescriptionSynthesisModel string            `json:"description_synthesis_model"`
	ContextWindow             int               `json:"context_window,omitempty"`
	Temperature               float64           `json:"temperature,omitempty"`
	TopP                      float64           `json:"top_p,omitempty"`
	Options                   map[string]interface{} `json:"options,omitempty"`
}

type ServerConfig struct {
	Port int `json:"port"`
	Host string `json:"host"`
}

type LycheeConfig struct {
	BaseURL string `json:"base_url"`
}

type AlbumsConfig struct {
	Blocklist  []string `json:"blocklist,omitempty"`
	PinnedOnly bool     `json:"pinned_only,omitempty"`
}

func LoadConfig(configPath string) (*Config, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, err
	}

	var config Config
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, err
	}

	// Set defaults
	if config.Server.Host == "" {
		config.Server.Host = "localhost"
	}
	if config.Server.Port == 0 {
		config.Server.Port = 8080
	}

	// Validate configuration
	if err := validateConfig(&config); err != nil {
		return nil, fmt.Errorf("configuration validation failed: %w", err)
	}

	return &config, nil
}

// validateConfig validates the configuration and returns an error if invalid
func validateConfig(config *Config) error {
	// Validate database config
	if config.Database.Host == "" {
		return fmt.Errorf("database host is required")
	}
	if config.Database.Username == "" {
		return fmt.Errorf("database username is required")
	}
	if config.Database.Database == "" {
		return fmt.Errorf("database name is required")
	}
	if config.Database.Port <= 0 || config.Database.Port > 65535 {
		return fmt.Errorf("database port must be between 1 and 65535")
	}

	// Validate Ollama config
	if config.Ollama.Endpoint == "" {
		return fmt.Errorf("ollama endpoint is required")
	}
	if _, err := url.Parse(config.Ollama.Endpoint); err != nil {
		return fmt.Errorf("invalid ollama endpoint URL: %w", err)
	}
	if config.Ollama.ImageAnalysisModel == "" {
		return fmt.Errorf("ollama image analysis model is required")
	}
	if config.Ollama.DescriptionSynthesisModel == "" {
		return fmt.Errorf("ollama description synthesis model is required")
	}

	// Validate Lychee config
	if config.Lychee.BaseURL == "" {
		return fmt.Errorf("lychee base URL is required")
	}
	if _, err := url.Parse(config.Lychee.BaseURL); err != nil {
		return fmt.Errorf("invalid lychee base URL: %w", err)
	}
	// Remove trailing slash for consistency
	config.Lychee.BaseURL = strings.TrimSuffix(config.Lychee.BaseURL, "/")

	// Validate server config
	if config.Server.Port <= 0 || config.Server.Port > 65535 {
		return fmt.Errorf("server port must be between 1 and 65535")
	}

	return nil
}