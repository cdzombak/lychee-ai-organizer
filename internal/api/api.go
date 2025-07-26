package api

import (
	"encoding/json"
	"log"
	"net/http"

	"lychee-ai-organizer/internal/cache"
	"lychee-ai-organizer/internal/database"
	"lychee-ai-organizer/internal/ollama"
)

type Server struct {
	db          *database.DB
	ollama      *ollama.Client
	cache       *cache.Cache
	mux         *http.ServeMux
}

type PhotoResponse struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	TakenAt     string `json:"taken_at"`
	Thumbnail   string `json:"thumbnail"`
	Description string `json:"description"`
}

type AlbumResponse struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

type SuggestionResponse struct {
	Albums []AlbumResponse `json:"albums"`
}

type MovePhotoRequest struct {
	PhotoID string `json:"photo_id"`
	AlbumID string `json:"album_id"`
}

func NewServer(db *database.DB, ollamaClient *ollama.Client, cacheClient *cache.Cache) *Server {
	s := &Server{
		db:     db,
		ollama: ollamaClient,
		cache:  cacheClient,
		mux:    http.NewServeMux(),
	}

	s.setupRoutes()
	return s
}

func (s *Server) setupRoutes() {
	s.mux.HandleFunc("/api/photos/unsorted", s.handleUnsortedPhotos)
	s.mux.HandleFunc("/api/photos/suggestions", s.handlePhotoSuggestions)
	s.mux.HandleFunc("/api/photos/move", s.handleMovePhoto)
	s.mux.HandleFunc("/api/rescan", s.handleRescan)
	s.mux.HandleFunc("/", s.handleStatic)
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

func (s *Server) handleUnsortedPhotos(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	photos, err := s.db.GetUnsortedPhotos()
	if err != nil {
		log.Printf("Error getting unsorted photos: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	var response []PhotoResponse
	for _, photo := range photos {
		desc := ""
		if photo.AIDescription.Valid {
			desc = photo.AIDescription.String
		}

		takenAt := "Unknown"
		if photo.TakenAt.Valid {
			takenAt = photo.TakenAt.Time.Format("2006-01-02 15:04:05")
		}

		response = append(response, PhotoResponse{
			ID:          photo.ID,
			Title:       photo.Title,
			TakenAt:     takenAt,
			Thumbnail:   "/api/photos/" + photo.ID + "/thumbnail",
			Description: desc,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (s *Server) handlePhotoSuggestions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	photoID := r.URL.Query().Get("photo_id")
	if photoID == "" {
		http.Error(w, "photo_id parameter required", http.StatusBadRequest)
		return
	}

	suggestions, cached := s.cache.Get(photoID)
	if !cached {
		// Generate suggestions
		albums, err := s.db.GetTopLevelAlbums()
		if err != nil {
			log.Printf("Error getting albums: %v", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		photos, err := s.db.GetUnsortedPhotos()
		if err != nil {
			log.Printf("Error getting photos: %v", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		var targetPhoto *database.Photo
		for _, photo := range photos {
			if photo.ID == photoID {
				targetPhoto = &photo
				break
			}
		}

		if targetPhoto == nil {
			http.Error(w, "Photo not found", http.StatusNotFound)
			return
		}

		suggestions, err = s.ollama.GenerateAlbumSuggestions(targetPhoto, albums)
		if err != nil {
			log.Printf("Error generating suggestions: %v", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		s.cache.Set(photoID, suggestions)
		s.cache.Save()
	}

	albums, err := s.db.GetTopLevelAlbums()
	if err != nil {
		log.Printf("Error getting albums: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	albumMap := make(map[string]database.Album)
	for _, album := range albums {
		albumMap[album.ID] = album
	}

	var response SuggestionResponse
	for _, albumID := range suggestions {
		if album, exists := albumMap[albumID]; exists {
			desc := ""
			if album.AIDescription.Valid {
				desc = album.AIDescription.String
			}

			response.Albums = append(response.Albums, AlbumResponse{
				ID:          album.ID,
				Name:        album.ID, // Using ID as name for now
				Description: desc,
			})
		}
		if len(response.Albums) >= 3 {
			break
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (s *Server) handleMovePhoto(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req MovePhotoRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.PhotoID == "" || req.AlbumID == "" {
		http.Error(w, "photo_id and album_id are required", http.StatusBadRequest)
		return
	}

	if err := s.db.MovePhotoToAlbum(req.PhotoID, req.AlbumID); err != nil {
		log.Printf("Error moving photo: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "success"})
}

func (s *Server) handleRescan(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// This will be handled by WebSocket in a separate handler
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]string{"status": "rescan started"})
}

func (s *Server) handleStatic(w http.ResponseWriter, r *http.Request) {
	// Serve the React app - will be implemented with embed later
	http.ServeFile(w, r, "web/static/index.html")
}