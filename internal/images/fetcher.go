package images

import (
	"encoding/base64"
	"fmt"
	"io"
	"log"
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
	imageURL := f.ConstructImageURL(variant)

	log.Printf("Fetching image from URL: %s (variant type: %d, short_path: %s)", imageURL, variant.Type, variant.ShortPath)

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

func (f *Fetcher) ConstructImageURL(variant *database.SizeVariant) string {
	// Construct the full URL for the image.
	// Each variant type is stored under its own subdirectory in /uploads/.
	variantDir := variantTypeDir(variant.Type)
	shortPath := strings.TrimPrefix(variant.ShortPath, variantDir+"/")
	return fmt.Sprintf("%s/uploads/%s/%s", f.baseURL, variantDir, shortPath)
}

// variantTypeDir returns the upload subdirectory name for a given size variant type.
func variantTypeDir(variantType int) string {
	switch variantType {
	case database.SizeVariantRaw:
		return "raw"
	case database.SizeVariantOriginal:
		return "original"
	case database.SizeVariantMedium2x:
		return "medium2x"
	case database.SizeVariantMedium:
		return "medium"
	case database.SizeVariantSmall2x:
		return "small2x"
	case database.SizeVariantSmall:
		return "small"
	case database.SizeVariantThumb2x:
		return "thumb2x"
	case database.SizeVariantThumb:
		return "thumb"
	case database.SizeVariantPlaceholder:
		return "placeholder"
	default:
		return "medium"
	}
}
