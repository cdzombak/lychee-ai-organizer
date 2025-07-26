package config

import (
	"encoding/json"
	"os"
)

type Config struct {
	Database DatabaseConfig `json:"database"`
	Ollama   OllamaConfig   `json:"ollama"`
	Server   ServerConfig   `json:"server"`
	Lychee   LycheeConfig   `json:"lychee"`
}

type DatabaseConfig struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Username string `json:"username"`
	Password string `json:"password"`
	Database string `json:"database"`
}

type OllamaConfig struct {
	Endpoint               string `json:"endpoint"`
	ImageAnalysisModel     string `json:"image_analysis_model"`
	DescriptionSynthesisModel string `json:"description_synthesis_model"`
}

type ServerConfig struct {
	Port int `json:"port"`
	Host string `json:"host"`
}

type LycheeConfig struct {
	BaseURL string `json:"base_url"`
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

	return &config, nil
}