# AI-Powered Lychee Photo Organization Web Application

This application helps organize an existing [Lychee](https://github.com/LycheeOrg/Lychee) photo library into albums using artificial intelligence. It connects to a [Lychee](https://github.com/LycheeOrg/Lychee) photo database and uses a local [Ollama](https://ollama.com) instance to generate descriptions for photos and albums, then provides intelligent suggestions for organizing unsorted photos.

![screenshot](screenshot.png)

## Features

- **AI Photo Analysis**: Automatically generates descriptions for photos using vision models
- **Smart Album Suggestions**: Recommends the best albums for each unsorted photo
- **Album Intelligence**: Creates summaries for albums based on their content
- **Real-time Progress**: Live updates during AI processing operations
- **Single Binary**: Self-contained executable with embedded web interface
- **Album Management**: Blocklist and pinned-only filtering options

## Configuration

### Prerequisites

- **Database**: Running Lychee photo database (MySQL, PostgreSQL, or SQLite)
- **Ollama**: Local instance with required models
- **Go**: Version 1.21+ for building

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

## Running

1. **Build**:
   ```bash
   go build -o lychee-ai-organizer
   ```

2. **Run**:
   ```bash
   ./lychee-ai-organizer -config config.json
   ```

3. **Access**: Open `http://localhost:8080` in your browser

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
