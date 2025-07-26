package websocket

import (
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
		}
	}
}

func (h *Handler) handleRescan(conn *websocket.Conn) {
	// Get photos without AI descriptions
	photos, err := h.db.GetPhotosWithoutAIDescription()
	if err != nil {
		h.sendError(conn, "Failed to get photos: "+err.Error())
		return
	}

	// Get albums without AI descriptions
	albums, err := h.db.GetAlbumsWithoutAIDescription()
	if err != nil {
		h.sendError(conn, "Failed to get albums: "+err.Error())
		return
	}

	totalWork := len(photos) + len(albums)
	if totalWork == 0 {
		h.sendMessage(conn, "complete", map[string]string{"message": "No work needed - all descriptions are up to date"})
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

	// Process albums
	for _, album := range albums {
		current++
		h.sendProgress(conn, "albums", current, totalWork, "Processing album: "+album.ID)

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