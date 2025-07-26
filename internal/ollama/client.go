package ollama

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"lychee-ai-organizer/internal/config"
	"lychee-ai-organizer/internal/database"
	"lychee-ai-organizer/internal/images"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/avast/retry-go"
	"github.com/ollama/ollama/api"
)

type Client struct {
	client       *api.Client
	imageModel   string
	synthModel   string
	db           *database.DB
	imageFetcher *images.Fetcher
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
	}, nil
}

func (c *Client) GeneratePhotoDescription(photo *database.Photo) (string, error) {
	// Get the image variant for this photo
	variant, err := c.db.GetPhotoSizeVariant(photo.ID)
	if err != nil {
		return "", fmt.Errorf("failed to get image variant: %w", err)
	}

	// Fetch the image bytes
	imageBytes, _, err := c.imageFetcher.GetImageBytes(variant)
	if err != nil {
		return "", fmt.Errorf("failed to fetch image: %w", err)
	}

	prompt := fmt.Sprintf(`Analyze this photo and provide a concise description in maximum 3 sentences. Focus on:
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

func (c *Client) GenerateAlbumDescription(album *database.Album, photos []database.Photo) (string, error) {
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

	if len(photoDescriptions) == 0 {
		return "", fmt.Errorf("no photo descriptions available for album synthesis")
	}

	prompt := fmt.Sprintf(`Based on the following photo descriptions from an album, create a one-paragraph summary that captures the essence of this photo collection:

Photo descriptions:
%s

Date range: %s to %s

Provide a cohesive paragraph that synthesizes the common themes, subjects, and mood across these photos. Include information about the time period covered.

Provide only the summary paragraph, no additional text.`,
		strings.Join(photoDescriptions, "\n- "),
		getMinDate(dates),
		getMaxDate(dates))

	req := &api.GenerateRequest{
		Model:  c.synthModel,
		Prompt: prompt,
		Stream: &[]bool{false}[0],
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
		return "", fmt.Errorf("failed to generate album description after retries: %w", err)
	}

	generatedDescription := strings.TrimSpace(response.String())
	
	// Remove <think> tags and their contents
	generatedDescription = removeThinkTags(generatedDescription)
	
	// Append date range information
	if len(dates) > 0 {
		minDate := getMinDate(dates)
		maxDate := getMaxDate(dates)
		dateRangeText := fmt.Sprintf(" The album contains photos from dates %s to %s.", minDate, maxDate)
		generatedDescription += dateRangeText
	}

	return generatedDescription, nil
}

func (c *Client) GenerateAlbumSuggestions(photo *database.Photo, albums []database.Album) ([]string, error) {
	var albumDescs []string
	for _, album := range albums {
		if album.AIDescription.Valid {
			albumDescs = append(albumDescs, fmt.Sprintf("Album ID %s: %s", album.ID, album.AIDescription.String))
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
- Temporal relevance (how well the photo's date fits with other photos)

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

	req := &api.GenerateRequest{
		Model:  c.synthModel,
		Prompt: prompt,
		Stream: &[]bool{false}[0],
		Format: "json",
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