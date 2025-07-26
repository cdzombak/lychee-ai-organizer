package database

import (
	"database/sql"
	"fmt"
	"lychee-ai-organizer/internal/config"

	_ "github.com/go-sql-driver/mysql"
)

type DB struct {
	conn *sql.DB
}

func NewDB(cfg *config.DatabaseConfig) (*DB, error) {
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?parseTime=true",
		cfg.Username, cfg.Password, cfg.Host, cfg.Port, cfg.Database)
	
	conn, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, err
	}

	if err := conn.Ping(); err != nil {
		return nil, err
	}

	return &DB{conn: conn}, nil
}

func (db *DB) Close() error {
	return db.conn.Close()
}

func (db *DB) GetUnsortedPhotos() ([]Photo, error) {
	query := `
		SELECT id, created_at, updated_at, owner_id, old_album_id, title, description, 
		       tags, license, is_starred, iso, make, model, lens, aperture, shutter, 
		       focal, latitude, longitude, altitude, img_direction, location, taken_at, 
		       taken_at_orig_tz, initial_taken_at, initial_taken_at_orig_tz, type, 
		       filesize, checksum, original_checksum, live_photo_short_path, 
		       live_photo_content_id, live_photo_checksum, _ai_description, _ai_description_ts
		FROM photos 
		WHERE id NOT IN (SELECT photo_id FROM photo_album)
		ORDER BY taken_at DESC, created_at DESC`

	rows, err := db.conn.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var photos []Photo
	for rows.Next() {
		var photo Photo
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
		)
		if err != nil {
			return nil, err
		}
		photos = append(photos, photo)
	}

	return photos, rows.Err()
}

func (db *DB) GetTopLevelAlbums() ([]Album, error) {
	query := `
		SELECT id, parent_id, license, album_thumb_aspect_ratio, album_timeline,
		       album_sorting_col, album_sorting_order, cover_id, header_id,
		       track_short_path, _lft, _rgt, _ai_description, _ai_description_ts
		FROM albums 
		WHERE parent_id IS NULL
		ORDER BY _lft`

	rows, err := db.conn.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var albums []Album
	for rows.Next() {
		var album Album
		err := rows.Scan(
			&album.ID, &album.ParentID, &album.License, &album.AlbumThumbAspectRatio,
			&album.AlbumTimeline, &album.AlbumSortingCol, &album.AlbumSortingOrder,
			&album.CoverID, &album.HeaderID, &album.TrackShortPath, &album.Lft,
			&album.Rgt, &album.AIDescription, &album.AIDescriptionTimestamp,
		)
		if err != nil {
			return nil, err
		}
		albums = append(albums, album)
	}

	return albums, rows.Err()
}

func (db *DB) GetPhotosWithoutAIDescription() ([]Photo, error) {
	query := `
		SELECT id, created_at, updated_at, owner_id, old_album_id, title, description, 
		       tags, license, is_starred, iso, make, model, lens, aperture, shutter, 
		       focal, latitude, longitude, altitude, img_direction, location, taken_at, 
		       taken_at_orig_tz, initial_taken_at, initial_taken_at_orig_tz, type, 
		       filesize, checksum, original_checksum, live_photo_short_path, 
		       live_photo_content_id, live_photo_checksum, _ai_description, _ai_description_ts
		FROM photos 
		WHERE _ai_description IS NULL
		ORDER BY taken_at DESC, created_at DESC`

	rows, err := db.conn.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var photos []Photo
	for rows.Next() {
		var photo Photo
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
		)
		if err != nil {
			return nil, err
		}
		photos = append(photos, photo)
	}

	return photos, rows.Err()
}

func (db *DB) GetAlbumsWithoutAIDescription() ([]Album, error) {
	query := `
		SELECT id, parent_id, license, album_thumb_aspect_ratio, album_timeline,
		       album_sorting_col, album_sorting_order, cover_id, header_id,
		       track_short_path, _lft, _rgt, _ai_description, _ai_description_ts
		FROM albums 
		WHERE parent_id IS NULL AND _ai_description IS NULL
		ORDER BY _lft`

	rows, err := db.conn.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var albums []Album
	for rows.Next() {
		var album Album
		err := rows.Scan(
			&album.ID, &album.ParentID, &album.License, &album.AlbumThumbAspectRatio,
			&album.AlbumTimeline, &album.AlbumSortingCol, &album.AlbumSortingOrder,
			&album.CoverID, &album.HeaderID, &album.TrackShortPath, &album.Lft,
			&album.Rgt, &album.AIDescription, &album.AIDescriptionTimestamp,
		)
		if err != nil {
			return nil, err
		}
		albums = append(albums, album)
	}

	return albums, rows.Err()
}

func (db *DB) UpdatePhotoAIDescription(photoID, description string) error {
	query := `UPDATE photos SET _ai_description = ?, _ai_description_ts = NOW() WHERE id = ?`
	_, err := db.conn.Exec(query, description, photoID)
	return err
}

func (db *DB) UpdateAlbumAIDescription(albumID, description string) error {
	query := `UPDATE albums SET _ai_description = ?, _ai_description_ts = NOW() WHERE id = ?`
	_, err := db.conn.Exec(query, description, albumID)
	return err
}

func (db *DB) GetPhotosInAlbum(albumID string) ([]Photo, error) {
	query := `
		SELECT p.id, p.created_at, p.updated_at, p.owner_id, p.old_album_id, p.title, p.description, 
		       p.tags, p.license, p.is_starred, p.iso, p.make, p.model, p.lens, p.aperture, p.shutter, 
		       p.focal, p.latitude, p.longitude, p.altitude, p.img_direction, p.location, p.taken_at, 
		       p.taken_at_orig_tz, p.initial_taken_at, p.initial_taken_at_orig_tz, p.type, 
		       p.filesize, p.checksum, p.original_checksum, p.live_photo_short_path, 
		       p.live_photo_content_id, p.live_photo_checksum, p._ai_description, p._ai_description_ts
		FROM photos p
		INNER JOIN photo_album pa ON p.id = pa.photo_id
		WHERE pa.album_id = ?
		ORDER BY p.taken_at DESC, p.created_at DESC`

	rows, err := db.conn.Query(query, albumID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var photos []Photo
	for rows.Next() {
		var photo Photo
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
		)
		if err != nil {
			return nil, err
		}
		photos = append(photos, photo)
	}

	return photos, rows.Err()
}

func (db *DB) MovePhotoToAlbum(photoID, albumID string) error {
	query := `INSERT INTO photo_album (album_id, photo_id) VALUES (?, ?) ON DUPLICATE KEY UPDATE album_id = ?`
	_, err := db.conn.Exec(query, albumID, photoID, albumID)
	return err
}

func (db *DB) GetPhotoSizeVariant(photoID string) (*SizeVariant, error) {
	// First try to get Medium variant (type 2), fallback to Original (type 0)
	query := `
		SELECT id, photo_id, type, short_path, width, height, ratio, filesize, storage_disk
		FROM size_variants 
		WHERE photo_id = ? AND type IN (?, ?)
		ORDER BY type ASC
		LIMIT 1`

	row := db.conn.QueryRow(query, photoID, SizeVariantMedium, SizeVariantOriginal)
	
	var variant SizeVariant
	err := row.Scan(
		&variant.ID, &variant.PhotoID, &variant.Type, &variant.ShortPath,
		&variant.Width, &variant.Height, &variant.Ratio, &variant.Filesize,
		&variant.StorageDisk,
	)
	
	if err != nil {
		return nil, err
	}
	
	return &variant, nil
}