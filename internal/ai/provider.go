package ai

import "lychee-ai-organizer/internal/database"

type Provider interface {
	GeneratePhotoDescription(photo *database.Photo) (string, error)

	GenerateAlbumDescription(album *database.Album, photos []database.Photo) (string, error)

	GenerateAlbumSuggestions(photo *database.Photo, albums []database.Album) ([]string, error)
}
