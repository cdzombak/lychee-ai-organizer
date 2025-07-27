package ollama

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"lychee-ai-organizer/internal/config"
	"lychee-ai-organizer/internal/database"
	"lychee-ai-organizer/internal/images"

	"github.com/avast/retry-go"
	"github.com/ollama/ollama/api"
)

type Client struct {
	client       *api.Client
	imageModel   string
	synthModel   string
	db           *database.DB
	imageFetcher *images.Fetcher
	config       *config.OllamaConfig
}

func NewClient(cfg *config.OllamaConfig, db *database.DB, imageFetcher *images.Fetcher) (*Client, error) {
	baseURL, err := url.Parse(cfg.Endpoint)
	if err != nil {
		return nil, fmt.Errorf("invalid Ollama endpoint URL: %w", err)
	}

	httpClient := &http.Client{}
	client := api.NewClient(baseURL, httpClient)

	return &Client{
		client:       client,
		imageModel:   cfg.ImageAnalysisModel,
		synthModel:   cfg.DescriptionSynthesisModel,
		db:           db,
		imageFetcher: imageFetcher,
		config:       cfg,
	}, nil
}

func (c *Client) GeneratePhotoDescription(photo *database.Photo) (string, error) {
	// Get the image variant for this photo first to check filename
	variant, err := c.db.GetPhotoSizeVariant(photo.ID)
	if err != nil {
		return "", fmt.Errorf("failed to get image variant: %w", err)
	}

	// Check if this is a movie file - if so, skip it
	if isMovieFile(photo, variant) {
		return "", fmt.Errorf("skipping movie file (type: %s, path: %s)", photo.Type, variant.ShortPath)
	}

	// Fetch the image bytes
	imageBytes, _, err := c.imageFetcher.GetImageBytes(variant)
	if err != nil {
		return "", fmt.Errorf("failed to fetch image: %w", err)
	}

	prompt := fmt.Sprintf(`Analyze this photo and provide a concise description in 2 sentences. Focus on:
- Subject matter and composition
- Photographic style and unique characteristics  
- Overall mood and atmosphere

Photo details:
- Title: %s
- Taken at: %s
- Camera: %s %s
- Location: %s

Provide only the description, no additional text.`,
		photo.Title,
		formatTakenAt(photo.TakenAt),
		getStringValue(photo.Make),
		getStringValue(photo.Model),
		getStringValue(photo.Location))

	req := &api.GenerateRequest{
		Model:  c.imageModel,
		Prompt: prompt,
		Stream: &[]bool{false}[0],
		Images: []api.ImageData{
			imageBytes,
		},
	}

	ctx := context.Background()
	var response strings.Builder

	err = retry.Do(
		func() error {
			response.Reset() // Clear previous attempts
			return c.client.Generate(ctx, req, func(resp api.GenerateResponse) error {
				response.WriteString(resp.Response)
				return nil
			})
		},
		retry.Attempts(3),
		retry.Delay(time.Second),
		retry.DelayType(retry.BackOffDelay),
	)
	if err != nil {
		return "", fmt.Errorf("failed to generate photo description after retries: %w", err)
	}

	description := strings.TrimSpace(response.String())

	// Remove <think> tags and their contents
	description = removeThinkTags(description)

	return description, nil
}

// buildOllamaOptions creates options map for Ollama API requests
func (c *Client) buildOllamaOptions() map[string]interface{} {
	options := make(map[string]interface{})

	// Set context window if specified
	if c.config.ContextWindow > 0 {
		options["num_ctx"] = c.config.ContextWindow
		log.Printf("Setting Ollama context window to %d", c.config.ContextWindow)
	}

	// Set temperature if specified
	if c.config.Temperature > 0 {
		options["temperature"] = c.config.Temperature
		log.Printf("Setting Ollama temperature to %f", c.config.Temperature)
	}

	// Set top_p if specified
	if c.config.TopP > 0 {
		options["top_p"] = c.config.TopP
		log.Printf("Setting Ollama top_p to %f", c.config.TopP)
	}

	// Add any additional options from config
	if c.config.Options != nil {
		for key, value := range c.config.Options {
			options[key] = value
			log.Printf("Setting custom Ollama option %s to %v", key, value)
		}
	}

	return options
}

func (c *Client) GenerateAlbumDescription(album *database.Album, photos []database.Photo) (string, error) {
	log.Printf("Starting album description generation for album %s (%s) with %d photos", album.ID, album.Title, len(photos))

	var photoDescriptions []string
	var dates []string

	log.Printf("Processing photos for album %s...", album.ID)
	for i, photo := range photos {
		log.Printf("Processing photo %d/%d (ID: %s) for album %s", i+1, len(photos), photo.ID, album.ID)

		if photo.AIDescription.Valid {
			photoDescriptions = append(photoDescriptions, photo.AIDescription.String)
			log.Printf("Added AI description for photo %s (length: %d chars)", photo.ID, len(photo.AIDescription.String))
		} else {
			log.Printf("Photo %s has no AI description, skipping", photo.ID)
		}

		// Use taken_at if available, otherwise fall back to created_at
		if photo.TakenAt.Valid {
			dates = append(dates, photo.TakenAt.Time.Format("2006-01-02"))
		} else {
			dates = append(dates, photo.CreatedAt.Format("2006-01-02"))
		}
	}

	log.Printf("Collected %d photo descriptions and %d dates for album %s", len(photoDescriptions), len(dates), album.ID)

	if len(photoDescriptions) == 0 {
		log.Printf("No photo descriptions available for album %s synthesis", album.ID)
		return "", fmt.Errorf("no photo descriptions available for album synthesis")
	}

	// Apply hierarchical compaction if we have more than 30 descriptions
	compactedDescriptions := photoDescriptions
	if len(photoDescriptions) > 30 {
		log.Printf("Album %s has %d descriptions, applying hierarchical compaction", album.ID, len(photoDescriptions))
		var err error
		compactedDescriptions, err = c.compactDescriptionsHierarchically(album.ID, photoDescriptions)
		if err != nil {
			log.Printf("Failed to compact descriptions for album %s: %v", album.ID, err)
			return "", fmt.Errorf("failed to compact descriptions: %w", err)
		}
		log.Printf("Compacted %d descriptions down to %d for album %s", len(photoDescriptions), len(compactedDescriptions), album.ID)
	}

	log.Printf("Creating prompt for album %s with %d descriptions", album.ID, len(compactedDescriptions))
	minDate := getMinDate(dates)
	maxDate := getMaxDate(dates)
	log.Printf("Date range for album %s: %s to %s", album.ID, minDate, maxDate)

	prompt := fmt.Sprintf(`Based on the following photo descriptions from an album, create a concise summary that captures the essence of this photo collection:

Photo descriptions:
%s

Date range: %s to %s

Provide a cohesive summary that synthesizes the common themes, subjects, and mood across these photos.

IMPORTANT: Keep your response to a maximum of 2 sentences. Be concise and focus on the most important aspects.

Provide only the summary, no additional text.`,
		strings.Join(compactedDescriptions, "\n- "),
		minDate,
		maxDate)

	log.Printf("Generated prompt for album %s (length: %d chars)", album.ID, len(prompt))

	log.Printf("Creating Ollama request for album %s using model %s", album.ID, c.synthModel)

	// Build options for the request
	options := c.buildOllamaOptions()
	log.Printf("Using Ollama options for album %s: %+v", album.ID, options)

	req := &api.GenerateRequest{
		Model:   c.synthModel,
		Prompt:  prompt,
		Stream:  &[]bool{false}[0],
		Options: options,
	}

	ctx := context.Background()
	var response strings.Builder

	log.Printf("Starting Ollama API call for album %s...", album.ID)
	err := retry.Do(
		func() error {
			log.Printf("Attempting Ollama API call for album %s", album.ID)
			response.Reset() // Clear previous attempts
			return c.client.Generate(ctx, req, func(resp api.GenerateResponse) error {
				log.Printf("Received response chunk for album %s (length: %d)", album.ID, len(resp.Response))
				response.WriteString(resp.Response)
				return nil
			})
		},
		retry.Attempts(3),
		retry.Delay(time.Second),
		retry.DelayType(retry.BackOffDelay),
	)
	if err != nil {
		log.Printf("Failed to generate album description for album %s after retries: %v", album.ID, err)
		return "", fmt.Errorf("failed to generate album description after retries: %w", err)
	}

	log.Printf("Successfully received response from Ollama for album %s", album.ID)
	generatedDescription := strings.TrimSpace(response.String())
	log.Printf("Raw response for album %s (length: %d chars): %s", album.ID, len(generatedDescription), generatedDescription)

	// Remove <think> tags and their contents
	log.Printf("Removing <think> tags from album %s description", album.ID)
	generatedDescription = removeThinkTags(generatedDescription)
	log.Printf("After removing <think> tags for album %s (length: %d chars)", album.ID, len(generatedDescription))

	// Append date range information
	if len(dates) > 0 {
		minDate := getMinDate(dates)
		maxDate := getMaxDate(dates)
		dateRangeText := fmt.Sprintf(" The album contains photos from dates %s to %s.", minDate, maxDate)
		generatedDescription += dateRangeText
		log.Printf("Appended date range to album %s description: %s", album.ID, dateRangeText)
	}

	log.Printf("Final description for album %s (length: %d chars): %s", album.ID, len(generatedDescription), generatedDescription)
	return generatedDescription, nil
}

func (c *Client) GenerateAlbumSuggestions(photo *database.Photo, albums []database.Album) ([]string, error) {
	var albumDescs []string
	for _, album := range albums {
		if album.AIDescription.Valid {
			albumDescs = append(albumDescs, fmt.Sprintf("Album ID %s: \"%s\": %s", album.ID, album.Title, album.AIDescription.String))
		}
	}

	if len(albumDescs) == 0 {
		return nil, fmt.Errorf("no album descriptions available for suggestions")
	}

	photoDesc := ""
	if photo.AIDescription.Valid {
		photoDesc = photo.AIDescription.String
	} else {
		return nil, fmt.Errorf("photo has no AI description")
	}

	// Get photo date (use taken_at if available, otherwise fall back to created_at)
	var photoDate string
	if photo.TakenAt.Valid {
		photoDate = photo.TakenAt.Time.Format("2006-01-02")
	} else {
		photoDate = photo.CreatedAt.Format("2006-01-02")
	}

	prompt := fmt.Sprintf(`Given this photo description:
%s

Photo date: %s

And these available albums:
%s

Analyze this photo and suggest the top 3 most appropriate albums for it. Consider:
- Thematic similarity (subject matter, content type)
- Contextual relevance (setting, event type, activity)
- Other clues (album title vs. photo subject, album date vs. photo date)

You must respond with valid JSON in exactly this format:
{
  "album_ids": ["AlbumID1", "AlbumID2", "AlbumID3"]
}

Rules:
- Use only Album IDs that appear in the available albums list above
- Return exactly 3 Album IDs in order of best match first
- Respond with only the JSON object, no other text
- The "album_ids" field must contain an array of strings`,
		photoDesc,
		photoDate,
		strings.Join(albumDescs, "\n"))

	log.Printf("Album suggestion prompt for photo %s:\n%s", photo.ID, prompt)

	// Build options for the request
	options := c.buildOllamaOptions()

	req := &api.GenerateRequest{
		Model:   c.synthModel,
		Prompt:  prompt,
		Stream:  &[]bool{false}[0],
		Format:  "json",
		Options: options,
	}

	ctx := context.Background()
	var response strings.Builder

	err := retry.Do(
		func() error {
			response.Reset() // Clear previous attempts
			return c.client.Generate(ctx, req, func(resp api.GenerateResponse) error {
				response.WriteString(resp.Response)
				return nil
			})
		},
		retry.Attempts(3),
		retry.Delay(time.Second),
		retry.DelayType(retry.BackOffDelay),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to generate album suggestions after retries: %w", err)
	}

	// Parse JSON response
	var jsonResponse struct {
		AlbumIDs []string `json:"album_ids"`
	}

	responseText := strings.TrimSpace(response.String())
	log.Printf("Album suggestion response for photo %s: %s", photo.ID, responseText)

	if err := json.Unmarshal([]byte(responseText), &jsonResponse); err != nil {
		return nil, fmt.Errorf("failed to parse JSON response: %w, response was: %s", err, responseText)
	}

	// Create a set of valid album IDs for validation
	validAlbumIDs := make(map[string]bool)
	for _, album := range albums {
		validAlbumIDs[album.ID] = true
	}

	// Filter and validate album IDs
	var suggestions []string
	for _, albumID := range jsonResponse.AlbumIDs {
		if validAlbumIDs[albumID] {
			suggestions = append(suggestions, albumID)
			if len(suggestions) >= 3 {
				break
			}
		}
	}

	return suggestions, nil
}

// removeThinkTags removes <think> tags and their contents from text
func removeThinkTags(text string) string {
	// Remove <think>...</think> blocks (including multiline)
	re := regexp.MustCompile(`(?s)<think>.*?</think>`)
	cleaned := re.ReplaceAllString(text, "")

	// Also remove standalone <think> tags without closing tags
	re2 := regexp.MustCompile(`<think>.*`)
	cleaned = re2.ReplaceAllString(cleaned, "")

	// Clean up extra whitespace
	cleaned = strings.TrimSpace(cleaned)

	return cleaned
}

func formatTakenAt(takenAt sql.NullTime) string {
	if !takenAt.Valid {
		return "Unknown"
	}
	return takenAt.Time.Format("2006-01-02 15:04:05")
}

func getStringValue(ns sql.NullString) string {
	if ns.Valid {
		return ns.String
	}
	return "Unknown"
}

func getMinDate(dates []string) string {
	if len(dates) == 0 {
		return "Unknown"
	}
	min := dates[0]
	for _, date := range dates[1:] {
		if date < min {
			min = date
		}
	}
	return min
}

func getMaxDate(dates []string) string {
	if len(dates) == 0 {
		return "Unknown"
	}
	max := dates[0]
	for _, date := range dates[1:] {
		if date > max {
			max = date
		}
	}
	return max
}

// isMovieFile checks if a photo is actually a movie file based on its type and filename
func isMovieFile(photo *database.Photo, variant *database.SizeVariant) bool {
	// Common movie file extensions
	movieExtensions := []string{
		".mp4", ".m4v", ".mov", ".avi", ".mkv", ".wmv", ".flv",
		".webm", ".ogv", ".3gp", ".m2v", ".mpg", ".mpeg", ".mts", ".m2ts",
	}

	// Check the photo type field (which should contain the file extension)
	photoType := strings.ToLower(photo.Type)
	for _, ext := range movieExtensions {
		if photoType == ext || photoType == strings.TrimPrefix(ext, ".") {
			return true
		}
	}

	// Also check the file extension from the variant's short_path
	if variant != nil {
		fileExt := strings.ToLower(filepath.Ext(variant.ShortPath))
		for _, ext := range movieExtensions {
			if fileExt == ext {
				return true
			}
		}
	}

	return false
}

// compactDescriptionsHierarchically applies recursive batch compression to reduce descriptions to 30 or fewer
func (c *Client) compactDescriptionsHierarchically(albumID string, descriptions []string) ([]string, error) {
	const batchSize = 30

	if len(descriptions) <= batchSize {
		return descriptions, nil
	}

	log.Printf("Starting hierarchical compaction for album %s with %d descriptions", albumID, len(descriptions))

	// Create batches of descriptions
	batches := make([][]string, 0)
	for i := 0; i < len(descriptions); i += batchSize {
		end := i + batchSize
		if end > len(descriptions) {
			end = len(descriptions)
		}
		batches = append(batches, descriptions[i:end])
	}

	log.Printf("Created %d batches of descriptions for album %s", len(batches), albumID)

	// Compress each batch
	compressedBatches := make([]string, 0, len(batches))
	for i, batch := range batches {
		log.Printf("Compressing batch %d/%d (%d descriptions) for album %s", i+1, len(batches), len(batch), albumID)

		compressed, err := c.compressBatchDescriptions(albumID, batch, i+1)
		if err != nil {
			return nil, fmt.Errorf("failed to compress batch %d: %w", i+1, err)
		}

		compressedBatches = append(compressedBatches, compressed)
		log.Printf("Successfully compressed batch %d for album %s (result length: %d chars)", i+1, albumID, len(compressed))
	}

	// If we still have too many compressed batches, recursively compress them
	if len(compressedBatches) > batchSize {
		log.Printf("Still have %d compressed batches for album %s, applying another level of compaction", len(compressedBatches), albumID)
		return c.compactDescriptionsHierarchically(albumID, compressedBatches)
	}

	log.Printf("Hierarchical compaction complete for album %s: %d -> %d descriptions", albumID, len(descriptions), len(compressedBatches))
	return compressedBatches, nil
}

// compressBatchDescriptions compresses a batch of descriptions into a single summary
func (c *Client) compressBatchDescriptions(albumID string, descriptions []string, batchNumber int) (string, error) {
	prompt := fmt.Sprintf(`Compress the following photo descriptions into a single, concise summary that captures the key themes, subjects, and characteristics across all photos:

Photo descriptions:
%s

Create a unified summary that:
- Identifies common subjects, themes, and visual elements
- Captures the overall mood and style
- Mentions key activities or events depicted
- Notes any significant compositional or photographic patterns

Keep the summary to 2-4 sentences maximum. Focus on what ties these photos together and their collective essence.

Provide only the summary, no additional text.`,
		strings.Join(descriptions, "\n- "))

	log.Printf("Compressing batch %d for album %s (prompt length: %d chars)", batchNumber, albumID, len(prompt))

	// Build options for the request
	options := c.buildOllamaOptions()

	req := &api.GenerateRequest{
		Model:   c.synthModel,
		Prompt:  prompt,
		Stream:  &[]bool{false}[0],
		Options: options,
	}

	ctx := context.Background()
	var response strings.Builder

	err := retry.Do(
		func() error {
			response.Reset()
			return c.client.Generate(ctx, req, func(resp api.GenerateResponse) error {
				response.WriteString(resp.Response)
				return nil
			})
		},
		retry.Attempts(3),
		retry.Delay(time.Second),
		retry.DelayType(retry.BackOffDelay),
	)
	if err != nil {
		return "", fmt.Errorf("failed to compress batch descriptions after retries: %w", err)
	}

	compressed := strings.TrimSpace(response.String())

	// Remove <think> tags and their contents
	compressed = removeThinkTags(compressed)

	log.Printf("Successfully compressed batch %d for album %s (result: %s)", batchNumber, albumID, compressed)
	return compressed, nil
}
