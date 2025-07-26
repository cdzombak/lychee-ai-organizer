package api

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"strconv"

	"lychee-ai-organizer/internal/cache"
	"lychee-ai-organizer/internal/database"
	"lychee-ai-organizer/internal/images"
	"lychee-ai-organizer/internal/ollama"
)

type Server struct {
	db           *database.DB
	ollama       *ollama.Client
	cache        *cache.Cache
	imageFetcher *images.Fetcher
	mux          *http.ServeMux
}

type PhotoResponse struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	TakenAt     string `json:"taken_at"`
	Thumbnail   string `json:"thumbnail"`
	FullSize    string `json:"full_size"`
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

func NewServer(db *database.DB, ollamaClient *ollama.Client, cacheClient *cache.Cache, imageFetcher *images.Fetcher) *Server {
	s := &Server{
		db:           db,
		ollama:       ollamaClient,
		cache:        cacheClient,
		imageFetcher: imageFetcher,
		mux:          http.NewServeMux(),
	}

	s.setupRoutes()
	return s
}

func (s *Server) setupRoutes() {
	s.mux.HandleFunc("/api/health", s.handleHealth)
	s.mux.HandleFunc("/api/photos/unsorted", s.handleUnsortedPhotos)
	s.mux.HandleFunc("/api/photos/suggestions", s.handlePhotoSuggestions)
	s.mux.HandleFunc("/api/photos/move", s.handleMovePhoto)
	s.mux.HandleFunc("/api/rescan", s.handleRescan)
	s.mux.HandleFunc("/", s.handleStatic)
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	// Add CORS headers
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	
	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok", "service": "lychee-ai-organizer"})
}

func (s *Server) handleUnsortedPhotos(w http.ResponseWriter, r *http.Request) {
	// Add CORS headers
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	
	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}
	
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	photoData, err := s.getUnsortedPhotosWithVariants()
	if err != nil {
		log.Printf("Error getting unsorted photos with variants: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	var response []PhotoResponse
	for _, data := range photoData {
		desc := ""
		if data.Photo.AIDescription.Valid {
			desc = data.Photo.AIDescription.String
		}

		takenAt := "Unknown"
		if data.Photo.TakenAt.Valid {
			takenAt = data.Photo.TakenAt.Time.Format("2006-01-02 15:04:05")
		}

		// Get URLs from variants
		thumbnailURL := s.selectBestVariantURL(data.Variants, true)
		fullSizeURL := s.selectBestVariantURL(data.Variants, false)

		response = append(response, PhotoResponse{
			ID:          data.Photo.ID,
			Title:       data.Photo.Title,
			TakenAt:     takenAt,
			Thumbnail:   thumbnailURL,
			FullSize:    fullSizeURL,
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

type PhotoWithVariants struct {
	Photo    database.Photo
	Variants []database.SizeVariant
}

func (s *Server) getUnsortedPhotosWithVariants() ([]PhotoWithVariants, error) {
	query := `
		SELECT 
			p.id, p.created_at, p.updated_at, p.owner_id, p.old_album_id, p.title, p.description, 
			p.tags, p.license, p.is_starred, p.iso, p.make, p.model, p.lens, p.aperture, p.shutter, 
			p.focal, p.latitude, p.longitude, p.altitude, p.img_direction, p.location, p.taken_at, 
			p.taken_at_orig_tz, p.initial_taken_at, p.initial_taken_at_orig_tz, p.type, 
			p.filesize, p.checksum, p.original_checksum, p.live_photo_short_path, 
			p.live_photo_content_id, p.live_photo_checksum, p._ai_description, p._ai_description_ts,
			sv.id as variant_id, sv.type as variant_type, sv.short_path, sv.width, sv.height, 
			sv.ratio, sv.filesize as variant_filesize, sv.storage_disk
		FROM photos p
		LEFT JOIN size_variants sv ON p.id = sv.photo_id
		WHERE p.id NOT IN (SELECT photo_id FROM photo_album)
		ORDER BY p.taken_at DESC, p.created_at DESC, sv.type DESC`

	rows, err := s.db.GetDB().Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	photoMap := make(map[string]*PhotoWithVariants)
	
	for rows.Next() {
		var photo database.Photo
		var variantID, variantType, shortPath, storageDisk, ratioStr sql.NullString
		var width, height, variantFilesize sql.NullInt64

		err := rows.Scan(
			&photo.ID, &photo.CreatedAt, &photo.UpdatedAt, &photo.OwnerID,
			&photo.OldAlbumID, &photo.Title, &photo.Description, &photo.Tags,
			&photo.License, &photo.IsStarred, &photo.ISO, &photo.Make, &photo.Model,
			&photo.Lens, &photo.Aperture, &photo.Shutter, &photo.Focal,
			&photo.Latitude, &photo.Longitude, &photo.Altitude, &photo.ImgDirection,
			&photo.Location, &photo.TakenAt, &photo.TakenAtOrigTz, &photo.InitialTakenAt,
			&photo.InitialTakenAtOrigTz, &photo.Type, &photo.Filesize, &photo.Checksum,
			&photo.OriginalChecksum, &photo.LivePhotoShortPath, &photo.LivePhotoContentID,
			&photo.LivePhotoChecksum, &photo.AIDescription, &photo.AIDescriptionTimestamp,
			&variantID, &variantType, &shortPath, &width, &height,
			&ratioStr, &variantFilesize, &storageDisk,
		)
		if err != nil {
			return nil, err
		}

		// Initialize photo data if not seen before
		if _, exists := photoMap[photo.ID]; !exists {
			photoMap[photo.ID] = &PhotoWithVariants{
				Photo:    photo,
				Variants: []database.SizeVariant{},
			}
		}

		// Add variant if it exists
		if variantID.Valid {
			variantIDInt, _ := strconv.ParseInt(variantID.String, 10, 64)
			variantTypeInt, _ := strconv.Atoi(variantType.String)
			
			// Parse ratio as float64
			var ratio float64
			if ratioStr.Valid {
				ratio, _ = strconv.ParseFloat(ratioStr.String, 64)
			}
			
			variant := database.SizeVariant{
				ID:          variantIDInt,
				PhotoID:     photo.ID,
				Type:        variantTypeInt,
				ShortPath:   shortPath.String,
				Width:       int(width.Int64),
				Height:      int(height.Int64),
				Ratio:       ratio,
				Filesize:    variantFilesize.Int64,
				StorageDisk: storageDisk.String,
			}
			photoMap[photo.ID].Variants = append(photoMap[photo.ID].Variants, variant)
		}
	}

	// Convert map to slice maintaining order
	var result []PhotoWithVariants
	seenPhotos := make(map[string]bool)
	
	// Re-run a simpler query to maintain proper order
	orderQuery := `
		SELECT id FROM photos 
		WHERE id NOT IN (SELECT photo_id FROM photo_album)
		ORDER BY taken_at DESC, created_at DESC`
	
	orderRows, err := s.db.GetDB().Query(orderQuery)
	if err != nil {
		return nil, err
	}
	defer orderRows.Close()
	
	for orderRows.Next() {
		var photoID string
		if err := orderRows.Scan(&photoID); err != nil {
			return nil, err
		}
		
		if data, exists := photoMap[photoID]; exists && !seenPhotos[photoID] {
			result = append(result, *data)
			seenPhotos[photoID] = true
		}
	}

	return result, rows.Err()
}

func (s *Server) selectBestVariantURL(variants []database.SizeVariant, isThumb bool) string {
	if len(variants) == 0 {
		return ""
	}

	var selectedVariant *database.SizeVariant

	if isThumb {
		// For thumbnails, prefer thumb (6) > medium (2) > original (0)
		for _, v := range variants {
			if v.Type == 6 { // Thumb
				selectedVariant = &v
				break
			}
		}
		if selectedVariant == nil {
			for _, v := range variants {
				if v.Type == database.SizeVariantMedium { // Medium
					selectedVariant = &v
					break
				}
			}
		}
		if selectedVariant == nil {
			for _, v := range variants {
				if v.Type == database.SizeVariantOriginal { // Original
					selectedVariant = &v
					break
				}
			}
		}
	} else {
		// For full size, prefer medium (2) > original (0)
		for _, v := range variants {
			if v.Type == database.SizeVariantMedium { // Medium
				selectedVariant = &v
				break
			}
		}
		if selectedVariant == nil {
			for _, v := range variants {
				if v.Type == database.SizeVariantOriginal { // Original
					selectedVariant = &v
					break
				}
			}
		}
	}

	if selectedVariant == nil {
		return ""
	}

	return s.imageFetcher.ConstructImageURL(selectedVariant)
}

func (s *Server) handleStatic(w http.ResponseWriter, r *http.Request) {
	// Add CORS headers for local development
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	
	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}
	
	// Serve the React app
	w.Header().Set("Content-Type", "text/html")
	http.ServeFile(w, r, "web/static/index.html")
}