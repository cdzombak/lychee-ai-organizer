package main

import (
	_ "embed"
	"fmt"
	"log"
	"net/http"

	"lychee-ai-organizer/internal/api"
	"lychee-ai-organizer/internal/cache"
	"lychee-ai-organizer/internal/config"
	"lychee-ai-organizer/internal/database"
	"lychee-ai-organizer/internal/images"
	"lychee-ai-organizer/internal/ollama"
	"lychee-ai-organizer/internal/websocket"
)

//go:embed web/static/index.html
var indexHTML []byte

type App struct {
	config     *config.Config
	configPath string
	cachePath  string
	db         *database.DB
	ollama     *ollama.Client
	apiServer  *api.Server
	wsHandler  *websocket.Handler
	cache      *cache.Cache
}

func NewApp(configPath, cachePath string) *App {
	return &App{
		configPath: configPath,
		cachePath:  cachePath,
	}
}

func (app *App) Run() error {
	// Load configuration
	cfg, err := config.LoadConfig(app.configPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}
	app.config = cfg

	// Initialize database
	db, err := database.NewDB(&cfg.Database)
	if err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}
	defer db.Close()
	app.db = db

	// Initialize image fetcher
	imageFetcher := images.NewFetcher(&cfg.Lychee)

	// Initialize Ollama client
	ollamaClient, err := ollama.NewClient(&cfg.Ollama, db, imageFetcher)
	if err != nil {
		return fmt.Errorf("failed to initialize Ollama client: %w", err)
	}
	app.ollama = ollamaClient

	// Initialize cache
	cacheClient := cache.NewCache(app.cachePath)
	if err := cacheClient.Load(); err != nil {
		log.Printf("Warning: failed to load cache: %v", err)
	}
	app.cache = cacheClient

	// Initialize API server
	app.apiServer = api.NewServer(db, ollamaClient, cacheClient)

	// Initialize WebSocket handler
	app.wsHandler = websocket.NewHandler(db, ollamaClient)

	// Set up HTTP routes
	http.HandleFunc("/", app.handleIndex)
	http.Handle("/api/", app.apiServer)
	http.HandleFunc("/ws", app.wsHandler.HandleWebSocket)

	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	log.Printf("Starting server on %s", addr)
	
	return http.ListenAndServe(addr, nil)
}

func (app *App) handleIndex(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	w.Write(indexHTML)
}