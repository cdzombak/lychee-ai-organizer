package websocket

import (
	"fmt"
	"log"
	"net/http"
	"sync"
	"sync/atomic"

	"github.com/gorilla/websocket"
	"lychee-ai-organizer/internal/ai"
	"lychee-ai-organizer/internal/database"
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
	db                    *database.DB
	aiProvider            ai.Provider
	maxConcurrentRequests int
	sendMu                sync.Mutex
}

func NewHandler(db *database.DB, aiProvider ai.Provider, maxConcurrentRequests int) *Handler {
	if maxConcurrentRequests <= 0 {
		maxConcurrentRequests = 4
	}
	return &Handler{
		db:                    db,
		aiProvider:            aiProvider,
		maxConcurrentRequests: maxConcurrentRequests,
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

	photoErrors := h.processPhotos(conn, photos, "photos", 0, totalWork)
	if len(photoErrors) > 0 {
		log.Printf("Rescan photo stage completed with %d errors", len(photoErrors))
	}

	albumErrors := h.processAlbums(conn, albums, "albums", len(photos), totalWork)
	if len(albumErrors) > 0 {
		log.Printf("Rescan album stage completed with %d errors", len(albumErrors))
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
	h.sendMu.Lock()
	defer h.sendMu.Unlock()
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

// processPhotos processes photo descriptions using a bounded worker pool so multiple
// LLM requests can run in parallel.
func (h *Handler) processPhotos(conn *websocket.Conn, photos []database.Photo, stage string, startIndex int, total int) []string {
	if len(photos) == 0 {
		return nil
	}
	if total == 0 {
		total = len(photos)
	}
	workers := h.maxConcurrentRequests
	if workers <= 0 {
		workers = 1
	}
	var (
		photoErrors []string
		errMu       sync.Mutex
		sem         = make(chan struct{}, workers)
		wg          sync.WaitGroup
		progress    atomic.Int32
	)

	for _, photo := range photos {
		photo := photo
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			current := startIndex + int(progress.Add(1))
			h.sendProgress(conn, stage, current, total, "Processing photo: "+photo.Title)

			description, err := h.aiProvider.GeneratePhotoDescription(&photo)
			if err != nil {
				errorMsg := fmt.Sprintf("Photo %s (%s): %v", photo.ID, photo.Title, err)
				log.Printf("Error generating photo description for %s: %v", photo.ID, err)
				errMu.Lock()
				photoErrors = append(photoErrors, errorMsg)
				errMu.Unlock()
				return
			}

			if err := h.db.UpdatePhotoAIDescription(photo.ID, description); err != nil {
				errorMsg := fmt.Sprintf("Photo %s (%s): Failed to save description: %v", photo.ID, photo.Title, err)
				log.Printf("Error saving photo description for %s: %v", photo.ID, err)
				errMu.Lock()
				photoErrors = append(photoErrors, errorMsg)
				errMu.Unlock()
				return
			}
		}()
	}

	wg.Wait()

	return photoErrors
}

// processAlbums mirrors processPhotos but works on album descriptions with the same
// concurrency controls.
func (h *Handler) processAlbums(conn *websocket.Conn, albums []database.Album, stage string, startIndex int, total int) []string {
	if len(albums) == 0 {
		return nil
	}
	if total == 0 {
		total = startIndex + len(albums)
	}
	workers := h.maxConcurrentRequests
	if workers <= 0 {
		workers = 1
	}
	var (
		albumErrors []string
		errMu       sync.Mutex
		sem         = make(chan struct{}, workers)
		wg          sync.WaitGroup
		progress    atomic.Int32
	)

	log.Printf("Starting processAlbums with %d albums (workers=%d)", len(albums), workers)
	for _, album := range albums {
		album := album
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			currentIndex := startIndex + int(progress.Add(1))
			h.sendProgress(conn, stage, currentIndex, total, "Describing album: "+album.Title)

			albumPhotos, err := h.db.GetPhotosInAlbum(album.ID)
			if err != nil {
				errorMsg := fmt.Sprintf("Album %s (%s): Failed to get photos: %v", album.ID, album.Title, err)
				log.Printf("Error getting photos for album %s: %v", album.ID, err)
				errMu.Lock()
				albumErrors = append(albumErrors, errorMsg)
				errMu.Unlock()
				return
			}

			if len(albumPhotos) == 0 {
				errorMsg := fmt.Sprintf("Album %s (%s): No photos found", album.ID, album.Title)
				log.Printf("No photos found for album %s (%s)", album.ID, album.Title)
				errMu.Lock()
				albumErrors = append(albumErrors, errorMsg)
				errMu.Unlock()
				return
			}

			description, err := h.aiProvider.GenerateAlbumDescription(&album, albumPhotos)
			if err != nil {
				errorMsg := fmt.Sprintf("Album %s (%s): %v", album.ID, album.Title, err)
				log.Printf("Error generating album description for %s: %v", album.ID, err)
				errMu.Lock()
				albumErrors = append(albumErrors, errorMsg)
				errMu.Unlock()
				return
			}

			if err := h.db.UpdateAlbumAIDescription(album.ID, description); err != nil {
				errorMsg := fmt.Sprintf("Album %s (%s): Failed to save description: %v", album.ID, album.Title, err)
				log.Printf("Error saving album description for %s: %v", album.ID, err)
				errMu.Lock()
				albumErrors = append(albumErrors, errorMsg)
				errMu.Unlock()
				return
			}

			log.Printf("Successfully processed album %s (%s)", album.ID, album.Title)
		}()
	}

	wg.Wait()
	log.Printf("Completed processAlbums: %d errors out of %d albums", len(albumErrors), len(albums))
	return albumErrors
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

	photoErrors := h.processPhotos(conn, photos, "photos", 0, len(photos))

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

	albumErrors := h.processAlbums(conn, albums, "albums", 0, len(albums))

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

	albumErrors := h.processAlbums(conn, albums, "albums", 0, len(albums))

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
