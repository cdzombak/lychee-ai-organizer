package database

import (
	"database/sql"
	"time"
)

type Photo struct {
	ID                     string         `db:"id"`
	CreatedAt              time.Time      `db:"created_at"`
	UpdatedAt              time.Time      `db:"updated_at"`
	OwnerID                int            `db:"owner_id"`
	OldAlbumID             sql.NullString `db:"old_album_id"`
	Title                  string         `db:"title"`
	Description            sql.NullString `db:"description"`
	Tags                   sql.NullString `db:"tags"`
	License                string         `db:"license"`
	IsStarred              bool           `db:"is_starred"`
	ISO                    sql.NullString `db:"iso"`
	Make                   sql.NullString `db:"make"`
	Model                  sql.NullString `db:"model"`
	Lens                   sql.NullString `db:"lens"`
	Aperture               sql.NullString `db:"aperture"`
	Shutter                sql.NullString `db:"shutter"`
	Focal                  sql.NullString `db:"focal"`
	Latitude               sql.NullFloat64 `db:"latitude"`
	Longitude              sql.NullFloat64 `db:"longitude"`
	Altitude               sql.NullFloat64 `db:"altitude"`
	ImgDirection           sql.NullFloat64 `db:"img_direction"`
	Location               sql.NullString `db:"location"`
	TakenAt                sql.NullTime   `db:"taken_at"`
	TakenAtOrigTz          sql.NullString `db:"taken_at_orig_tz"`
	InitialTakenAt         sql.NullTime   `db:"initial_taken_at"`
	InitialTakenAtOrigTz   sql.NullString `db:"initial_taken_at_orig_tz"`
	Type                   string         `db:"type"`
	Filesize               int64          `db:"filesize"`
	Checksum               string         `db:"checksum"`
	OriginalChecksum       string         `db:"original_checksum"`
	LivePhotoShortPath     sql.NullString `db:"live_photo_short_path"`
	LivePhotoContentID     sql.NullString `db:"live_photo_content_id"`
	LivePhotoChecksum      sql.NullString `db:"live_photo_checksum"`
	AIDescription          sql.NullString `db:"_ai_description"`
	AIDescriptionTimestamp sql.NullTime   `db:"_ai_description_ts"`
}

type Album struct {
	ID                     string         `db:"id"`
	Title                  string         `db:"title"`
	ParentID               sql.NullString `db:"parent_id"`
	License                string         `db:"license"`
	AlbumThumbAspectRatio  sql.NullString `db:"album_thumb_aspect_ratio"`
	AlbumTimeline          sql.NullString `db:"album_timeline"`
	AlbumSortingCol        sql.NullString `db:"album_sorting_col"`
	AlbumSortingOrder      sql.NullString `db:"album_sorting_order"`
	CoverID                sql.NullString `db:"cover_id"`
	HeaderID               sql.NullString `db:"header_id"`
	TrackShortPath         sql.NullString `db:"track_short_path"`
	Lft                    int64          `db:"_lft"`
	Rgt                    int64          `db:"_rgt"`
	AIDescription          sql.NullString `db:"_ai_description"`
	AIDescriptionTimestamp sql.NullTime   `db:"_ai_description_ts"`
}

type PhotoAlbum struct {
	AlbumID string `db:"album_id"`
	PhotoID string `db:"photo_id"`
}

type SizeVariant struct {
	ID          int64  `db:"id"`
	PhotoID     string `db:"photo_id"`
	Type        int    `db:"type"` // 0: original, ..., 6: thumb
	ShortPath   string `db:"short_path"`
	Width       int    `db:"width"`
	Height      int    `db:"height"`
	Ratio       float64 `db:"ratio"`
	Filesize    int64  `db:"filesize"`
	StorageDisk string `db:"storage_disk"`
}

const (
	SizeVariantOriginal = 0
	SizeVariantMedium   = 2 // Assuming medium is type 2 based on common Lychee setup
)