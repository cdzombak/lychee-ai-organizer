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
	AI       AIConfig       `json:"ai,omitempty"`
	Ollama   OllamaConfig   `json:"ollama,omitempty"`
	Server   ServerConfig   `json:"server"`
	Lychee   LycheeConfig   `json:"lychee"`
	Albums   AlbumsConfig   `json:"albums,omitempty"`
}

const (
	TypeMySQL                    = "mysql"
	TypePostgreSQL               = "postgresql"
	TypeSQLite                   = "sqlite"
	defaultMaxConcurrentRequests = 4
)

type DatabaseConfig struct {
	Type     string `json:"type"`
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Username string `json:"username"`
	Password string `json:"password"`
	Database string `json:"database"`
}

type AIConfig struct {
	Provider                  string                 `json:"provider"` // "ollama" or "openai"
	Endpoint                  string                 `json:"endpoint"`
	APIKey                    string                 `json:"api_key,omitempty"`
	ImageAnalysisModel        string                 `json:"image_analysis_model"`
	DescriptionSynthesisModel string                 `json:"description_synthesis_model"`
	ContextWindow             int                    `json:"context_window,omitempty"`
	Temperature               float64                `json:"temperature,omitempty"`
	TopP                      float64                `json:"top_p,omitempty"`
	MaxConcurrentRequests     int                    `json:"max_concurrent_requests,omitempty"`
	BatchSize                 int                    `json:"batch_size,omitempty"` // Number of photo descriptions to process before compaction (default: 30)
	Options                   map[string]interface{} `json:"options,omitempty"`
}

type OllamaConfig struct {
	Endpoint                  string                 `json:"endpoint"`
	ImageAnalysisModel        string                 `json:"image_analysis_model"`
	DescriptionSynthesisModel string                 `json:"description_synthesis_model"`
	ContextWindow             int                    `json:"context_window,omitempty"`
	Temperature               float64                `json:"temperature,omitempty"`
	TopP                      float64                `json:"top_p,omitempty"`
	MaxConcurrentRequests     int                    `json:"max_concurrent_requests,omitempty"`
	BatchSize                 int                    `json:"batch_size,omitempty"` // Number of photo descriptions to process before compaction (default: 30)
	Options                   map[string]interface{} `json:"options,omitempty"`
}

type ServerConfig struct {
	Port int    `json:"port"`
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
	if config.Database.Type == "" {
		return fmt.Errorf("database type is required (mysql, postgresql, or sqlite)")
	}

	validTypes := map[string]bool{TypeMySQL: true, TypePostgreSQL: true, TypeSQLite: true}
	if !validTypes[config.Database.Type] {
		return fmt.Errorf("database type must be one of: %s, %s, %s", TypeMySQL, TypePostgreSQL, TypeSQLite)
	}

	if config.Database.Type == TypeSQLite {
		if config.Database.Database == "" {
			return fmt.Errorf("database path is required for SQLite")
		}
	} else {
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
	}

	// Validate AI config - support both new AI config and legacy Ollama config
	hasAIConfig := config.AI.Endpoint != ""
	hasOllamaConfig := config.Ollama.Endpoint != ""

	if !hasAIConfig && !hasOllamaConfig {
		return fmt.Errorf("either 'ai' or 'ollama' configuration is required")
	}

	if hasAIConfig && hasOllamaConfig {
		return fmt.Errorf("cannot specify both 'ai' and 'ollama' configuration; use only 'ai'")
	}

	if hasAIConfig {
		// Validate new AI config
		if config.AI.Provider == "" {
			return fmt.Errorf("ai.provider is required (ollama or openai)")
		}
		if config.AI.MaxConcurrentRequests < 0 {
			return fmt.Errorf("ai.max_concurrent_requests must be zero or positive")
		}
		validProviders := map[string]bool{"ollama": true, "openai": true}
		if !validProviders[config.AI.Provider] {
			return fmt.Errorf("ai.provider must be either 'ollama' or 'openai'")
		}
		if _, err := url.Parse(config.AI.Endpoint); err != nil {
			return fmt.Errorf("invalid ai.endpoint URL: %w", err)
		}
		if config.AI.ImageAnalysisModel == "" {
			return fmt.Errorf("ai.image_analysis_model is required")
		}
		if config.AI.DescriptionSynthesisModel == "" {
			return fmt.Errorf("ai.description_synthesis_model is required")
		}
		if config.AI.Provider == "openai" && config.AI.APIKey == "" {
			return fmt.Errorf("ai.api_key is required when using openai provider")
		}
	} else {
		// Validate legacy Ollama config for backward compatibility
		if config.Ollama.MaxConcurrentRequests < 0 {
			return fmt.Errorf("ollama.max_concurrent_requests must be zero or positive")
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

// MaxConcurrentRequests returns the configured maximum number of concurrent LLM calls,
// defaulting to a safe value when unset.
func (config *Config) MaxConcurrentRequests() int {
	if config == nil {
		return defaultMaxConcurrentRequests
	}
	if config.AI.Endpoint != "" {
		if config.AI.MaxConcurrentRequests > 0 {
			return config.AI.MaxConcurrentRequests
		}
	}
	if config.Ollama.Endpoint != "" {
		if config.Ollama.MaxConcurrentRequests > 0 {
			return config.Ollama.MaxConcurrentRequests
		}
	}
	return defaultMaxConcurrentRequests
}
