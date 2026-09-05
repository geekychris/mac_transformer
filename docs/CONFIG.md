# Configuration reference

Config lives at `~/.mac-transformer/config.json`. Edit it via the app's Settings tab or `PUT /config`. Every field is optional — missing keys use the built-in defaults.

## Core

| Field | Default | Description |
|---|---|---|
| `output_dir` | `~/Documents/MacTransformer` | Where artifacts go when `layout != beside` |
| `default_layout` | `beside` | Layout: `beside` (next to input), `flat`, or `subfolder` |
| `user_plugin_dir` | `~/.mac-transformer/plugins` | Where the app looks for user-added plugins |
| `port` | `8771` | REST/MCP bind port. Restart required to rebind |

## TTS

| Field | Default | Description |
|---|---|---|
| `default_tts_engine` | `kokoro` | `kokoro` or `say` |
| `default_kokoro_voice` | `af_bella` | Any voice from `/voices?engine=kokoro` |
| `default_say_voice` | `Samantha` | Any voice from `say -v ?` |
| `default_speed` | `1.0` | 0.5–2.0 |
| `audio_by_chapter` | `false` | Split multi-chapter audio into per-section files |

## Publishing

| Field | Default | Description |
|---|---|---|
| `publish_to_books` | `false` | Add produced PDFs to Apple Books |
| `default_books_collection` | `""` | Books collection name (optional) |
| `publish_to_music` | `false` | Add produced audio to Apple Music |
| `default_music_playlist` | `""` | Music playlist name (optional) |
| `default_music_genre` | `""` | ID3 genre tag |
| `publish_to_reading_list` | `false` | Add PDFs to Safari Reading List (iCloud syncs to iPhone/iPad) |
| `publish_to_obsidian` | `false` | Copy .md/.txt into an Obsidian vault |
| `obsidian_vault` | `""` | Absolute path to vault directory |
| `obsidian_folder` | `""` | Subfolder within vault (optional) |
| `publish_to_notion` | `false` | Create Notion pages via API |
| `notion_token` | `""` | Notion integration token (`secret_...`) |
| `notion_database` | `""` | Target database id (32-char hex, no dashes) |
| `default_tags` | `[]` | macOS Finder tags applied to every artifact |

## PDF

| Field | Default | Description |
|---|---|---|
| `default_pdf_author` | `Mac Transformer` | Stamped into produced PDFs' Info dict as Author |

## Summarization

| Field | Default | Description |
|---|---|---|
| `summarizer_url` | `""` | OpenAI-compatible `/chat/completions` endpoint |
| `summarizer_model` | `gpt-4o-mini` | Model id |
| `summarizer_key` | `""` | Bearer token (empty = no auth header) |
| `summarizer_prompt` | *(built-in)* | System/user prompt template; document text is appended |

Examples:

- **Anthropic Claude** (via a LiteLLM proxy): `http://127.0.0.1:4000/chat/completions`, model `claude-3-5-sonnet-latest`
- **OpenAI**: `https://api.openai.com/v1/chat/completions`, key = your API key
- **Ollama** (local): `http://127.0.0.1:11434/v1/chat/completions`, model `llama3`, no key

## Notifications

| Field | Default | Description |
|---|---|---|
| `notifications_enabled` | `false` | Post a banner on job completion / failure / cancel |

## Recipes

```json
{
  "recipes": [
    {
      "name": "Audiobook",
      "icon": "🎧",
      "request": {
        "outputs": ["audio"],
        "tts": {"engine": "kokoro", "voice": "af_bella", "speed": 1.2},
        "layout": "beside",
        "audio_by_chapter": true,
        "publish_to_music": true,
        "music_playlist": "Reading queue"
      }
    }
  ]
}
```

Any Request field is recipeable. On submit, `req = ApplyRecipe(req, recipe)` — recipe fields override caller fields where both are set.

## Watch folders

```json
{
  "watch_folders": [
    {
      "enabled": true,
      "path": "/Users/chris/Inbox",
      "recipe": "Audiobook",
      "extensions": ["pdf", "docx"]
    }
  ]
}
```

- `path`: absolute directory
- `recipe`: name of a saved recipe to apply on submit (optional)
- `extensions`: filter — empty means "any format the pipeline knows about"
- `recursive`: (not yet implemented) whether to descend into subdirectories

Debounces file events by 1.5s to avoid triggering three times per save.

## Plugin overrides

```json
{
  "plugin_overrides": {
    "docx->pdf": "libreoffice-headless",
    "pdf->txt": "ocr-pdf"
  }
}
```

Forces a specific plugin for a given conversion. Skips the priority-based fallback chain — if the override fails, the whole step fails.
