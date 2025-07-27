# Ollama Configuration Options

The application supports configuring various Ollama parameters to optimize performance and handle large prompts.

## Configuration Parameters

### Required Fields
- `endpoint`: Ollama API endpoint URL
- `image_analysis_model`: Model for analyzing photos (e.g., "llava:7b")
- `description_synthesis_model`: Model for generating descriptions (e.g., "deepseek-r1:8b")

### Optional Performance Parameters
- `context_window`: Maximum context length (tokens). Default: model default
- `temperature`: Sampling temperature (0.0-1.0). Default: model default
- `top_p`: Top-p sampling (0.0-1.0). Default: model default
- `options`: Additional Ollama parameters as key-value pairs

## Common Issues and Solutions

### Prompt Truncation
If you see "truncating input prompt" in Ollama logs, increase the `context_window`:

```json
{
  "ollama": {
    "context_window": 32768
  }
}
```

### Large Album Processing
For albums with many photos generating long prompts:
- Set `context_window` to 32768 or higher
- Consider reducing `temperature` for more consistent output

## Example Configurations

### Basic Configuration
```json
{
  "ollama": {
    "endpoint": "http://localhost:11434",
    "image_analysis_model": "llava:7b",
    "description_synthesis_model": "deepseek-r1:8b",
    "context_window": 8192
  }
}
```

### Advanced Configuration
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

## Model-Specific Recommendations

### DeepSeek-R1
- Context window: 32768 or higher (supports up to 131K)
- Temperature: 0.7
- Good for reasoning about album content

### Llama 3.1
- Context window: 8192-32768
- Temperature: 0.8
- Balanced performance

### Qwen2
- Context window: 32768 (supports up to 128K)
- Temperature: 0.7
- Excellent for multilingual content