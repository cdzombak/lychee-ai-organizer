package images

import (
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"path"
	"strings"

	"lychee-ai-organizer/internal/config"
	"lychee-ai-organizer/internal/database"
)

type Fetcher struct {
	baseURL string
	client  *http.Client
}

func NewFetcher(cfg *config.LycheeConfig) *Fetcher {
	return &Fetcher{
		baseURL: strings.TrimSuffix(cfg.BaseURL, "/"),
		client:  &http.Client{},
	}
}

func (f *Fetcher) GetImageBytes(variant *database.SizeVariant) ([]byte, string, error) {
	// Construct the full URL for the image
	// Based on the example: "https://pictures.dzombak.com/uploads/medium/17/bc/ede2998bb6238e38debbede5dc6c.jpeg"
	var sizeDir string
	switch variant.Type {
	case database.SizeVariantOriginal:
		sizeDir = "big"  // Original images are typically in "big" directory
	case database.SizeVariantMedium:
		sizeDir = "medium"
	default:
		sizeDir = "big"  // Fallback to original
	}
	
	imageURL := fmt.Sprintf("%s/uploads/%s/%s", f.baseURL, sizeDir, variant.ShortPath)
	
	// Fetch the image
	resp, err := f.client.Get(imageURL)
	if err != nil {
		return nil, "", fmt.Errorf("failed to fetch image from %s: %w", imageURL, err)
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("failed to fetch image from %s: status %d", imageURL, resp.StatusCode)
	}
	
	// Read the image data
	imageData, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", fmt.Errorf("failed to read image data: %w", err)
	}
	
	// Determine MIME type from file extension
	ext := strings.ToLower(path.Ext(variant.ShortPath))
	var mimeType string
	switch ext {
	case ".jpg", ".jpeg":
		mimeType = "image/jpeg"
	case ".png":
		mimeType = "image/png"
	case ".gif":
		mimeType = "image/gif"
	case ".webp":
		mimeType = "image/webp"
	default:
		mimeType = "image/jpeg" // Default fallback
	}
	
	return imageData, mimeType, nil
}

func (f *Fetcher) GetImageBase64(variant *database.SizeVariant) (string, string, error) {
	imageData, mimeType, err := f.GetImageBytes(variant)
	if err != nil {
		return "", "", err
	}
	
	base64Data := base64.StdEncoding.EncodeToString(imageData)
	return base64Data, mimeType, nil
}