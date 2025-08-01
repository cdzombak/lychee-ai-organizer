package database

import (
	"database/sql"
	"fmt"
	"log"
	"lychee-ai-organizer/internal/config"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/lib/pq"
	_ "github.com/mattn/go-sqlite3"
)

type DB struct {
	conn *sql.DB
	dbType string
	blocklist map[string]bool
	pinnedOnly bool
}

func NewDB(cfg *config.DatabaseConfig, albumBlocklist []string, pinnedOnly bool) (*DB, error) {
	var dsn string
	var driverName string
	
	switch cfg.Type {
	case config.TypeMySQL:
		dsn = fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?parseTime=true",
			cfg.Username, cfg.Password, cfg.Host, cfg.Port, cfg.Database)
		driverName = "mysql"
	case config.TypePostgreSQL:
		dsn = fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=disable",
			cfg.Username, cfg.Password, cfg.Host, cfg.Port, cfg.Database)
		driverName = "postgres"
	case config.TypeSQLite:
		dsn = fmt.Sprintf("file:%s?cache=shared&mode=rwc", cfg.Database)
		driverName = "sqlite3"
	default:
		return nil, fmt.Errorf("unsupported database type: %s", cfg.Type)
	}
	
	conn, err := sql.Open(driverName, dsn)
	if err != nil {
		return nil, err
	}

	if err := conn.Ping(); err != nil {
		return nil, err
	}

	// Convert blocklist to map for faster lookups
	blocklist := make(map[string]bool)
	for _, albumID := range albumBlocklist {
		blocklist[albumID] = true
	}

	return &DB{conn: conn, dbType: cfg.Type, blocklist: blocklist, pinnedOnly: pinnedOnly}, nil
}

func (db *DB) Close() error {
	return db.conn.Close()
}

func (db *DB) GetDB() *sql.DB {
	return db.conn
}

func (db *DB) IsAlbumBlocked(albumID string) bool {
	return db.blocklist[albumID]
}

func (db *DB) buildBlocklistCondition() (string, []interface{}) {
	if len(db.blocklist) == 0 {
		return "", nil
	}
	
	placeholders := make([]string, 0, len(db.blocklist))
	args := make([]interface{}, 0, len(db.blocklist))
	
	for albumID := range db.blocklist {
		placeholders = append(placeholders, "?")
		args = append(args, albumID)
	}
	
	condition := fmt.Sprintf(" AND ba.id NOT IN (%s)", strings.Join(placeholders, ","))
	return condition, args
}

// scanPhoto scans a database row into a Photo struct
func scanPhoto(rows *sql.Rows) (*Photo, error) {
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
	return &photo, nil
}

// photoSelectColumns returns the standard photo columns for SELECT queries
func photoSelectColumns() string {
	return `id, created_at, updated_at, owner_id, old_album_id, title, description, 
	        tags, license, is_starred, iso, make, model, lens, aperture, shutter, 
	        focal, latitude, longitude, altitude, img_direction, location, taken_at, 
	        taken_at_orig_tz, initial_taken_at, initial_taken_at_orig_tz, type, 
	        filesize, checksum, original_checksum, live_photo_short_path, 
	        live_photo_content_id, live_photo_checksum, _ai_description, _ai_description_ts`
}

func (db *DB) GetUnsortedPhotos() ([]Photo, error) {
	query := fmt.Sprintf(`
		SELECT %s
		FROM photos 
		WHERE id NOT IN (SELECT photo_id FROM photo_album)
		ORDER BY taken_at DESC, created_at DESC`, photoSelectColumns())

	rows, err := db.conn.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var photos []Photo
	for rows.Next() {
		photo, err := scanPhoto(rows)
		if err != nil {
			return nil, err
		}
		photos = append(photos, *photo)
	}

	return photos, rows.Err()
}

func (db *DB) GetTopLevelAlbums() ([]Album, error) {
	blocklistCondition, blocklistArgs := db.buildBlocklistCondition()
	
	pinnedCondition := ""
	if db.pinnedOnly {
		pinnedCondition = " AND ba.is_pinned = 1"
	}
	
	query := `
		SELECT ba.id, ba.created_at, ba.updated_at, ba.published_at, ba.title, ba.description,
		       ba.owner_id, ba.is_nsfw, ba.is_pinned, ba.sorting_col, ba.sorting_order,
		       ba.copyright, ba.photo_layout, ba.photo_timeline, a.parent_id,
		       ba._ai_description, ba._ai_description_ts
		FROM base_albums ba
		LEFT JOIN albums a ON ba.id = a.id
		WHERE (a.parent_id IS NULL OR a.id IS NULL)` + blocklistCondition + pinnedCondition + `
		ORDER BY ba.title`

	rows, err := db.conn.Query(query, blocklistArgs...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var albums []Album
	for rows.Next() {
		var album Album
		err := rows.Scan(
			&album.ID, &album.CreatedAt, &album.UpdatedAt, &album.PublishedAt,
			&album.Title, &album.Description, &album.OwnerID, &album.IsNsfw,
			&album.IsPinned, &album.SortingCol, &album.SortingOrder,
			&album.Copyright, &album.PhotoLayout, &album.PhotoTimeline,
			&album.ParentID, &album.AIDescription, &album.AIDescriptionTimestamp,
		)
		if err != nil {
			return nil, err
		}
		albums = append(albums, album)
	}

	return albums, rows.Err()
}

func (db *DB) GetPhotosWithoutAIDescription() ([]Photo, error) {
	blocklistCondition := ""
	var blocklistArgs []interface{}
	
	if len(db.blocklist) > 0 {
		placeholders := make([]string, 0, len(db.blocklist))
		for albumID := range db.blocklist {
			placeholders = append(placeholders, "?")
			blocklistArgs = append(blocklistArgs, albumID)
		}
		blocklistCondition = fmt.Sprintf(" AND id NOT IN (SELECT photo_id FROM photo_album WHERE album_id IN (%s))", strings.Join(placeholders, ","))
	}
	
	query := fmt.Sprintf(`
		SELECT %s
		FROM photos 
		WHERE _ai_description IS NULL%s
		ORDER BY taken_at DESC, created_at DESC`, photoSelectColumns(), blocklistCondition)

	rows, err := db.conn.Query(query, blocklistArgs...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var photos []Photo
	for rows.Next() {
		photo, err := scanPhoto(rows)
		if err != nil {
			return nil, err
		}
		photos = append(photos, *photo)
	}

	return photos, rows.Err()
}

func (db *DB) GetAlbumsWithoutAIDescription() ([]Album, error) {
	blocklistCondition, blocklistArgs := db.buildBlocklistCondition()
	
	pinnedCondition := ""
	if db.pinnedOnly {
		pinnedCondition = " AND ba.is_pinned = 1"
	}
	
	query := `
		SELECT ba.id, ba.created_at, ba.updated_at, ba.published_at, ba.title, ba.description,
		       ba.owner_id, ba.is_nsfw, ba.is_pinned, ba.sorting_col, ba.sorting_order,
		       ba.copyright, ba.photo_layout, ba.photo_timeline, a.parent_id,
		       ba._ai_description, ba._ai_description_ts
		FROM base_albums ba
		LEFT JOIN albums a ON ba.id = a.id
		WHERE (a.parent_id IS NULL OR a.id IS NULL) AND ba._ai_description IS NULL` + blocklistCondition + pinnedCondition + `
		ORDER BY ba.title`

	rows, err := db.conn.Query(query, blocklistArgs...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var albums []Album
	for rows.Next() {
		var album Album
		err := rows.Scan(
			&album.ID, &album.CreatedAt, &album.UpdatedAt, &album.PublishedAt,
			&album.Title, &album.Description, &album.OwnerID, &album.IsNsfw,
			&album.IsPinned, &album.SortingCol, &album.SortingOrder,
			&album.Copyright, &album.PhotoLayout, &album.PhotoTimeline,
			&album.ParentID, &album.AIDescription, &album.AIDescriptionTimestamp,
		)
		if err != nil {
			return nil, err
		}
		albums = append(albums, album)
	}

	return albums, rows.Err()
}

func (db *DB) UpdatePhotoAIDescription(photoID, description string) error {
	query := `UPDATE photos SET _ai_description = ?, _ai_description_ts = ? WHERE id = ?`
	_, err := db.conn.Exec(query, description, time.Now(), photoID)
	return err
}

func (db *DB) UpdateAlbumAIDescription(albumID, description string) error {
	log.Printf("Updating AI description for album %s (description length: %d)", albumID, len(description))
	query := `UPDATE base_albums SET _ai_description = ?, _ai_description_ts = ? WHERE id = ?`
	
	log.Printf("Executing UPDATE query for album %s", albumID)
	result, err := db.conn.Exec(query, description, time.Now(), albumID)
	if err != nil {
		log.Printf("Failed to update album %s: %v", albumID, err)
		return err
	}
	
	rowsAffected, _ := result.RowsAffected()
	log.Printf("Successfully updated album %s (%d rows affected)", albumID, rowsAffected)
	return nil
}

func (db *DB) GetPhotosInAlbum(albumID string) ([]Photo, error) {
	query := fmt.Sprintf(`
		SELECT %s
		FROM photos p
		INNER JOIN photo_album pa ON p.id = pa.photo_id
		WHERE pa.album_id = ?
		ORDER BY p.taken_at DESC, p.created_at DESC`, 
		strings.ReplaceAll(photoSelectColumns(), "id,", "p.id,"))

	rows, err := db.conn.Query(query, albumID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var photos []Photo
	for rows.Next() {
		photo, err := scanPhoto(rows)
		if err != nil {
			return nil, err
		}
		photos = append(photos, *photo)
	}

	return photos, rows.Err()
}

func (db *DB) MovePhotoToAlbum(photoID, albumID string) error {
	switch db.dbType {
	case config.TypeMySQL:
		query := `INSERT INTO photo_album (album_id, photo_id) VALUES (?, ?) ON DUPLICATE KEY UPDATE album_id = ?`
		_, err := db.conn.Exec(query, albumID, photoID, albumID)
		return err
	case config.TypePostgreSQL:
		query := `INSERT INTO photo_album (album_id, photo_id) VALUES ($1, $2) ON CONFLICT (album_id, photo_id) DO UPDATE SET album_id = $1`
		_, err := db.conn.Exec(query, albumID, photoID)
		return err
	case config.TypeSQLite:
		query := `INSERT OR REPLACE INTO photo_album (album_id, photo_id) VALUES (?, ?)`
		_, err := db.conn.Exec(query, albumID, photoID)
		return err
	default:
		return fmt.Errorf("unsupported database type: %s", db.dbType)
	}
}

func (db *DB) GetAllPhotosWithoutAIDescription() ([]Photo, error) {
	blocklistCondition := ""
	blocklistExclude := ""
	var allArgs []interface{}
	
	if len(db.blocklist) > 0 {
		placeholders := make([]string, 0, len(db.blocklist))
		for albumID := range db.blocklist {
			placeholders = append(placeholders, "?")
			allArgs = append(allArgs, albumID)
		}
		blocklistCondition = fmt.Sprintf(" AND ba.id NOT IN (%s)", strings.Join(placeholders, ","))
		
		// Add second set of args for the second exclusion
		for albumID := range db.blocklist {
			allArgs = append(allArgs, albumID)
		}
		blocklistExclude = fmt.Sprintf(" AND id NOT IN (SELECT photo_id FROM photo_album WHERE album_id IN (%s))", strings.Join(placeholders, ","))
	}
	
	query := fmt.Sprintf(`
		SELECT %s
		FROM photos 
		WHERE _ai_description IS NULL AND (
			id NOT IN (SELECT photo_id FROM photo_album) OR 
			id IN (SELECT DISTINCT pa.photo_id FROM photo_album pa 
				   JOIN base_albums ba ON pa.album_id = ba.id 
				   LEFT JOIN albums a ON ba.id = a.id 
				   WHERE (a.parent_id IS NULL OR a.id IS NULL)%s)
		)%s
		ORDER BY taken_at DESC, created_at DESC`, photoSelectColumns(), blocklistCondition, blocklistExclude)

	rows, err := db.conn.Query(query, allArgs...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var photos []Photo
	for rows.Next() {
		photo, err := scanPhoto(rows)
		if err != nil {
			return nil, err
		}
		photos = append(photos, *photo)
	}

	return photos, rows.Err()
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
