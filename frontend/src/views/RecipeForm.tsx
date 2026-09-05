import { OutputKind, OutputLayout, Recipe, TTSEngine } from '../api'

interface Props {
  recipe: Recipe
  onChange: (r: Recipe) => void
}

// RecipeForm renders each field of a Recipe.request as a proper form
// control, replacing the raw JSON textarea. Fields with no value are
// omitted from the saved recipe so they inherit defaults at run time.
export function RecipeForm({ recipe, onChange }: Props) {
  const r = recipe.request
  const setReq = (patch: Partial<typeof r>) =>
    onChange({ ...recipe, request: { ...r, ...patch } })

  const outputs = (r.outputs as OutputKind[] | undefined) ?? []
  const toggleOutput = (k: OutputKind) => {
    const next = outputs.includes(k) ? outputs.filter((x) => x !== k) : [...outputs, k]
    setReq({ outputs: next })
  }

  return (
    <div className="recipe-form">
      <div className="row">
        <label className="lbl">Outputs</label>
        <div className="chips">
          {(['pdf', 'text', 'audio', 'summary'] as OutputKind[]).map((k) => (
            <button
              key={k}
              type="button"
              className={outputs.includes(k) ? 'chip active' : 'chip'}
              onClick={() => toggleOutput(k)}
            >
              {k}
            </button>
          ))}
        </div>
      </div>

      <div className="row">
        <label className="lbl">Layout</label>
        <select
          value={r.layout ?? ''}
          onChange={(e) => setReq({ layout: (e.target.value || undefined) as OutputLayout })}
        >
          <option value="">— inherit —</option>
          <option value="beside">Beside input</option>
          <option value="flat">Flat in output dir</option>
          <option value="subfolder">Subfolder per input</option>
        </select>
      </div>

      <div className="row">
        <label className="lbl">Output dir</label>
        <input
          placeholder="/absolute/path (empty = inherit)"
          value={r.output_dir ?? ''}
          onChange={(e) => setReq({ output_dir: e.target.value || undefined })}
        />
      </div>

      {outputs.includes('audio') && (
        <>
          <div className="row">
            <label className="lbl">TTS engine</label>
            <select
              value={r.tts?.engine ?? ''}
              onChange={(e) =>
                setReq({ tts: { ...(r.tts ?? {}), engine: (e.target.value || undefined) as TTSEngine } })
              }
            >
              <option value="">— inherit —</option>
              <option value="kokoro">Kokoro (local)</option>
              <option value="say">macOS say</option>
            </select>
          </div>
          <div className="row">
            <label className="lbl">Voice</label>
            <input
              placeholder="e.g. af_bella or Samantha"
              value={r.tts?.voice ?? ''}
              onChange={(e) =>
                setReq({ tts: { ...(r.tts ?? {}), voice: e.target.value || undefined } })
              }
            />
          </div>
          <div className="row">
            <label className="lbl">Speed</label>
            <input
              type="number"
              step={0.1}
              min={0.5}
              max={2}
              value={r.tts?.speed ?? ''}
              onChange={(e) =>
                setReq({
                  tts: {
                    ...(r.tts ?? {}),
                    speed: e.target.value ? parseFloat(e.target.value) : undefined,
                  },
                })
              }
            />
          </div>
          <div className="row">
            <label className="lbl">
              <input
                type="checkbox"
                checked={r.audio_by_chapter ?? false}
                onChange={(e) => setReq({ audio_by_chapter: e.target.checked || undefined })}
              />
              &nbsp;Split by chapter
            </label>
          </div>
        </>
      )}

      <div className="row">
        <label className="lbl">Tags</label>
        <input
          placeholder="comma-separated"
          value={(r.tags ?? []).join(', ')}
          onChange={(e) =>
            setReq({
              tags: e.target.value.split(',').map((s) => s.trim()).filter(Boolean),
            })
          }
        />
      </div>

      <div className="row">
        <label className="lbl">
          <input
            type="checkbox"
            checked={r.publish_to_books ?? false}
            onChange={(e) => setReq({ publish_to_books: e.target.checked || undefined })}
          />
          &nbsp;📚 Books
        </label>
        <input
          placeholder="collection (optional)"
          value={r.books_collection ?? ''}
          onChange={(e) => setReq({ books_collection: e.target.value || undefined })}
          disabled={!r.publish_to_books}
        />
      </div>

      <div className="row">
        <label className="lbl">
          <input
            type="checkbox"
            checked={r.publish_to_music ?? false}
            onChange={(e) => setReq({ publish_to_music: e.target.checked || undefined })}
          />
          &nbsp;🎵 Music
        </label>
        <input
          placeholder="playlist (optional)"
          value={r.music_playlist ?? ''}
          onChange={(e) => setReq({ music_playlist: e.target.value || undefined })}
          disabled={!r.publish_to_music}
        />
      </div>

      <div className="row">
        <label className="lbl">
          <input
            type="checkbox"
            checked={r.publish_to_reading_list ?? false}
            onChange={(e) => setReq({ publish_to_reading_list: e.target.checked || undefined })}
          />
          &nbsp;📖 Reading List
        </label>
      </div>

      <div className="row">
        <label className="lbl">
          <input
            type="checkbox"
            checked={r.publish_to_obsidian ?? false}
            onChange={(e) => setReq({ publish_to_obsidian: e.target.checked || undefined })}
          />
          &nbsp;🔮 Obsidian
        </label>
      </div>

      <div className="row">
        <label className="lbl">
          <input
            type="checkbox"
            checked={r.publish_to_notion ?? false}
            onChange={(e) => setReq({ publish_to_notion: e.target.checked || undefined })}
          />
          &nbsp;📝 Notion
        </label>
      </div>
    </div>
  )
}
