import { useEffect, useState } from 'react'
import { api, Config, OutputLayout, Recipe, TTSEngine, WatchFolder } from '../api'
import { RecipeForm } from './RecipeForm'

interface Props {
  config: Config
  onSave: (c: Config) => Promise<void>
}

export function SettingsView({ config, onSave }: Props) {
  const [c, setC] = useState<Config>(config)
  const [voices, setVoices] = useState<string[]>([])
  const [status, setStatus] = useState<string | null>(null)

  useEffect(() => setC(config), [config])
  useEffect(() => {
    api
      .listVoices(c.default_tts_engine)
      .then((r) => setVoices(r.voices))
      .catch(() => setVoices([]))
  }, [c.default_tts_engine])

  const upd = (patch: Partial<Config>) => setC({ ...c, ...patch })

  const save = async () => {
    setStatus(null)
    try {
      await onSave(c)
      setStatus('Saved.')
    } catch (e: any) {
      setStatus('Error: ' + (e.message ?? String(e)))
    }
  }

  return (
    <div className="settings">
      <div className="row">
        <label className="lbl">Default TTS engine</label>
        <select
          value={c.default_tts_engine}
          onChange={(e) => upd({ default_tts_engine: e.target.value as TTSEngine })}
        >
          <option value="kokoro">Kokoro (local)</option>
          <option value="say">macOS say</option>
        </select>
      </div>
      <div className="row">
        <label className="lbl">Default Kokoro voice</label>
        <input
          list="kokoro-voices"
          value={c.default_kokoro_voice}
          onChange={(e) => upd({ default_kokoro_voice: e.target.value })}
        />
        <datalist id="kokoro-voices">
          {c.default_tts_engine === 'kokoro' &&
            voices.map((v) => <option key={v} value={v} />)}
        </datalist>
      </div>
      <div className="row">
        <label className="lbl">Default `say` voice</label>
        <input
          list="say-voices"
          value={c.default_say_voice}
          onChange={(e) => upd({ default_say_voice: e.target.value })}
        />
        <datalist id="say-voices">
          {c.default_tts_engine === 'say' && voices.map((v) => <option key={v} value={v} />)}
        </datalist>
      </div>
      <div className="row">
        <label className="lbl">Default speed</label>
        <input
          type="number"
          step={0.1}
          min={0.5}
          max={2.0}
          value={c.default_speed}
          onChange={(e) => upd({ default_speed: parseFloat(e.target.value) })}
        />
      </div>
      <div className="row">
        <label className="lbl">Output directory</label>
        <input value={c.output_dir} onChange={(e) => upd({ output_dir: e.target.value })} />
      </div>
      <div className="row">
        <label className="lbl">Default layout</label>
        <select
          value={c.default_layout}
          onChange={(e) => upd({ default_layout: e.target.value as OutputLayout })}
        >
          <option value="subfolder">Subfolder per input</option>
          <option value="flat">Flat in output directory</option>
          <option value="beside">Beside the input file</option>
        </select>
      </div>

      <div className="section-hdr">Publishing</div>
      <div className="row">
        <label className="lbl">
          <input
            type="checkbox"
            checked={c.publish_to_books}
            onChange={(e) => upd({ publish_to_books: e.target.checked })}
          />
          &nbsp;PDFs → Apple Books
        </label>
        <input
          placeholder="default collection (optional)"
          value={c.default_books_collection ?? ''}
          onChange={(e) => upd({ default_books_collection: e.target.value })}
        />
      </div>
      <div className="row">
        <label className="lbl">
          <input
            type="checkbox"
            checked={c.publish_to_music}
            onChange={(e) => upd({ publish_to_music: e.target.checked })}
          />
          &nbsp;Audio → Apple Music
        </label>
        <input
          placeholder="default playlist (optional)"
          value={c.default_music_playlist ?? ''}
          onChange={(e) => upd({ default_music_playlist: e.target.value })}
        />
        <input
          placeholder="default genre (optional)"
          value={c.default_music_genre ?? ''}
          onChange={(e) => upd({ default_music_genre: e.target.value })}
        />
      </div>
      <div className="row">
        <label className="lbl">
          <input
            type="checkbox"
            checked={c.publish_to_reading_list}
            onChange={(e) => upd({ publish_to_reading_list: e.target.checked })}
          />
          &nbsp;PDFs → Safari Reading List
        </label>
        <span className="muted">iCloud syncs to iPhone/iPad.</span>
      </div>
      <div className="row">
        <label className="lbl">Default Finder tags</label>
        <input
          placeholder="comma-separated"
          value={(c.default_tags ?? []).join(', ')}
          onChange={(e) =>
            upd({
              default_tags: e.target.value
                .split(',')
                .map((s) => s.trim())
                .filter(Boolean),
            })
          }
        />
      </div>

      <div className="section-hdr">Audio</div>
      <div className="row">
        <label className="lbl">
          <input
            type="checkbox"
            checked={c.audio_by_chapter}
            onChange={(e) => upd({ audio_by_chapter: e.target.checked })}
          />
          &nbsp;Split audio by chapter (with ID3 tags)
        </label>
        <span className="muted">Needs ffmpeg for tagging.</span>
      </div>

      <div className="section-hdr">Notifications</div>
      <div className="row">
        <label className="lbl">
          <input
            type="checkbox"
            checked={c.notifications_enabled}
            onChange={(e) => upd({ notifications_enabled: e.target.checked })}
          />
          &nbsp;Post banner when jobs finish
        </label>
      </div>

      <div className="section-hdr">Recipes</div>
      <RecipesEditor
        recipes={c.recipes ?? []}
        onChange={(recipes) => upd({ recipes })}
      />

      <div className="section-hdr">Watch folders</div>
      <WatchFoldersEditor
        folders={c.watch_folders ?? []}
        recipes={c.recipes ?? []}
        onChange={(watch_folders) => upd({ watch_folders })}
      />

      <div className="row">
        <label className="lbl">User plugin directory</label>
        <input
          value={c.user_plugin_dir}
          onChange={(e) => upd({ user_plugin_dir: e.target.value })}
        />
      </div>
      <div className="row">
        <label className="lbl">REST/MCP port</label>
        <input
          type="number"
          value={c.port}
          onChange={(e) => upd({ port: parseInt(e.target.value, 10) })}
        />
        <span className="muted">Restart required to rebind.</span>
      </div>

      {status && <div className="row status">{status}</div>}

      <div className="row actions">
        <button className="btn primary" onClick={save}>
          Save
        </button>
      </div>
    </div>
  )
}

// ---- Sub-components ----------------------------------------------------

// RecipesEditor renders one row per recipe. Each row is a JSON textarea
// backing the request template plus a name field. Simple / minimal so the
// user can save any set of Request fields as a recipe.
function RecipesEditor({
  recipes,
  onChange,
}: {
  recipes: Recipe[]
  onChange: (r: Recipe[]) => void
}) {
  const update = (i: number, patch: Partial<Recipe>) => {
    const next = recipes.slice()
    next[i] = { ...next[i], ...patch }
    onChange(next)
  }
  const remove = (i: number) => {
    const next = recipes.slice()
    next.splice(i, 1)
    onChange(next)
  }
  const add = () => {
    onChange([
      ...recipes,
      {
        name: `Recipe ${recipes.length + 1}`,
        request: {
          outputs: ['pdf', 'text', 'audio'],
          tts: { engine: 'kokoro', voice: 'af_bella', speed: 1.0 },
          layout: 'beside',
        },
      },
    ])
  }
  return (
    <div className="recipes-editor">
      {recipes.length === 0 && <div className="muted">No recipes yet.</div>}
      {recipes.map((r, i) => (
        <div key={i} className="recipe-row">
          <div className="row">
            <input
              className="recipe-icon"
              placeholder="🎧"
              value={r.icon ?? ''}
              onChange={(e) => update(i, { icon: e.target.value })}
              maxLength={2}
            />
            <input
              className="recipe-name"
              value={r.name}
              onChange={(e) => update(i, { name: e.target.value })}
              placeholder="Name"
            />
            <button className="btn small" onClick={() => remove(i)}>
              Remove
            </button>
          </div>
          <RecipeForm
            recipe={r}
            onChange={(next) => {
              const arr = recipes.slice()
              arr[i] = next
              onChange(arr)
            }}
          />
        </div>
      ))}
      <button className="btn" onClick={add}>
        Add recipe
      </button>
    </div>
  )
}

// WatchFoldersEditor renders a folder path, its recipe pick, extensions
// filter, and enabled toggle. Uses the same recipes list so we can present
// a dropdown of names.
function WatchFoldersEditor({
  folders,
  recipes,
  onChange,
}: {
  folders: WatchFolder[]
  recipes: Recipe[]
  onChange: (f: WatchFolder[]) => void
}) {
  const update = (i: number, patch: Partial<WatchFolder>) => {
    const next = folders.slice()
    next[i] = { ...next[i], ...patch }
    onChange(next)
  }
  const remove = (i: number) => {
    const next = folders.slice()
    next.splice(i, 1)
    onChange(next)
  }
  const add = () => {
    onChange([...folders, { enabled: true, path: '', recipe: '', extensions: [] }])
  }
  return (
    <div className="watch-editor">
      {folders.length === 0 && <div className="muted">No watch folders configured.</div>}
      {folders.map((f, i) => (
        <div key={i} className="watch-row">
          <div className="row">
            <label className="lbl">
              <input
                type="checkbox"
                checked={f.enabled}
                onChange={(e) => update(i, { enabled: e.target.checked })}
              />
              &nbsp;on
            </label>
            <input
              placeholder="/absolute/path/to/folder"
              value={f.path}
              onChange={(e) => update(i, { path: e.target.value })}
            />
          </div>
          <div className="row">
            <label className="lbl">Recipe</label>
            <select
              value={f.recipe ?? ''}
              onChange={(e) => update(i, { recipe: e.target.value })}
            >
              <option value="">— none —</option>
              {recipes.map((r) => (
                <option key={r.name} value={r.name}>
                  {r.name}
                </option>
              ))}
            </select>
            <input
              placeholder="extensions (comma, blank = any known type)"
              value={(f.extensions ?? []).join(', ')}
              onChange={(e) =>
                update(i, {
                  extensions: e.target.value
                    .split(',')
                    .map((s) => s.trim())
                    .filter(Boolean),
                })
              }
            />
            <button className="btn small" onClick={() => remove(i)}>
              Remove
            </button>
          </div>
        </div>
      ))}
      <button className="btn" onClick={add}>
        Add folder
      </button>
    </div>
  )
}
