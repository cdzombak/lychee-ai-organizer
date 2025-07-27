package websocket

import (
	"fmt"
	"log"
	"net/http"

	"github.com/gorilla/websocket"
	"lychee-ai-organizer/internal/database"
	"lychee-ai-organizer/internal/ollama"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

type Message struct {
	Type    string      `json:"type"`
	Payload interface{} `json:"payload"`
}

type ProgressUpdate struct {
	Stage       string `json:"stage"`
	Current     int    `json:"current"`
	Total       int    `json:"total"`
	Description string `json:"description"`
}

type ErrorSummary struct {
	PhotoErrors []string `json:"photo_errors"`
	AlbumErrors []string `json:"album_errors"`
	TotalErrors int      `json:"total_errors"`
}

type Handler struct {
	db     *database.DB
	ollama *ollama.Client
}

func NewHandler(db *database.DB, ollamaClient *ollama.Client) *Handler {
	return &Handler{
		db:     db,
		ollama: ollamaClient,
	}
}

func (h *Handler) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade error: %v", err)
		return
	}
	defer conn.Close()

	for {
		var msg Message
		if err := conn.ReadJSON(&msg); err != nil {
			log.Printf("WebSocket read error: %v", err)
			break
		}

		switch msg.Type {
		case "start_rescan":
			go h.handleRescan(conn)
		case "describe_photos":
			go h.handleDescribePhotos(conn)
		case "describe_all_albums":
			go h.handleDescribeAllAlbums(conn)
		case "retry_album_failures":
			go h.handleRetryAlbumFailures(conn)
		}
	}
}

func (h *Handler) handleRescan(conn *websocket.Conn) {
	// Get photos without AI descriptions (only process photos that don't have descriptions)
	photos, err := h.db.GetPhotosWithoutAIDescription()
	if err != nil {
		h.sendError(conn, "Failed to get photos: "+err.Error())
		return
	}

	// Get ALL top-level albums (rescan regenerates all album descriptions)
	albums, err := h.db.GetTopLevelAlbums()
	if err != nil {
		h.sendError(conn, "Failed to get albums: "+err.Error())
		return
	}

	totalWork := len(photos) + len(albums)
	if totalWork == 0 {
		h.sendMessage(conn, "complete", map[string]string{"message": "No photos or albums to process"})
		return
	}

	current := 0

	// Process photos
	for _, photo := range photos {
		current++
		h.sendProgress(conn, "photos", current, totalWork, "Processing photo: "+photo.Title)

		description, err := h.ollama.GeneratePhotoDescription(&photo)
		if err != nil {
			log.Printf("Error generating photo description for %s: %v", photo.ID, err)
			continue
		}

		if err := h.db.UpdatePhotoAIDescription(photo.ID, description); err != nil {
			log.Printf("Error saving photo description for %s: %v", photo.ID, err)
			continue
		}
	}

	// Process albums (regenerate all album descriptions)
	for _, album := range albums {
		current++
		h.sendProgress(conn, "albums", current, totalWork, "Regenerating album description: "+album.ID)

		albumPhotos, err := h.db.GetPhotosInAlbum(album.ID)
		if err != nil {
			log.Printf("Error getting photos for album %s: %v", album.ID, err)
			continue
		}

		if len(albumPhotos) == 0 {
			continue
		}

		description, err := h.ollama.GenerateAlbumDescription(&album, albumPhotos)
		if err != nil {
			log.Printf("Error generating album description for %s: %v", album.ID, err)
			continue
		}

		if err := h.db.UpdateAlbumAIDescription(album.ID, description); err != nil {
			log.Printf("Error saving album description for %s: %v", album.ID, err)
			continue
		}
	}

	h.sendMessage(conn, "complete", map[string]string{"message": "Rescan complete"})
}

func (h *Handler) sendProgress(conn *websocket.Conn, stage string, current, total int, description string) {
	update := ProgressUpdate{
		Stage:       stage,
		Current:     current,
		Total:       total,
		Description: description,
	}
	h.sendMessage(conn, "progress", update)
}

func (h *Handler) sendMessage(conn *websocket.Conn, msgType string, payload interface{}) {
	msg := Message{
		Type:    msgType,
		Payload: payload,
	}

	if err := conn.WriteJSON(msg); err != nil {
		log.Printf("WebSocket write error: %v", err)
	}
}

func (h *Handler) sendError(conn *websocket.Conn, errorMsg string) {
	h.sendMessage(conn, "error", map[string]string{"error": errorMsg})
}

func (h *Handler) handleDescribePhotos(conn *websocket.Conn) {
	// Get all photos without AI descriptions (unsorted + top-level albums)
	photos, err := h.db.GetAllPhotosWithoutAIDescription()
	if err != nil {
		h.sendError(conn, "Failed to get photos: "+err.Error())
		return
	}

	if len(photos) == 0 {
		h.sendMessage(conn, "complete", map[string]interface{}{
			"message": "No photos need descriptions",
			"errors":  ErrorSummary{PhotoErrors: []string{}, AlbumErrors: []string{}, TotalErrors: 0},
		})
		return
	}

	var photoErrors []string
	total := len(photos)

	// Process photos
	for i, photo := range photos {
		h.sendProgress(conn, "photos", i+1, total, "Processing photo: "+photo.Title)

		description, err := h.ollama.GeneratePhotoDescription(&photo)
		if err != nil {
			errorMsg := fmt.Sprintf("Photo %s (%s): %v", photo.ID, photo.Title, err)
			log.Printf("Error generating photo description for %s: %v", photo.ID, err)
			photoErrors = append(photoErrors, errorMsg)
			continue
		}

		if err := h.db.UpdatePhotoAIDescription(photo.ID, description); err != nil {
			errorMsg := fmt.Sprintf("Photo %s (%s): Failed to save description: %v", photo.ID, photo.Title, err)
			log.Printf("Error saving photo description for %s: %v", photo.ID, err)
			photoErrors = append(photoErrors, errorMsg)
			continue
		}
	}

	errorSummary := ErrorSummary{
		PhotoErrors: photoErrors,
		AlbumErrors: []string{},
		TotalErrors: len(photoErrors),
	}

	h.sendMessage(conn, "complete", map[string]interface{}{
		"message": fmt.Sprintf("Described %d photos", len(photos)-len(photoErrors)),
		"errors":  errorSummary,
	})
}

func (h *Handler) handleDescribeAllAlbums(conn *websocket.Conn) {
	// Get ALL top-level albums (regenerate all album descriptions)
	albums, err := h.db.GetTopLevelAlbums()
	if err != nil {
		h.sendError(conn, "Failed to get albums: "+err.Error())
		return
	}

	if len(albums) == 0 {
		h.sendMessage(conn, "complete", map[string]interface{}{
			"message": "No albums to describe",
			"errors":  ErrorSummary{PhotoErrors: []string{}, AlbumErrors: []string{}, TotalErrors: 0},
		})
		return
	}

	var albumErrors []string
	total := len(albums)

	// Process albums
	for i, album := range albums {
		h.sendProgress(conn, "albums", i+1, total, "Describing album: "+album.Title)

		albumPhotos, err := h.db.GetPhotosInAlbum(album.ID)
		if err != nil {
			errorMsg := fmt.Sprintf("Album %s (%s): Failed to get photos: %v", album.ID, album.Title, err)
			log.Printf("Error getting photos for album %s: %v", album.ID, err)
			albumErrors = append(albumErrors, errorMsg)
			continue
		}

		if len(albumPhotos) == 0 {
			errorMsg := fmt.Sprintf("Album %s (%s): No photos found", album.ID, album.Title)
			albumErrors = append(albumErrors, errorMsg)
			continue
		}

		description, err := h.ollama.GenerateAlbumDescription(&album, albumPhotos)
		if err != nil {
			errorMsg := fmt.Sprintf("Album %s (%s): %v", album.ID, album.Title, err)
			log.Printf("Error generating album description for %s: %v", album.ID, err)
			albumErrors = append(albumErrors, errorMsg)
			continue
		}

		if err := h.db.UpdateAlbumAIDescription(album.ID, description); err != nil {
			errorMsg := fmt.Sprintf("Album %s (%s): Failed to save description: %v", album.ID, album.Title, err)
			log.Printf("Error saving album description for %s: %v", album.ID, err)
			albumErrors = append(albumErrors, errorMsg)
			continue
		}
	}

	errorSummary := ErrorSummary{
		PhotoErrors: []string{},
		AlbumErrors: albumErrors,
		TotalErrors: len(albumErrors),
	}

	h.sendMessage(conn, "complete", map[string]interface{}{
		"message": fmt.Sprintf("Described %d albums", len(albums)-len(albumErrors)),
		"errors":  errorSummary,
	})
}

func (h *Handler) handleRetryAlbumFailures(conn *websocket.Conn) {
	// Get albums without AI descriptions
	albums, err := h.db.GetAlbumsWithoutAIDescription()
	if err != nil {
		h.sendError(conn, "Failed to get albums: "+err.Error())
		return
	}

	if len(albums) == 0 {
		h.sendMessage(conn, "complete", map[string]interface{}{
			"message": "No albums need descriptions",
			"errors":  ErrorSummary{PhotoErrors: []string{}, AlbumErrors: []string{}, TotalErrors: 0},
		})
		return
	}

	var albumErrors []string
	total := len(albums)

	// Process albums
	for i, album := range albums {
		h.sendProgress(conn, "albums", i+1, total, "Describing album: "+album.Title)

		albumPhotos, err := h.db.GetPhotosInAlbum(album.ID)
		if err != nil {
			errorMsg := fmt.Sprintf("Album %s (%s): Failed to get photos: %v", album.ID, album.Title, err)
			log.Printf("Error getting photos for album %s: %v", album.ID, err)
			albumErrors = append(albumErrors, errorMsg)
			continue
		}

		if len(albumPhotos) == 0 {
			errorMsg := fmt.Sprintf("Album %s (%s): No photos found", album.ID, album.Title)
			albumErrors = append(albumErrors, errorMsg)
			continue
		}

		description, err := h.ollama.GenerateAlbumDescription(&album, albumPhotos)
		if err != nil {
			errorMsg := fmt.Sprintf("Album %s (%s): %v", album.ID, album.Title, err)
			log.Printf("Error generating album description for %s: %v", album.ID, err)
			albumErrors = append(albumErrors, errorMsg)
			continue
		}

		if err := h.db.UpdateAlbumAIDescription(album.ID, description); err != nil {
			errorMsg := fmt.Sprintf("Album %s (%s): Failed to save description: %v", album.ID, album.Title, err)
			log.Printf("Error saving album description for %s: %v", album.ID, err)
			albumErrors = append(albumErrors, errorMsg)
			continue
		}
	}

	errorSummary := ErrorSummary{
		PhotoErrors: []string{},
		AlbumErrors: albumErrors,
		TotalErrors: len(albumErrors),
	}

	h.sendMessage(conn, "complete", map[string]interface{}{
		"message": fmt.Sprintf("Described %d albums", len(albums)-len(albumErrors)),
		"errors":  errorSummary,
	})
}