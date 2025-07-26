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
	// Construct the full URL for the image
	// The short_path format varies by variant type:
	// - Medium/Small variants: "17/bc/ede2998bb6238e38debbede5dc6c.jpeg" 
	// - Original variants: "original/bc/76/cf76569f88279d64fc47b12a96db.jpg"
	
	var imageURL string
	if variant.Type == database.SizeVariantOriginal {
		// For original variants, strip "original/" prefix and use /uploads/original/
		shortPath := variant.ShortPath
		if strings.HasPrefix(shortPath, "original/") {
			shortPath = strings.TrimPrefix(shortPath, "original/")
		}
		imageURL = fmt.Sprintf("%s/uploads/original/%s", f.baseURL, shortPath)
	} else {
		// For other variants (medium, etc.), use the size directory
		var sizeDir string
		switch variant.Type {
		case database.SizeVariantMedium:
			sizeDir = "medium"
		default:
			sizeDir = "medium" // Default fallback
		}
		imageURL = fmt.Sprintf("%s/uploads/%s/%s", f.baseURL, sizeDir, variant.ShortPath)
	}
	
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
	// Construct the full URL for the image
	// The short_path format varies by variant type:
	// - Medium variants: "17/bc/ede2998bb6238e38debbede5dc6c.jpeg" 
	// - Thumbnail variants: "thumb/72/9f/6ac4c28108f1b276a8cc45e99141.jpeg"
	// - Original variants: "original/bc/76/cf76569f88279d64fc47b12a96db.jpg"
	
	if variant.Type == database.SizeVariantOriginal {
		// For original variants, strip "original/" prefix and use /uploads/original/
		shortPath := variant.ShortPath
		if strings.HasPrefix(shortPath, "original/") {
			shortPath = strings.TrimPrefix(shortPath, "original/")
		}
		return fmt.Sprintf("%s/uploads/original/%s", f.baseURL, shortPath)
	} else if variant.Type == 6 && strings.HasPrefix(variant.ShortPath, "thumb/") {
		// For thumbnails where short_path already includes "thumb/" prefix
		return fmt.Sprintf("%s/uploads/%s", f.baseURL, variant.ShortPath)
	} else if variant.Type == database.SizeVariantMedium && strings.HasPrefix(variant.ShortPath, "medium/") {
		// For medium variants where short_path already includes "medium/" prefix
		return fmt.Sprintf("%s/uploads/%s", f.baseURL, variant.ShortPath)
	} else {
		// For other variants, use the size directory
		var sizeDir string
		switch variant.Type {
		case database.SizeVariantMedium:
			sizeDir = "medium"
		case 6: // Thumbnail type
			sizeDir = "thumb"
		default:
			sizeDir = "medium" // Default fallback
		}
		return fmt.Sprintf("%s/uploads/%s/%s", f.baseURL, sizeDir, variant.ShortPath)
	}
}