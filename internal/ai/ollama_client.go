package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"lychee-ai-organizer/internal/config"
	"lychee-ai-organizer/internal/database"
	"lychee-ai-organizer/internal/images"

	"github.com/avast/retry-go"
	"github.com/ollama/ollama/api"
)

type OllamaClient struct {
	client       *api.Client
	imageModel   string
	synthModel   string
	db           *database.DB
	imageFetcher *images.Fetcher
	config       *config.OllamaConfig
}

func NewOllamaClient(cfg *config.OllamaConfig, db *database.DB, imageFetcher *images.Fetcher) (*OllamaClient, error) {
	baseURL, err := url.Parse(cfg.Endpoint)
	if err != nil {
		return nil, fmt.Errorf("invalid Ollama endpoint URL: %w", err)
	}

	httpClient := &http.Client{}
	client := api.NewClient(baseURL, httpClient)

	return &OllamaClient{
		client:       client,
		imageModel:   cfg.ImageAnalysisModel,
		synthModel:   cfg.DescriptionSynthesisModel,
		db:           db,
		imageFetcher: imageFetcher,
		config:       cfg,
	}, nil
}

func (c *OllamaClient) GeneratePhotoDescription(photo *database.Photo) (string, error) {
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
	description, err := c.generateWithRetry(ctx, req)
	if err != nil {
		return "", fmt.Errorf("failed to generate photo description after retries: %w", err)
	}

	// Remove <think> tags and their contents
	description = removeThinkTags(description)

	return description, nil
}

// buildOllamaOptions creates options map for Ollama API requests
func (c *OllamaClient) buildOllamaOptions() map[string]interface{} {
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

// generateWithRetry performs an Ollama API call with retry logic
func (c *OllamaClient) generateWithRetry(ctx context.Context, req *api.GenerateRequest) (string, error) {
	var response strings.Builder

	err := retry.Do(
		func() error {
			response.Reset() // Clear previous attempts
			return c.client.Generate(ctx, req, func(resp api.GenerateResponse) error {
				response.WriteString(resp.Response)
				return nil
			})
		},
		retry.Attempts(retryAttempts),
		retry.Delay(time.Second),
		retry.DelayType(retry.BackOffDelay),
	)
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(response.String()), nil
}

func (c *OllamaClient) GenerateAlbumDescription(album *database.Album, photos []database.Photo) (string, error) {
	log.Printf("Generating description for album %s (%s) with %d photos", album.ID, album.Title, len(photos))

	photoDescriptions, dates, err := extractPhotoData(photos)
	if err != nil {
		return "", err
	}

	if len(photoDescriptions) == 0 {
		return "", fmt.Errorf("no photo descriptions available for album synthesis")
	}

	// Apply hierarchical compaction if needed
	compactedDescriptions := photoDescriptions
	if len(photoDescriptions) > maxDescriptionsBeforeCompaction {
		log.Printf("Album %s has %d descriptions, applying compaction", album.ID, len(photoDescriptions))
		compactedDescriptions, err = c.compactDescriptionsHierarchically(album.ID, photoDescriptions)
		if err != nil {
			return "", fmt.Errorf("failed to compact descriptions: %w", err)
		}
		log.Printf("Compacted %d descriptions to %d for album %s", len(photoDescriptions), len(compactedDescriptions), album.ID)
	}

	prompt := buildAlbumDescriptionPrompt(compactedDescriptions, dates)
	req := &api.GenerateRequest{
		Model:   c.synthModel,
		Prompt:  prompt,
		Stream:  &[]bool{false}[0],
		Options: c.buildOllamaOptions(),
	}

	ctx := context.Background()
	generatedDescription, err := c.generateWithRetry(ctx, req)
	if err != nil {
		return "", fmt.Errorf("failed to generate album description after retries: %w", err)
	}

	// Remove <think> tags and their contents
	generatedDescription = removeThinkTags(generatedDescription)

	// Append date range information
	if len(dates) > 0 {
		minDate := getMinDate(dates)
		maxDate := getMaxDate(dates)
		dateRangeText := fmt.Sprintf(" The album contains photos from dates %s to %s.", minDate, maxDate)
		generatedDescription += dateRangeText
	}

	log.Printf("Generated description for album %s (length: %d chars)", album.ID, len(generatedDescription))
	return generatedDescription, nil
}

func (c *OllamaClient) GenerateAlbumSuggestions(photo *database.Photo, albums []database.Album) ([]string, error) {
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

	log.Printf("Generating album suggestions for photo %s", photo.ID)

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
	responseText, err := c.generateWithRetry(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to generate album suggestions after retries: %w", err)
	}

	// Parse JSON response
	var jsonResponse struct {
		AlbumIDs []string `json:"album_ids"`
	}

	if err := json.Unmarshal([]byte(responseText), &jsonResponse); err != nil {
		return nil, fmt.Errorf("failed to parse JSON response: %w, response was: %s", err, responseText)
	}

	log.Printf("Generated %d album suggestions for photo %s", len(jsonResponse.AlbumIDs), photo.ID)

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

// compactDescriptionsHierarchically applies recursive batch compression to reduce descriptions to manageable size
func (c *OllamaClient) compactDescriptionsHierarchically(albumID string, descriptions []string) ([]string, error) {
	if len(descriptions) <= maxDescriptionsBeforeCompaction {
		return descriptions, nil
	}

	log.Printf("Starting hierarchical compaction for album %s with %d descriptions", albumID, len(descriptions))

	// Create batches of descriptions
	batches := make([][]string, 0)
	for i := 0; i < len(descriptions); i += maxDescriptionsBeforeCompaction {
		end := i + maxDescriptionsBeforeCompaction
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
	if len(compressedBatches) > maxDescriptionsBeforeCompaction {
		log.Printf("Still have %d compressed batches for album %s, applying another level of compaction", len(compressedBatches), albumID)
		return c.compactDescriptionsHierarchically(albumID, compressedBatches)
	}

	log.Printf("Hierarchical compaction complete for album %s: %d -> %d descriptions", albumID, len(descriptions), len(compressedBatches))
	return compressedBatches, nil
}

// compressBatchDescriptions compresses a batch of descriptions into a single summary
func (c *OllamaClient) compressBatchDescriptions(albumID string, descriptions []string, batchNumber int) (string, error) {
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
	compressed, err := c.generateWithRetry(ctx, req)
	if err != nil {
		return "", fmt.Errorf("failed to compress batch descriptions after retries: %w", err)
	}

	// Remove <think> tags and their contents
	compressed = removeThinkTags(compressed)

	log.Printf("Successfully compressed batch %d for album %s (%d chars)", batchNumber, albumID, len(compressed))
	return compressed, nil
}
