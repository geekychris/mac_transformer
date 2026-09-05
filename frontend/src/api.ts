// Thin fetch-based client for the local REST API on 127.0.0.1:8771. The
// frontend runs inside a Wails WebView, so this is same-origin from its
// point of view; we use fetch instead of Wails bindings so the same code
// works if we ever open the UI in a plain browser tab.

const BASE = 'http://127.0.0.1:8771'

export type OutputKind = 'pdf' | 'text' | 'audio'
export type TTSEngine = 'kokoro' | 'say'
export type OutputLayout = 'flat' | 'subfolder' | 'beside'
export type JobStatus = 'pending' | 'running' | 'completed' | 'failed' | 'cancelled'

export interface PublishResult {
  target: 'books' | 'music' | 'tags' | 'reading_list' | 'obsidian' | 'notion'
  ok: boolean
  detail?: string
  error?: string
}

export interface Recipe {
  name: string
  icon?: string
  request: Partial<TransformRequest>
}

export interface WatchFolder {
  enabled: boolean
  path: string
  recipe?: string
  extensions?: string[]
  recursive?: boolean
}

export interface Artifact {
  kind?: OutputKind
  format: string
  path: string
  plugin: string
  bytes: number
  publish?: PublishResult[]
  tags?: string[]
}

export type StepStatus = 'pending' | 'running' | 'completed' | 'failed' | 'skipped'

export interface StepReport {
  kind: OutputKind
  status: StepStatus
  plugin?: string
  chain?: string[]
  message?: string
  error?: string
  started?: string
  ended?: string
}

export interface Job {
  id: string
  request: TransformRequest
  status: JobStatus
  message?: string
  artifacts?: Artifact[]
  steps?: StepReport[]
  created: string
  started?: string
  ended?: string
}

export interface TransformRequest {
  path: string
  outputs: OutputKind[]
  tts?: { engine?: TTSEngine; voice?: string; speed?: number }
  output_dir?: string
  layout?: OutputLayout
  plugin_overrides?: Record<string, string>

  publish_to_books?: boolean
  books_collection?: string
  publish_to_music?: boolean
  music_playlist?: string
  music_genre?: string
  publish_to_reading_list?: boolean
  publish_to_obsidian?: boolean
  publish_to_notion?: boolean
  tags?: string[]
  audio_by_chapter?: boolean
  recipe?: string
  force?: boolean
}

export interface PluginView {
  name: string
  description: string
  capabilities: { from: string[]; to: string[]; priority: number }[]
  available: boolean
  reason?: string
  bundled: boolean
}

export interface Config {
  default_tts_engine: TTSEngine
  default_kokoro_voice: string
  default_say_voice: string
  default_speed: number
  output_dir: string
  default_layout: OutputLayout
  user_plugin_dir: string
  port: number
  plugin_overrides?: Record<string, string>

  publish_to_books: boolean
  default_books_collection: string
  publish_to_music: boolean
  default_music_playlist: string
  default_music_genre: string
  default_tags?: string[]
  publish_to_reading_list: boolean
  notifications_enabled: boolean
  audio_by_chapter: boolean
  recipes?: Recipe[]
  watch_folders?: WatchFolder[]
}

async function j<T>(r: Response): Promise<T> {
  if (!r.ok) throw new Error(`${r.status}: ${await r.text()}`)
  return r.json() as Promise<T>
}

export const api = {
  health: () => fetch(`${BASE}/health`).then(j<any>),
  transform: async (req: TransformRequest): Promise<{ job: Job; duplicate: boolean }> => {
    const r = await fetch(`${BASE}/transform`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(req),
    })
    if (!r.ok) throw new Error(`${r.status}: ${await r.text()}`)
    const job = (await r.json()) as Job
    return { job, duplicate: r.headers.get('X-Duplicate-Of') !== null }
  },
  listJobs: () => fetch(`${BASE}/jobs`).then(j<Job[]>),
  getJob: (id: string) => fetch(`${BASE}/jobs/${id}`).then(j<Job>),
  listPlugins: () => fetch(`${BASE}/plugins`).then(j<PluginView[]>),
  reloadPlugins: () => fetch(`${BASE}/plugins/reload`, { method: 'POST' }).then(j<any>),
  listVoices: (engine: TTSEngine) =>
    fetch(`${BASE}/voices?engine=${engine}`).then(j<{ engine: TTSEngine; voices: string[] }>),
  getConfig: () => fetch(`${BASE}/config`).then(j<Config>),
  setConfig: (c: Config) =>
    fetch(`${BASE}/config`, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(c),
    }).then(j<Config>),
  cancelJob: (id: string) =>
    fetch(`${BASE}/jobs/${id}/cancel`, { method: 'POST' }).then(j<Job>),
  retryJob: (id: string, kinds?: OutputKind[]) =>
    fetch(`${BASE}/jobs/${id}/retry`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(kinds ? { kinds } : {}),
    }).then(j<Job>),
  listRecipes: () => fetch(`${BASE}/recipes`).then(j<Recipe[]>),
  publishJob: (
    id: string,
    opts: {
      publish_to_books?: boolean
      books_collection?: string
      publish_to_music?: boolean
      music_playlist?: string
      music_genre?: string
      publish_to_reading_list?: boolean
      tags?: string[]
    }
  ) =>
    fetch(`${BASE}/jobs/${id}/publish`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(opts),
    }).then(j<Job>),
  openArtifact: (path: string) =>
    fetch(`${BASE}/open`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ path }),
    }).then(j<any>),
  revealArtifact: (path: string) =>
    fetch(`${BASE}/reveal`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ path }),
    }).then(j<any>),
}

// subscribeFilesReceived listens for the "files-received" custom event Go
// emits when files land on us via drag-drop, Finder "Open With", or CLI
// args. Uses the Wails runtime; safe to call from the browser too (it just
// becomes a no-op if the runtime isn't present).
export function subscribeFilesReceived(cb: (paths: string[]) => void): () => void {
  let cancel: undefined | (() => void)
  // Dynamic import so a plain-browser dev flow doesn't blow up.
  import('@wailsio/runtime')
    .then(({ Events }) => {
      const off = Events.On('files-received', (e: any) => {
        const files: string[] = Array.isArray(e?.data) ? e.data : []
        if (files.length) cb(files)
      })
      cancel = typeof off === 'function' ? off : undefined
    })
    .catch(() => {})
  return () => cancel?.()
}

// Server-Sent Events stream. The caller supplies a handler; the returned
// function closes the connection.
export function subscribeEvents(onEvent: (ev: { type: string; job: Job }) => void): () => void {
  const src = new EventSource(`${BASE}/events`)
  const types = ['created', 'started', 'progress', 'completed', 'failed']
  for (const t of types) {
    src.addEventListener(t, (e) => {
      const data = JSON.parse((e as MessageEvent).data)
      onEvent(data)
    })
  }
  return () => src.close()
}
