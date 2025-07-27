# AI-Powered Photo Organization Web Application

This application helps organize an existing photo library into albums using artificial intelligence. It connects to a Lychee photo database and uses a local Ollama instance to generate descriptions for photos and albums, then provides intelligent suggestions for organizing unsorted photos.

## Features

- **AI-Powered Photo Analysis**: Generates concise descriptions for photos using Ollama vision models
- **Album Description Synthesis**: Creates summaries for albums based on contained photos
- **Intelligent Photo Sorting**: Suggests the best albums for unsorted photos
- **Real-time Progress Updates**: WebSocket-based progress tracking during AI analysis
- **Single Binary**: Self-contained Go binary with embedded React frontend
- **Suggestion Caching**: Avoids redundant AI generation across application runs
- **Album Blocklist**: Exclude specific albums from AI processing and suggestions

## Prerequisites

1. **MySQL Database**: A running Lychee photo database with the required schema modifications
2. **Ollama**: A local Ollama instance with vision and text models installed
3. **Go**: Go 1.21 or later for building the application

## Database Setup

Before running the application, you must add the required AI description columns to your existing Lychee database:

```sql
-- Add AI description columns to albums table
ALTER TABLE `albums`
ADD COLUMN `_ai_description` TEXT DEFAULT NULL,
ADD COLUMN `_ai_description_ts` TIMESTAMP NULL DEFAULT NULL;

-- Add AI description columns to photos table
ALTER TABLE `photos`
ADD COLUMN `_ai_description` TEXT DEFAULT NULL,
ADD COLUMN `_ai_description_ts` TIMESTAMP NULL DEFAULT NULL;
```

## Ollama Setup

1. Install and start Ollama
2. Pull the recommended models:
   ```bash
   ollama pull qwen2.5vl:3b      # For image analysis (recommended)
   ollama pull qwen3:8b          # For description synthesis (recommended)
   ```

## Configuration

1. Copy the example configuration file:
   ```bash
   cp config.example.json config.json
   ```

2. Edit `config.json` with your database and Ollama settings:
   ```json
   {
     "database": {
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
       "blocklist": ["album-id-1", "album-id-2"],
       "pinned_only": false
     }
   }
   ```

3. **Album Configuration (Optional)**: Configure album-related behavior:
   - **Blocklist**: Add album IDs to the `blocklist` array to exclude them from:
     - AI description generation (both albums and their photos)
     - Album suggestions for unsorted photos
     - Progress tracking during rescan operations
   - **Pinned Only**: Set `pinned_only` to `true` to restrict album suggestions to only pinned albums (`is_pinned = true` in the database). This helps focus suggestions on your most important albums.

### Ollama Configuration Options

The application supports configuring various Ollama parameters to optimize performance and handle large prompts.

#### Configuration Parameters

**Required Fields**
- `endpoint`: Ollama API endpoint URL
- `image_analysis_model`: Model for analyzing photos (recommended: "qwen2.5vl:3b")
- `description_synthesis_model`: Model for generating descriptions (recommended: "qwen3:8b")

**Optional Performance Parameters**
- `context_window`: Maximum context length (tokens). Default: model default
- `temperature`: Sampling temperature (0.0-1.0). Default: model default
- `top_p`: Top-p sampling (0.0-1.0). Default: model default
- `options`: Additional Ollama parameters as key-value pairs

#### Common Issues and Solutions

**Prompt Truncation**
If you see "truncating input prompt" in Ollama logs, increase the `context_window`:

```json
{
  "ollama": {
    "context_window": 32768
  }
}
```

**Large Album Processing**
For albums with many photos generating long prompts:
- Set `context_window` to 32768 or higher
- Consider reducing `temperature` for more consistent output

#### Example Configurations

**Basic Configuration (Recommended)**
```json
{
  "ollama": {
    "endpoint": "http://localhost:11434",
    "image_analysis_model": "qwen2.5vl:3b",
    "description_synthesis_model": "qwen3:8b",
    "context_window": 40960
  }
}
```

**Advanced Configuration**
```json
{
  "ollama": {
    "endpoint": "http://localhost:11434",
    "image_analysis_model": "llava:13b",
    "description_synthesis_model": "deepseek-r1:8b",
    "context_window": 32768,
    "temperature": 0.7,
    "top_p": 0.9,
    "options": {
      "num_predict": 512,
      "repeat_penalty": 1.1
    }
  }
}
```

#### Model-Specific Recommendations

**Qwen2.5VL:3b + Qwen3:8b (Recommended)**
- Best overall performance for photo organization tasks
- Excellent image understanding and description generation
- Context window: 40960 (recommended)
- Temperature: 0.7
- Good balance of speed and accuracy

**DeepSeek-R1**
- Context window: 32768 or higher (supports up to 131K)
- Temperature: 0.7
- Good for reasoning about album content

**Llama 3.1**
- Context window: 8192-32768
- Temperature: 0.8
- Balanced performance

**Qwen2**
- Context window: 32768 (supports up to 128K)
- Temperature: 0.7
- Excellent for multilingual content

## Building and Running

1. Build the application:
   ```bash
   go build -o lychee-ai-organizer
   ```

2. Run the application:
   ```bash
   ./lychee-ai-organizer -config config.json
   ```

3. Open your browser to `http://localhost:8080`

## Usage

### Initial Setup
1. When you first run the application, click the "Rescan" button to generate AI descriptions for all photos and top-level albums that don't have them yet
2. The progress will be shown in real-time via WebSocket updates
3. This process may take some time depending on the number of photos

### Organizing Photos
1. Unsorted photos appear in the filmstrip at the bottom
2. Select a photo to view it in the main display
3. Three album suggestions appear at the top based on AI analysis
4. Click an album suggestion to move the photo to that album
5. Use the navigation arrows or filmstrip to browse photos
6. The application automatically advances to the next photo after sorting

### Features
- **Photo Display**: Shows photo title, capture date, and AI-generated description
- **Navigation**: Previous/Next buttons and thumbnail filmstrip
- **Album Suggestions**: Three AI-generated suggestions per photo
- **Real-time Updates**: Progress tracking during rescan operations

## File Structure

```
├── main.go                    # Application entry point
├── app.go                     # Main application logic
├── go.mod                     # Go module dependencies
├── config.example.json        # Example configuration
├── internal/
│   ├── config/               # Configuration management
│   ├── database/             # Database models and operations
│   ├── ollama/              # Ollama API integration
│   ├── api/                 # REST API endpoints
│   ├── websocket/           # WebSocket handlers
└── web/
    └── static/
        └── index.html        # React frontend (embedded)
```

## API Endpoints

- `GET /api/photos/unsorted` - Get all unsorted photos
- `GET /api/photos/suggestions?photo_id=<id>` - Get album suggestions for a photo
- `POST /api/photos/move` - Move a photo to an album
- `POST /api/rescan` - Trigger rescan (also available via WebSocket)
- `WS /ws` - WebSocket for real-time updates

## Troubleshooting

### Common Issues

1. **Database Connection Failed**
   - Verify MySQL is running and credentials are correct
   - Ensure the database schema modifications have been applied

2. **Ollama Connection Failed**
   - Verify Ollama is running on the specified endpoint
   - Check that the required models are pulled and available

3. **No Photos Found**
   - Ensure photos exist in the database that are not already in albums
   - Check that the `photo_album` junction table accurately reflects current organization

4. **AI Descriptions Not Generating**
   - Check Ollama logs for errors
   - Verify the specified models are compatible with your Ollama version
   - Ensure sufficient system resources for AI model inference

### Performance Notes

- Image analysis is computationally intensive and may take time
- Consider using smaller models for faster processing on limited hardware

## Security Considerations

- This application is designed for local, personal use only
- No authentication is implemented
- Ensure your MySQL database is properly secured
- Do not expose the application to untrusted networks

## License

This project is provided as-is for personal use. See the specification document for more details about scope and limitations.