import { useEffect, useState, DragEvent } from 'react'
import { api, Config, OutputKind, OutputLayout, Recipe, TTSEngine } from '../api'

interface Props {
  config: Config
  prefillPaths?: string[]
  onConsumePrefill?: () => void
  onSubmit: () => void
}

// DropZone lets the user pick files (or drop them onto the window) and choose
// which artifacts to produce. Wails v3's WebView exposes dropped file paths
// via a runtime event; here we fall back to reading File.path (Chromium /
// Wails-webkit specific) if the runtime event isn't set up.
export function DropZone({ config, prefillPaths, onConsumePrefill, onSubmit }: Props) {
  const [pathInput, setPathInput] = useState('')

  // Merge new prefilled paths (from drag-drop / Open With / CLI args) into
  // the textarea without stomping whatever the user was typing.
  useEffect(() => {
    if (!prefillPaths || prefillPaths.length === 0) return
    setPathInput((cur) => {
      const existing = cur.split('\n').map((s) => s.trim()).filter(Boolean)
      const merged = Array.from(new Set([...existing, ...prefillPaths]))
      return merged.join('\n')
    })
    onConsumePrefill?.()
  }, [prefillPaths, onConsumePrefill])
  const [outputs, setOutputs] = useState<OutputKind[]>(['pdf', 'text', 'audio'])
  const [engine, setEngine] = useState<TTSEngine>(config.default_tts_engine)
  const [voice, setVoice] = useState(config.default_kokoro_voice)
  const [speed, setSpeed] = useState(config.default_speed)
  const [outputDir, setOutputDir] = useState(config.output_dir)
  const [layout, setLayout] = useState<OutputLayout>(config.default_layout || 'subfolder')
  const [publishBooks, setPublishBooks] = useState<boolean>(config.publish_to_books)
  const [booksCollection, setBooksCollection] = useState<string>(config.default_books_collection ?? '')
  const [publishMusic, setPublishMusic] = useState<boolean>(config.publish_to_music)
  const [musicPlaylist, setMusicPlaylist] = useState<string>(config.default_music_playlist ?? '')
  const [musicGenre, setMusicGenre] = useState<string>(config.default_music_genre ?? '')
  const [publishReadingList, setPublishReadingList] = useState<boolean>(config.publish_to_reading_list)
  const [tagsText, setTagsText] = useState<string>((config.default_tags ?? []).join(', '))
  const [publishOpen, setPublishOpen] = useState<boolean>(false)
  const [audioByChapter, setAudioByChapter] = useState<boolean>(config.audio_by_chapter)
  const [recipes, setRecipes] = useState<Recipe[]>([])
  const [selectedRecipe, setSelectedRecipe] = useState<string>('')

  useEffect(() => {
    api.listRecipes().then(setRecipes).catch(() => setRecipes([]))
  }, [])
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const toggle = (k: OutputKind) => {
    setOutputs((cur) => (cur.includes(k) ? cur.filter((x) => x !== k) : [...cur, k]))
  }

  const submit = async () => {
    setError(null)
    const paths = pathInput.split('\n').map((s) => s.trim()).filter(Boolean)
    if (paths.length === 0) {
      setError('Provide at least one absolute file path.')
      return
    }
    setBusy(true)
    try {
      const tags = tagsText
        .split(',')
        .map((s) => s.trim())
        .filter(Boolean)
      const submit = (p: string, force?: boolean) =>
        api.transform({
          path: p,
          outputs,
          tts: { engine, voice, speed },
          output_dir: outputDir,
          layout,
          publish_to_books: publishBooks,
          books_collection: booksCollection || undefined,
          publish_to_music: publishMusic,
          music_playlist: musicPlaylist || undefined,
          music_genre: musicGenre || undefined,
          publish_to_reading_list: publishReadingList,
          tags,
          audio_by_chapter: audioByChapter,
          recipe: selectedRecipe || undefined,
          force,
        })
      const dupes: string[] = []
      for (const p of paths) {
        const res = await submit(p)
        if (res.duplicate) dupes.push(p)
      }
      if (dupes.length > 0) {
        const msg =
          dupes.length === 1
            ? `"${dupes[0].split('/').pop()}" was already processed. Re-run anyway?`
            : `${dupes.length} of the submitted files were already processed. Re-run all?`
        if (window.confirm(msg)) {
          for (const p of dupes) {
            await submit(p, true)
          }
        }
      }
      setPathInput('')
      onSubmit()
    } catch (e: any) {
      setError(e.message ?? String(e))
    } finally {
      setBusy(false)
    }
  }

  // Drop paths arrive via the Wails "files-received" event forwarded by
  // Go — App.tsx feeds them in as prefillPaths. The DOM handlers here just
  // suppress the browser's default (which would open the file in-window)
  // so the OS-level drop reaches Wails.
  const onDrop = (e: DragEvent<HTMLDivElement>) => e.preventDefault()
  const onDragOver = (e: DragEvent<HTMLDivElement>) => e.preventDefault()

  return (
    <div className="dropzone">
      <div className="drop" onDrop={onDrop} onDragOver={onDragOver}>
        <div className="drop-hint">Drop files here or paste paths below</div>
        <textarea
          className="path-input"
          placeholder={`/absolute/path/to/document.docx\n/absolute/path/to/slides.pptx`}
          value={pathInput}
          onChange={(e) => setPathInput(e.target.value)}
          rows={4}
        />
      </div>

      {recipes.length > 0 && (
        <div className="row">
          <label className="lbl">Recipe</label>
          <select
            value={selectedRecipe}
            onChange={(e) => setSelectedRecipe(e.target.value)}
          >
            <option value="">— none (use fields below) —</option>
            {recipes.map((r) => (
              <option key={r.name} value={r.name}>
                {r.icon ? r.icon + ' ' : ''}
                {r.name}
              </option>
            ))}
          </select>
        </div>
      )}

      <div className="row">
        <label className="lbl">Outputs</label>
        <div className="chips">
          {(['pdf', 'text', 'audio'] as OutputKind[]).map((k) => (
            <button
              key={k}
              className={outputs.includes(k) ? 'chip active' : 'chip'}
              onClick={() => toggle(k)}
              type="button"
            >
              {k}
            </button>
          ))}
        </div>
      </div>

      {outputs.includes('audio') && (
        <>
          <div className="row">
            <label className="lbl">TTS engine</label>
            <select value={engine} onChange={(e) => setEngine(e.target.value as TTSEngine)}>
              <option value="kokoro">Kokoro (local)</option>
              <option value="say">macOS say</option>
            </select>
          </div>
          <div className="row">
            <label className="lbl">Voice</label>
            <input value={voice} onChange={(e) => setVoice(e.target.value)} />
          </div>
          <div className="row">
            <label className="lbl">Speed</label>
            <input
              type="number"
              step="0.1"
              min={0.5}
              max={2.0}
              value={speed}
              onChange={(e) => setSpeed(parseFloat(e.target.value))}
            />
          </div>
          <div className="row">
            <label className="lbl">
              <input
                type="checkbox"
                checked={audioByChapter}
                onChange={(e) => setAudioByChapter(e.target.checked)}
              />
              &nbsp;Split by chapter (with ID3 tags)
            </label>
          </div>
        </>
      )}

      <div className="row">
        <label className="lbl">Write artifacts</label>
        <div className="chips">
          {(
            [
              ['subfolder', 'in subfolder', `${outputDir}/<name>/…`],
              ['flat',      'flat',         `${outputDir}/<name>.ext`],
              ['beside',    'beside input', `<dir(input)>/<name>.ext`],
            ] as [OutputLayout, string, string][]
          ).map(([k, label, hint]) => (
            <button
              key={k}
              className={layout === k ? 'chip active' : 'chip'}
              onClick={() => setLayout(k)}
              title={hint}
              type="button"
            >
              {label}
            </button>
          ))}
        </div>
      </div>

      {layout !== 'beside' && (
        <div className="row">
          <label className="lbl">Output dir</label>
          <input value={outputDir} onChange={(e) => setOutputDir(e.target.value)} />
        </div>
      )}

      <div className="disclosure-section">
        <button
          type="button"
          className="disclosure-btn"
          onClick={() => setPublishOpen((v) => !v)}
          aria-expanded={publishOpen}
        >
          {publishOpen ? '▼' : '▶'} Publish &amp; tag {(publishBooks || publishMusic || tagsText) ? '·' : ''}
          {publishBooks ? ' 📚' : ''}
          {publishMusic ? ' 🎵' : ''}
          {tagsText ? ` 🏷 ${tagsText}` : ''}
        </button>

        {publishOpen && (
          <div className="publish-panel">
            <div className="row">
              <label className="lbl">
                <input
                  type="checkbox"
                  checked={publishBooks}
                  onChange={(e) => setPublishBooks(e.target.checked)}
                />
                &nbsp;Publish PDF to Books
              </label>
              <input
                placeholder="collection (optional)"
                value={booksCollection}
                onChange={(e) => setBooksCollection(e.target.value)}
                disabled={!publishBooks}
              />
            </div>

            <div className="row">
              <label className="lbl">
                <input
                  type="checkbox"
                  checked={publishReadingList}
                  onChange={(e) => setPublishReadingList(e.target.checked)}
                />
                &nbsp;Add PDF to Safari Reading List
              </label>
            </div>

            <div className="row">
              <label className="lbl">
                <input
                  type="checkbox"
                  checked={publishMusic}
                  onChange={(e) => setPublishMusic(e.target.checked)}
                />
                &nbsp;Publish audio to Music
              </label>
              <input
                placeholder="playlist (optional)"
                value={musicPlaylist}
                onChange={(e) => setMusicPlaylist(e.target.value)}
                disabled={!publishMusic}
              />
              <input
                placeholder="genre (optional)"
                value={musicGenre}
                onChange={(e) => setMusicGenre(e.target.value)}
                disabled={!publishMusic}
              />
            </div>

            <div className="row">
              <label className="lbl">Tags</label>
              <input
                placeholder="comma-separated (e.g. Research, Notes)"
                value={tagsText}
                onChange={(e) => setTagsText(e.target.value)}
              />
            </div>
          </div>
        )}
      </div>

      {error && <div className="err">{error}</div>}

      <div className="row actions">
        <button className="btn primary" disabled={busy} onClick={submit}>
          {busy ? 'Submitting…' : 'Transform'}
        </button>
      </div>
    </div>
  )
}
