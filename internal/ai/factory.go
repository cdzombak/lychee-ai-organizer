package ai

import (
	"fmt"

	"lychee-ai-organizer/internal/config"
	"lychee-ai-organizer/internal/database"
	"lychee-ai-organizer/internal/images"
)

func NewProvider(cfg *config.Config, db *database.DB, imageFetcher *images.Fetcher) (Provider, error) {
	hasAIConfig := cfg.AI.Endpoint != ""
	
	if hasAIConfig {
		switch cfg.AI.Provider {
		case "ollama":
			ollamaCfg := &config.OllamaConfig{
				Endpoint:                  cfg.AI.Endpoint,
				ImageAnalysisModel:        cfg.AI.ImageAnalysisModel,
				DescriptionSynthesisModel: cfg.AI.DescriptionSynthesisModel,
				ContextWindow:             cfg.AI.ContextWindow,
				Temperature:               cfg.AI.Temperature,
				TopP:                      cfg.AI.TopP,
				Options:                   cfg.AI.Options,
			}
			return NewOllamaClient(ollamaCfg, db, imageFetcher)
		case "openai":
			return NewOpenAIClient(&cfg.AI, db, imageFetcher)
		default:
			return nil, fmt.Errorf("unsupported AI provider: %s", cfg.AI.Provider)
		}
	}
	
	return NewOllamaClient(&cfg.Ollama, db, imageFetcher)
}
