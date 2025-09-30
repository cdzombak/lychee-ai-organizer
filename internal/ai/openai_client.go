package ai

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"lychee-ai-organizer/internal/config"
	"lychee-ai-organizer/internal/database"
	"lychee-ai-organizer/internal/images"

	"github.com/avast/retry-go"
)

const (
	// defaultBatchSize is the default threshold for applying hierarchical compaction
	defaultBatchSize = 30
	// retryAttempts is the number of times to retry failed API calls
	retryAttempts = 3
)

type OpenAIClient struct {
	endpoint     string
	apiKey       string
	imageModel   string
	synthModel   string
	db           *database.DB
	imageFetcher *images.Fetcher
	config       *config.AIConfig
}

func NewOpenAIClient(cfg *config.AIConfig, db *database.DB, imageFetcher *images.Fetcher) (*OpenAIClient, error) {
	return &OpenAIClient{
		endpoint:     strings.TrimSuffix(cfg.Endpoint, "/"),
		apiKey:       cfg.APIKey,
		imageModel:   cfg.ImageAnalysisModel,
		synthModel:   cfg.DescriptionSynthesisModel,
		db:           db,
		imageFetcher: imageFetcher,
		config:       cfg,
	}, nil
}

type openAIMessage struct {
	Role    string                   `json:"role"`
	Content interface{}              `json:"content"` // Can be string or array of content parts
}

type openAIContentPart struct {
	Type     string                `json:"type"`
	Text     string                `json:"text,omitempty"`
	ImageURL *openAIImageURL       `json:"image_url,omitempty"`
}

type openAIImageURL struct {
	URL string `json:"url"`
}

type openAIChatRequest struct {
	Model       string          `json:"model"`
	Messages    []openAIMessage `json:"messages"`
	MaxTokens   int             `json:"max_tokens,omitempty"`
	Temperature float64         `json:"temperature,omitempty"`
	TopP        float64         `json:"top_p,omitempty"`
}

type openAIChatResponse struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	Model   string `json:"model"`
	Choices []struct {
		Index   int `json:"index"`
		Message struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}

func (c *OpenAIClient) GeneratePhotoDescription(photo *database.Photo) (string, error) {
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

	base64Image := base64.StdEncoding.EncodeToString(imageBytes)
	imageDataURL := fmt.Sprintf("data:image/jpeg;base64,%s", base64Image)

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

	contentParts := []openAIContentPart{
		{
			Type: "text",
			Text: prompt,
		},
		{
			Type: "image_url",
			ImageURL: &openAIImageURL{
				URL: imageDataURL,
			},
		},
	}

	req := &openAIChatRequest{
		Model: c.imageModel,
		Messages: []openAIMessage{
			{
				Role:    "user",
				Content: contentParts,
			},
		},
	}

	if c.config.Temperature > 0 {
		req.Temperature = c.config.Temperature
	}
	if c.config.TopP > 0 {
		req.TopP = c.config.TopP
	}

	description, err := c.chatWithRetry(req)
	if err != nil {
		return "", fmt.Errorf("failed to generate photo description after retries: %w", err)
	}

	// Remove <think> tags and their contents
	description = removeThinkTags(description)

	return description, nil
}

func (c *OpenAIClient) GenerateAlbumDescription(album *database.Album, photos []database.Photo) (string, error) {
	log.Printf("Generating description for album %s (%s) with %d photos", album.ID, album.Title, len(photos))

	photoDescriptions, dates, err := extractPhotoData(photos)
	if err != nil {
		return "", err
	}

	if len(photoDescriptions) == 0 {
		return "", fmt.Errorf("no photo descriptions available for album synthesis")
	}

	// Apply hierarchical compaction if needed
	batchSize := c.getBatchSize()
	compactedDescriptions := photoDescriptions
	if len(photoDescriptions) > batchSize {
		log.Printf("Album %s has %d descriptions, applying compaction (batch size: %d)", album.ID, len(photoDescriptions), batchSize)
		compactedDescriptions, err = c.compactDescriptionsHierarchically(album.ID, photoDescriptions)
		if err != nil {
			return "", fmt.Errorf("failed to compact descriptions: %w", err)
		}
		log.Printf("Compacted %d descriptions to %d for album %s", len(photoDescriptions), len(compactedDescriptions), album.ID)
	}

	prompt := buildAlbumDescriptionPrompt(compactedDescriptions, dates)
	
	req := &openAIChatRequest{
		Model: c.synthModel,
		Messages: []openAIMessage{
			{
				Role:    "user",
				Content: prompt,
			},
		},
	}

	if c.config.Temperature > 0 {
		req.Temperature = c.config.Temperature
	}
	if c.config.TopP > 0 {
		req.TopP = c.config.TopP
	}

	generatedDescription, err := c.chatWithRetry(req)
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

func (c *OpenAIClient) GenerateAlbumSuggestions(photo *database.Photo, albums []database.Album) ([]string, error) {
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

	req := &openAIChatRequest{
		Model: c.synthModel,
		Messages: []openAIMessage{
			{
				Role:    "user",
				Content: prompt,
			},
		},
	}

	if c.config.Temperature > 0 {
		req.Temperature = c.config.Temperature
	}
	if c.config.TopP > 0 {
		req.TopP = c.config.TopP
	}

	responseText, err := c.chatWithRetry(req)
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

func (c *OpenAIClient) chatWithRetry(req *openAIChatRequest) (string, error) {
	var response string

	err := retry.Do(
		func() error {
			resp, err := c.sendChatRequest(req)
			if err != nil {
				return err
			}
			response = resp
			return nil
		},
		retry.Attempts(retryAttempts),
		retry.Delay(time.Second),
		retry.DelayType(retry.BackOffDelay),
	)
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(response), nil
}

func (c *OpenAIClient) sendChatRequest(req *openAIChatRequest) (string, error) {
	ctx := context.Background()

	reqBody, err := json.Marshal(req)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := createHTTPRequest(ctx, "POST", c.endpoint+"/v1/chat/completions", bytes.NewReader(reqBody), c.apiKey)
	if err != nil {
		return "", err
	}

	respBody, err := sendHTTPRequest(httpReq)
	if err != nil {
		return "", err
	}

	var chatResp openAIChatResponse
	if err := json.Unmarshal(respBody, &chatResp); err != nil {
		return "", fmt.Errorf("failed to unmarshal response: %w, body: %s", err, string(respBody))
	}

	log.Printf("OpenAI API response: choices=%d, model=%s, finish_reason=%s",
		len(chatResp.Choices),
		chatResp.Model,
		func() string {
			if len(chatResp.Choices) > 0 {
				return chatResp.Choices[0].FinishReason
			}
			return "N/A"
		}())

	if len(chatResp.Choices) == 0 {
		return "", fmt.Errorf("no choices in response")
	}

	content := chatResp.Choices[0].Message.Content
	if content == "" {
		return "", fmt.Errorf("empty content in response, finish_reason: %s, response body: %s",
			chatResp.Choices[0].FinishReason, string(respBody))
	}

	return content, nil
}

func (c *OpenAIClient) getBatchSize() int {
	if c.config.BatchSize > 0 {
		return c.config.BatchSize
	}
	return defaultBatchSize
}

func (c *OpenAIClient) compactDescriptionsHierarchically(albumID string, descriptions []string) ([]string, error) {
	batchSize := c.getBatchSize()

	if len(descriptions) <= batchSize {
		return descriptions, nil
	}

	log.Printf("Starting hierarchical compaction for album %s with %d descriptions (batch size: %d)", albumID, len(descriptions), batchSize)

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

func (c *OpenAIClient) compressBatchDescriptions(albumID string, descriptions []string, batchNumber int) (string, error) {
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

	req := &openAIChatRequest{
		Model: c.synthModel,
		Messages: []openAIMessage{
			{
				Role:    "user",
				Content: prompt,
			},
		},
	}

	if c.config.Temperature > 0 {
		req.Temperature = c.config.Temperature
	}
	if c.config.TopP > 0 {
		req.TopP = c.config.TopP
	}

	compressed, err := c.chatWithRetry(req)
	if err != nil {
		return "", fmt.Errorf("failed to compress batch descriptions after retries: %w", err)
	}

	// Remove <think> tags and their contents
	compressed = removeThinkTags(compressed)

	log.Printf("Successfully compressed batch %d for album %s (%d chars)", batchNumber, albumID, len(compressed))
	return compressed, nil
}


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

func extractPhotoData(photos []database.Photo) ([]string, []string, error) {
	var photoDescriptions []string
	var dates []string

	for _, photo := range photos {
		if photo.AIDescription.Valid {
			photoDescriptions = append(photoDescriptions, photo.AIDescription.String)
		}

		// Use taken_at if available, otherwise fall back to created_at
		if photo.TakenAt.Valid {
			dates = append(dates, photo.TakenAt.Time.Format("2006-01-02"))
		} else {
			dates = append(dates, photo.CreatedAt.Format("2006-01-02"))
		}
	}

	return photoDescriptions, dates, nil
}

func buildAlbumDescriptionPrompt(descriptions []string, dates []string) string {
	minDate := getMinDate(dates)
	maxDate := getMaxDate(dates)

	return fmt.Sprintf(`Based on the following photo descriptions from an album, create a concise summary that captures the essence of this photo collection:

Photo descriptions:
%s

Date range: %s to %s

Provide a cohesive summary that synthesizes the common themes, subjects, and mood across these photos.

IMPORTANT: Keep your response to a maximum of 2 sentences. Be concise and focus on the most important aspects.

Provide only the summary, no additional text.`,
		strings.Join(descriptions, "\n- "),
		minDate,
		maxDate)
}
