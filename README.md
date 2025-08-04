# AI-Powered Lychee Photo Organization Web Application

This application helps organize an existing [Lychee](https://github.com/LycheeOrg/Lychee) photo library into albums using artificial intelligence. It connects to a [Lychee](https://github.com/LycheeOrg/Lychee) photo database and uses a local [Ollama](https://ollama.com) instance to generate descriptions for photos and albums, then provides intelligent suggestions for organizing unsorted photos.

![screenshot](screenshot.png)

## Configuration

### Prerequisites

- **Database**: Running Lychee photo database (MySQL, PostgreSQL, or SQLite)
- **Ollama**: Local instance with required models

### Database Setup

Add AI description columns to your Lychee database:

#### MySQL
```sql
-- Add AI description columns to base_albums table
ALTER TABLE `base_albums`
ADD COLUMN `_ai_description` TEXT DEFAULT NULL,
ADD COLUMN `_ai_description_ts` TIMESTAMP NULL DEFAULT NULL;

-- Add AI description columns to photos table
ALTER TABLE `photos`
ADD COLUMN `_ai_description` TEXT DEFAULT NULL,
ADD COLUMN `_ai_description_ts` TIMESTAMP NULL DEFAULT NULL;
```

#### PostgreSQL
```sql
-- Add AI description columns to base_albums table
ALTER TABLE base_albums
ADD COLUMN _ai_description TEXT DEFAULT NULL,
ADD COLUMN _ai_description_ts TIMESTAMP DEFAULT NULL;

-- Add AI description columns to photos table
ALTER TABLE photos
ADD COLUMN _ai_description TEXT DEFAULT NULL,
ADD COLUMN _ai_description_ts TIMESTAMP DEFAULT NULL;
```

#### SQLite
```sql
-- Add AI description columns to base_albums table
ALTER TABLE base_albums
ADD COLUMN _ai_description TEXT DEFAULT NULL;

ALTER TABLE base_albums
ADD COLUMN _ai_description_ts DATETIME DEFAULT NULL;

-- Add AI description columns to photos table
ALTER TABLE photos
ADD COLUMN _ai_description TEXT DEFAULT NULL;

ALTER TABLE photos
ADD COLUMN _ai_description_ts DATETIME DEFAULT NULL;
```

### Ollama Setup

Install Ollama and pull the recommended models:

```bash
ollama pull qwen2.5vl:3b      # For image analysis
ollama pull qwen3:8b          # For description synthesis
```

### Application Configuration

1. Copy the example configuration:
   ```bash
   cp config.example.json config.json
   ```

2. Edit `config.json`:

   **MySQL Configuration:**
   ```json
   {
     "database": {
       "type": "mysql",
       "host": "localhost",
       "port": 3306,
       "username": "your_db_user",
       "password": "your_db_password",
       "database": "lychee"
     },
     "ollama": {
       "endpoint": "http://localhost:11434",
       "image_analysis_model": "qwen2.5vl:3b",
       "description_synthesis_model": "qwen3:8b",
       "context_window": 40960
     },
     "server": {
       "host": "localhost",
       "port": 8080
     },
     "albums": {
       "blocklist": [],
       "pinned_only": false
     }
   }
   ```

   **PostgreSQL Configuration:**
   ```json
   {
     "database": {
       "type": "postgresql",
       "host": "localhost",
       "port": 5432,
       "username": "your_db_user",
       "password": "your_db_password",
       "database": "lychee"
     },
     "ollama": {
       "endpoint": "http://localhost:11434",
       "image_analysis_model": "qwen2.5vl:3b",
       "description_synthesis_model": "qwen3:8b",
       "context_window": 40960
     },
     "server": {
       "host": "localhost",
       "port": 8080
     },
     "albums": {
       "blocklist": [],
       "pinned_only": false
     }
   }
   ```

   **SQLite Configuration:**
   ```json
   {
     "database": {
       "type": "sqlite",
       "database": "/path/to/lychee.db"
     },
     "ollama": {
       "endpoint": "http://localhost:11434",
       "image_analysis_model": "qwen2.5vl:3b",
       "description_synthesis_model": "qwen3:8b",
       "context_window": 40960
     },
     "server": {
       "host": "localhost",
       "port": 8080
     },
     "albums": {
       "blocklist": [],
       "pinned_only": false
     }
   }
   ```

#### Album Options

- **Blocklist**: Exclude specific album IDs from AI processing and suggestions
- **Pinned Only**: Restrict suggestions to pinned albums only (`is_pinned = true`)

#### Ollama Performance Options

- `context_window`: Maximum context length (recommended for `qwen3:8b`: 40960)
- `temperature`: Sampling temperature (0.0-1.0)
- `top_p`: Top-p sampling (0.0-1.0)
- `options`: Additional Ollama parameters

## Installation

### macOS via Homebrew

```shell
brew install cdzombak/oss/lychee-ai-organizer
```

### Debian via apt repository

[Install my Debian repository](https://www.dzombak.com/blog/2025/06/updated-instructions-for-installing-my-debian-package-repositories/) if you haven't already:

```shell
sudo mkdir -p /etc/apt/keyrings
curl -fsSL https://dist.cdzombak.net/keys/dist-cdzombak-net.gpg -o /etc/apt/keyrings/dist-cdzombak-net.gpg
sudo chmod 644 /etc/apt/keyrings/dist-cdzombak-net.gpg
sudo mkdir -p /etc/apt/sources.list.d
sudo curl -fsSL https://dist.cdzombak.net/cdzombak-oss.sources -o /etc/apt/sources.list.d/cdzombak-oss.sources
sudo chmod 644 /etc/apt/sources.list.d/cdzombak-oss.sources
sudo apt update
```

Then install `lychee-ai-organizer` via `apt-get`:

```shell
sudo apt-get install lychee-ai-organizer
```

### Manual installation from build artifacts

Pre-built binaries for Linux and macOS on various architectures are downloadable from each [GitHub Release](https://github.com/cdzombak/lychee-ai-organizer/releases). Debian packages for each release are available as well.

### Build and install locally

```shell
git clone https://github.com/cdzombak/lychee-ai-organizer.git
cd lychee-ai-organizer
make build

cp out/lychee-ai-organizer $INSTALL_DIR
```

## Running

1. **Run** (if installed via package manager):
   ```bash
   lychee-ai-organizer -config config.json
   ```

2. **Run** (if built locally):
   ```bash
   ./out/lychee-ai-organizer -config config.json
   ```

3. **Access**: Open `http://localhost:8080` in your browser

### Docker images

Docker images are available for a variety of Linux architectures from [Docker Hub](https://hub.docker.com/r/cdzombak/lychee-ai-organizer) and [GHCR](https://github.com/cdzombak/lychee-ai-organizer/pkgs/container/lychee-ai-organizer). Images are based on the `scratch` image and are as small as possible.

Run them via, for example:

```shell
docker run --rm -v /path/to/config.json:/config.json cdzombak/lychee-ai-organizer:1 -config /config.json
docker run --rm -v /path/to/config.json:/config.json ghcr.io/cdzombak/lychee-ai-organizer:1 -config /config.json
```

## Usage

### Initial Setup

1. **Generate Photo Descriptions**: Click "Describe Photos" to analyze all unsorted photos
2. **Generate Album Descriptions**: Click "Describe All Albums" to create album summaries
3. **Monitor Progress**: Real-time updates show processing status

**Important**: Always run "Describe Photos" first, then "Describe All Albums" for optimal results.

### Organizing Photos

1. **View Photos**: Unsorted photos appear in the bottom filmstrip
2. **Navigate**: Click thumbnails or use arrow keys to browse photos
3. **Get Suggestions**: Three AI-recommended albums appear at the top
4. **Organize**: Click an album button to move the photo
5. **Continue**: The interface automatically advances to the next photo

### Additional Operations

- **Retry Album Failures**: Reprocess any albums that failed during description generation
- **Navigation**: Use Previous/Next buttons or arrow keys
- **Photo Info**: View title, date, and AI-generated description for each photo

## Troubleshooting

### Common Issues

**Database Connection Failed**
- Verify database credentials and connectivity
- Ensure schema modifications are applied
- For SQLite: Ensure the database file path is correct and writable
- For PostgreSQL: Ensure the database exists and user has proper permissions

**Ollama Connection Failed**
- Check Ollama is running on specified endpoint
- Verify models are pulled and available

**No Photos Found**
- Ensure unsorted photos exist in database
- Check `photo_album` table reflects current organization

**Prompt Truncation**
- Increase `context_window` to 32768 or higher
- Monitor Ollama logs for truncation warnings

### Performance

- Image analysis requires significant computational resources
- Processing time scales with photo count and model size
- Consider using smaller models on limited hardware

## API Reference

- `GET /api/photos/unsorted` - List unsorted photos
- `GET /api/photos/suggestions?photo_id=<id>` - Get album suggestions
- `POST /api/photos/move` - Move photo to album
- `POST /api/rescan` - Trigger AI processing
- `WS /ws` - WebSocket for real-time updates

## Security

- Designed for local, personal use only
- No authentication implemented
- Secure your MySQL database appropriately
- Do not expose to untrusted networks

## License

GNU General Public License v3.0; see [LICENSE](LICENSE) in this repository.

## Author

[Claude Code](https://www.anthropic.com/claude-code) wrote this code with management by Chris Dzombak ([dzombak.com](https://www.dzombak.com) / [github.com/cdzombak](https://www.github.com/cdzombak)).
