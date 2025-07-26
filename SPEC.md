Here is the updated specification for the AI-Powered Photo Organization Web Application, now including the detailed database schema.

## Specification: AI-Powered Photo Organization Web Application

This document outlines the specifications for a web application designed to assist users in organizing an existing photo library into albums using artificial intelligence.

### 1. Overview

The application will be a self-contained Golang binary with an embedded React single-page application. It will connect to a user-provided MySQL database of photos and leverage a local Ollama instance to generate descriptions for photos and albums. These descriptions will then be used to provide intelligent suggestions for organizing unsorted photos. The application is intended for local, personal use, and as such, authentication is not within the current scope.

### 2. Core Functionality

#### 2.1. AI-Powered Content Description

*   **Photo Description Generation:** For each photo in the database that lacks an AI-generated description, the application will use a user-configured Ollama endpoint to generate a concise description.
    *   The description will be a maximum of three sentences.
    *   It will focus on the photo's subject matter, photographic style, unique characteristics, and overall mood.
    *   The photo's capture date will be included in the description.
    *   **Image Processing:** The application fetches the actual image file from the Lychee installation using the `size_variants` table. It prioritizes Medium size variants (type 2) and falls back to Original variants (type 0) if Medium is unavailable. The image is downloaded from the Lychee base URL and sent as binary data to the vision model.
*   **Album Description Generation:** For each top-level photo album without an AI-generated description, the application will generate a one-paragraph summary.
    *   This summary will be synthesized from the descriptions of all the photos contained within that album.
    *   The summary will also include a synopsis of the capture dates of the photos in the album.
    *   After the generated description, the text "The album contains photos from dates X to Y." will be appended, where X and Y are calculated based on the `taken_at` field for each photo, or the `created_at` timestamp for photos where `taken_at` is null.
*   **Storage of Descriptions:** Generated descriptions for both photos and albums will be stored in the database in two nullable columns: `_ai_description` (TEXT) and `_ai_description_ts` (TIMESTAMP).
*   **Description Regeneration:** The application will not regenerate a description for a photo that already has one. Album descriptions will be regenerated for all top-level albums when the user clicks the "Rescan" button.

#### 2.2. Intelligent Photo Sorting

*   **Album Suggestions:** After all photos and top-level albums have descriptions, the application will generate album suggestions for each unsorted photo.
*   **Suggestion Mechanism:** To generate suggestions, the application will query the Ollama endpoint, providing it with the description of the unsorted photo, the photo's date (`taken_at` or `created_at` if `taken_at` is null), and the descriptions of all top-level albums. The prompt will instruct Ollama to consider thematic similarity, subject matter, context, and temporal relevance when suggesting the top three most likely albums for the photo.
*   **Suggestion Caching:** These album suggestions will be stored in a local cache file to avoid redundant generation on subsequent application runs. The path to this cache file will be configurable.

### 3. User Interface (UI)

The application will feature a single-page React UI with the following components:

*   **Unsorted Photo Filmstrip:** A horizontally scrolling filmstrip at the bottom of the screen will display thumbnails of all unsorted photos.
*   **Main Photo Display:** When a photo is selected from the filmstrip, it will be prominently displayed in the center of the screen.
*   **Album Suggestions:** Three large, prominent buttons will be displayed at the top of the screen, each representing a suggested album for the currently displayed photo.
*   **Sorting Action:** Clicking on an album suggestion will move the photo to that album. The photo will then be removed from the unsorted filmstrip, and the UI will automatically advance to the next unsorted photo.
*   **Navigation:** "Next" and "Previous" buttons will be positioned on the right and left sides of the main photo display, allowing the user to skip to the next or previous unsorted photo without sorting.
*   **Rescan Functionality:** A "Rescan" button will be located in the bottom-left corner of the UI. When clicked, the application will:
    *   Generate descriptions for any photos that do not have an AI-generated description
    *   Regenerate descriptions for all top-level albums (regardless of whether they already have descriptions)
*   **Real-time Progress Updates:** While the initial description generation is in progress, the UI will display a real-time updating list of the remaining work. This will be the only interactive element on the screen until the analysis is complete. This real-time communication will be handled using WebSockets.

### 4. Technical Specifications

#### 4.1. Backend (Golang)

*   **Single Binary:** The entire application will be compiled into a single, self-contained Golang binary.
*   **Embedded Web Assets:** The React single-page application's static assets (HTML, CSS, JavaScript) will be embedded within the Go binary using the `go:embed` directive.
*   **Database Connectivity:** The application will connect to an existing MySQL database using the `go-sql-driver/mysql` driver.
*   **Ollama Integration:** The application will interact with the user-provided Ollama API endpoint using the official `github.com/ollama/ollama/api` client library.
    *   **Retry Logic:** All Ollama API calls implement automatic retry logic using the `avast/retry-go` library with up to 3 attempts, starting with a 1-second delay and exponential backoff.
*   **Configuration:**
    *   Application configuration, including MySQL connection details, Ollama endpoint information (including the models for image analysis and description synthesis), and Lychee base URL, will be provided in a JSON file.
    *   The path to this configuration file will be specified using the `-config` command-line argument.
    *   The standard `encoding/json` package will be used for parsing the configuration file.
    *   **Lychee Integration:** The configuration must include the base URL of the Lychee installation for image fetching (e.g., "https://pictures.dzombak.com").
*   **Caching:**
    *   Album suggestions will be cached locally in a file specified by the `-cache` command-line argument.
    *   Standard file I/O operations will be used to manage the cache file.
*   **Web Server:** A lightweight web server will be implemented using Go's standard `net/http` package to serve the React application and handle API requests.
*   **Real-time Communication:** WebSockets will be used for real-time updates to the UI during the initial analysis phase. The `gorilla/websocket` library is a suitable choice for this.

#### 4.2. Frontend (React)

*   **Single-Page Application (SPA):** The UI will be a modern, responsive single-page application built with React.
*   **Component-Based Architecture:** The UI will be structured into reusable components for the photo strip, main photo display, album suggestions, and navigation controls.
*   **API Communication:** The frontend will communicate with the Golang backend via a RESTful API for actions such as fetching unsorted photos, moving photos to albums, and initiating a rescan.
*   **WebSocket Integration:** The frontend will establish a WebSocket connection with the backend to receive real-time updates on the progress of the initial AI analysis.

### 5. Database Schema

The application will interact with the following tables in the user's MySQL database.

#### 5.1. `albums` Table

This table stores information about the photo albums.

```sql
CREATE TABLE `albums` (
  `id` char(24) NOT NULL,
  `parent_id` char(24) DEFAULT NULL,
  `license` varchar(20) NOT NULL DEFAULT 'none',
  `album_thumb_aspect_ratio` varchar(6) DEFAULT NULL,
  `album_timeline` varchar(20) DEFAULT NULL,
  `album_sorting_col` varchar(30) DEFAULT NULL,
  `album_sorting_order` varchar(10) DEFAULT NULL,
  `cover_id` char(24) DEFAULT NULL,
  `header_id` char(24) DEFAULT NULL,
  `track_short_path` varchar(255) DEFAULT NULL,
  `_lft` bigint(20) unsigned NOT NULL DEFAULT 0,
  `_rgt` bigint(20) unsigned NOT NULL DEFAULT 0,
  PRIMARY KEY (`id`),
  KEY `albums__lft__rgt__index` (`_lft`,`_rgt` DESC),
  KEY `albums_parent_id_foreign` (`parent_id`),
  KEY `albums_cover_id_foreign` (`cover_id`),
  CONSTRAINT `albums_cover_id_foreign` FOREIGN KEY (`cover_id`) REFERENCES `photos` (`id`) ON DELETE SET NULL ON UPDATE CASCADE,
  CONSTRAINT `albums_id_foreign` FOREIGN KEY (`id`) REFERENCES `base_albums` (`id`),
  CONSTRAINT `albums_parent_id_foreign` FOREIGN KEY (`parent_id`) REFERENCES `albums` (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci ROW_FORMAT=DYNAMIC;
```

#### 5.2. `photos` Table

This table contains the details of each individual photo.

```sql
CREATE TABLE `photos` (
  `id` char(24) NOT NULL,
  `created_at` datetime(6) NOT NULL,
  `updated_at` datetime(6) NOT NULL,
  `owner_id` int(10) unsigned NOT NULL DEFAULT 0,
  `old_album_id` char(24) DEFAULT NULL,
  `title` varchar(100) NOT NULL,
  `description` text DEFAULT NULL,
  `tags` text DEFAULT NULL,
  `license` varchar(20) NOT NULL DEFAULT 'none',
  `is_starred` tinyint(1) NOT NULL DEFAULT 0,
  `iso` varchar(255) DEFAULT NULL,
  `make` varchar(255) DEFAULT NULL,
  `model` varchar(255) DEFAULT NULL,
  `lens` varchar(255) DEFAULT NULL,
  `aperture` varchar(255) DEFAULT NULL,
  `shutter` varchar(255) DEFAULT NULL,
  `focal` varchar(255) DEFAULT NULL,
  `latitude` decimal(10,8) DEFAULT NULL,
  `longitude` decimal(11,8) DEFAULT NULL,
  `altitude` decimal(10,4) DEFAULT NULL,
  `img_direction` decimal(10,4) DEFAULT NULL,
  `location` varchar(255) DEFAULT NULL,
  `taken_at` datetime(6) DEFAULT NULL COMMENT 'relative to UTC',
  `taken_at_orig_tz` varchar(31) DEFAULT NULL COMMENT 'the timezone at which the photo has originally been taken',
  `initial_taken_at` datetime DEFAULT NULL COMMENT 'backup of the original taken_at value',
  `initial_taken_at_orig_tz` varchar(31) DEFAULT NULL COMMENT 'backup of the timezone at which the photo has originally been taken',
  `type` varchar(30) NOT NULL,
  `filesize` bigint(20) unsigned NOT NULL DEFAULT 0,
  `checksum` varchar(40) NOT NULL,
  `original_checksum` varchar(40) NOT NULL,
  `live_photo_short_path` varchar(255) DEFAULT NULL,
  `live_photo_content_id` varchar(255) DEFAULT NULL,
  `live_photo_checksum` varchar(40) DEFAULT NULL,
  PRIMARY KEY (`id`),
  KEY `photos_owner_id_foreign` (`owner_id`),
  KEY `photos_checksum_index` (`checksum`),
  KEY `photos_original_checksum_index` (`original_checksum`),
  KEY `photos_live_photo_content_id_index` (`live_photo_content_id`),
  KEY `photos_live_photo_checksum_index` (`live_photo_checksum`),
  KEY `photos_album_id_taken_at_index` (`old_album_id`,`taken_at`),
  KEY `photos_album_id_created_at_index` (`old_album_id`,`created_at`),
  KEY `photos_album_id_is_starred_index` (`old_album_id`,`is_starred`),
  KEY `photos_album_id_type_index` (`old_album_id`,`type`),
  KEY `photos_album_id_is_starred_created_at_index` (`old_album_id`,`is_starred`,`created_at`),
  KEY `photos_album_id_is_starred_taken_at_index` (`old_album_id`,`is_starred`,`taken_at`),
  KEY `photos_album_id_is_starred_type_index` (`old_album_id`,`is_starred`,`type`),
  KEY `photos_album_id_is_starred_title_index` (`old_album_id`,`is_starred`,`title`),
  KEY `photos_album_id_is_starred_description(128)_index` (`old_album_id`,`is_starred`,`description`(128)),
  CONSTRAINT `photos_owner_id_foreign` FOREIGN KEY (`owner_id`) REFERENCES `users` (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci ROW_FORMAT=DYNAMIC;
```

#### 5.3. `photo_album` Table

This is a junction table that manages the many-to-many relationship between photos and albums.

```sql
CREATE TABLE `photo_album` (
  `album_id` char(24) NOT NULL,
  `photo_id` char(24) NOT NULL,
  PRIMARY KEY (`photo_id`,`album_id`),
  KEY `photo_album_album_id_photo_id_index` (`album_id`,`photo_id`),
  KEY `photo_album_album_id_index` (`album_id`),
  KEY `photo_album_photo_id_index` (`photo_id`),
  CONSTRAINT `photo_album_album_id_foreign` FOREIGN KEY (`album_id`) REFERENCES `albums` (`id`),
  CONSTRAINT `photo_album_photo_id_foreign` FOREIGN KEY (`photo_id`) REFERENCES `photos` (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci ROW_FORMAT=DYNAMIC;
```

#### 5.4. `size_variants` Table

This table stores multiple size variants for each photo, which the application uses to fetch image files for AI analysis.

```sql
CREATE TABLE `size_variants` (
  `id` bigint(20) unsigned NOT NULL AUTO_INCREMENT,
  `photo_id` char(24) NOT NULL,
  `type` int(10) unsigned NOT NULL DEFAULT 0 COMMENT '0: original, 2: medium, 6: thumb',
  `short_path` varchar(255) NOT NULL,
  `width` int(11) NOT NULL,
  `height` int(11) NOT NULL,
  `ratio` double NOT NULL DEFAULT 1,
  `filesize` bigint(20) unsigned NOT NULL DEFAULT 0,
  `storage_disk` varchar(255) NOT NULL DEFAULT 'images',
  PRIMARY KEY (`id`),
  UNIQUE KEY `size_variants_photo_id_type_unique` (`photo_id`,`type`),
  KEY `size_variants_short_path_index` (`short_path`),
  CONSTRAINT `size_variants_photo_id_foreign` FOREIGN KEY (`photo_id`) REFERENCES `photos` (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci ROW_FORMAT=DYNAMIC;
```

The application queries this table to find appropriate image variants for AI analysis, preferring Medium variants (type=2) over Original variants (type=0).

#### 5.5. Schema Modifications

To support the AI-powered features, the following columns must be added to the `albums` and `photos` tables. The application will not perform these migrations; they must be applied by the user beforehand.

**`albums` table modifications:**

```sql
ALTER TABLE `albums`
ADD COLUMN `_ai_description` TEXT DEFAULT NULL,
ADD COLUMN `_ai_description_ts` TIMESTAMP NULL DEFAULT NULL;
```

**`photos` table modifications:**

```sql
ALTER TABLE `photos`
ADD COLUMN `_ai_description` TEXT DEFAULT NULL,
ADD COLUMN `_ai_description_ts` TIMESTAMP NULL DEFAULT NULL;
```

### 6. Out of Scope

*   **Authentication and User Management:** The application is designed for single-user, local operation. No authentication mechanisms will be implemented.
*   **Photo and Album Creation/Deletion:** The application will only organize existing photos into existing albums. It will not provide functionality for creating, deleting, or editing photos or albums directly.
*   **Database Schema Management:** The application assumes the necessary columns (`_ai_description`, `_ai_description_ts`) have been added to the `photos` and `albums` tables. It will not perform any database migrations.
