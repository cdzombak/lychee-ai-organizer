package cache

import (
	"encoding/json"
	"os"
)

type AlbumSuggestion struct {
	PhotoID     string   `json:"photo_id"`
	Suggestions []string `json:"suggestions"`
}

type Cache struct {
	filePath    string
	suggestions map[string][]string
}

func NewCache(filePath string) *Cache {
	return &Cache{
		filePath:    filePath,
		suggestions: make(map[string][]string),
	}
}

func (c *Cache) Load() error {
	data, err := os.ReadFile(c.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	var suggestions []AlbumSuggestion
	if err := json.Unmarshal(data, &suggestions); err != nil {
		return err
	}

	for _, s := range suggestions {
		c.suggestions[s.PhotoID] = s.Suggestions
	}

	return nil
}

func (c *Cache) Save() error {
	var suggestions []AlbumSuggestion
	for photoID, sug := range c.suggestions {
		suggestions = append(suggestions, AlbumSuggestion{
			PhotoID:     photoID,
			Suggestions: sug,
		})
	}

	data, err := json.Marshal(suggestions)
	if err != nil {
		return err
	}

	return os.WriteFile(c.filePath, data, 0644)
}

func (c *Cache) Get(photoID string) ([]string, bool) {
	suggestions, exists := c.suggestions[photoID]
	return suggestions, exists
}

func (c *Cache) Set(photoID string, suggestions []string) {
	c.suggestions[photoID] = suggestions
}