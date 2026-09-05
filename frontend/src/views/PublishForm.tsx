import { useState } from 'react'
import { api, Artifact } from '../api'

interface Props {
  jobId: string
  artifacts: Artifact[]
  onDone: () => void
  onCancel: () => void
}

// PublishForm is the inline retroactive-publish form shown on each job's
// expanded body. It renders only the target sections whose artifact types
// are actually present in the job — no point offering "Music playlist" on
// a job with no audio artifact.
export function PublishForm({ jobId, artifacts, onDone, onCancel }: Props) {
  const hasPDF = artifacts.some((a) => a.format === 'pdf')
  const hasAudio = artifacts.some((a) => ['mp3', 'aiff', 'wav'].includes(a.format))

  const [books, setBooks] = useState(false)
  const [booksCollection, setBooksCollection] = useState('')
  const [music, setMusic] = useState(false)
  const [musicPlaylist, setMusicPlaylist] = useState('')
  const [musicGenre, setMusicGenre] = useState('')
  const [readingList, setReadingList] = useState(false)
  const [tagsText, setTagsText] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const submit = async () => {
    setError(null)
    setBusy(true)
    try {
      const tags = tagsText.split(',').map((s) => s.trim()).filter(Boolean)
      await api.publishJob(jobId, {
        publish_to_books: hasPDF && books,
        books_collection: booksCollection || undefined,
        publish_to_music: hasAudio && music,
        music_playlist: musicPlaylist || undefined,
        music_genre: musicGenre || undefined,
        publish_to_reading_list: hasPDF && readingList,
        tags,
      })
      onDone()
    } catch (e: any) {
      setError(e.message ?? String(e))
    } finally {
      setBusy(false)
    }
  }

  const nothingChecked = !(
    (hasPDF && books) ||
    (hasAudio && music) ||
    (hasPDF && readingList) ||
    tagsText.trim()
  )

  return (
    <div className="retro-publish">
      <div className="retro-hdr muted subhead">Publish this job</div>
      {hasPDF && (
        <>
          <div className="row">
            <label className="lbl">
              <input type="checkbox" checked={books} onChange={(e) => setBooks(e.target.checked)} />
              &nbsp;📚 PDF → Books
            </label>
            <input
              placeholder="collection (optional)"
              value={booksCollection}
              onChange={(e) => setBooksCollection(e.target.value)}
              disabled={!books}
            />
          </div>
          <div className="row">
            <label className="lbl">
              <input
                type="checkbox"
                checked={readingList}
                onChange={(e) => setReadingList(e.target.checked)}
              />
              &nbsp;📖 PDF → Safari Reading List
            </label>
          </div>
        </>
      )}
      {hasAudio && (
        <div className="row">
          <label className="lbl">
            <input type="checkbox" checked={music} onChange={(e) => setMusic(e.target.checked)} />
            &nbsp;🎵 Audio → Music
          </label>
          <input
            placeholder="playlist (optional)"
            value={musicPlaylist}
            onChange={(e) => setMusicPlaylist(e.target.value)}
            disabled={!music}
          />
          <input
            placeholder="genre (optional)"
            value={musicGenre}
            onChange={(e) => setMusicGenre(e.target.value)}
            disabled={!music}
          />
        </div>
      )}
      <div className="row">
        <label className="lbl">🏷 Tags</label>
        <input
          placeholder="comma-separated (e.g. Research, Follow-up)"
          value={tagsText}
          onChange={(e) => setTagsText(e.target.value)}
        />
      </div>

      {!hasPDF && !hasAudio && (
        <div className="muted">
          Only tags are available for this job (no PDF / audio artifacts).
        </div>
      )}
      {error && <div className="err">{error}</div>}

      <div className="row actions">
        <button className="btn" onClick={onCancel} type="button">
          Cancel
        </button>
        <button
          className="btn primary"
          onClick={submit}
          disabled={busy || nothingChecked}
          type="button"
        >
          {busy ? 'Publishing…' : 'Publish'}
        </button>
      </div>
    </div>
  )
}
