import { useEffect, useRef, useState } from 'react'
import { api, Config, Job, PluginView, subscribeEvents, subscribeFilesReceived } from './api'
import { DropZone } from './views/DropZone'
import { QueueView } from './views/QueueView'
import { SettingsView } from './views/SettingsView'
import { PluginsView } from './views/PluginsView'
import './app.css'

type Tab = 'transform' | 'queue' | 'plugins' | 'settings' | 'search'

interface SearchHit {
  job_id: string
  path: string
  snippet: string
}

function App() {
  const [tab, setTab] = useState<Tab>('transform')
  const [jobs, setJobs] = useState<Job[]>([])
  const [plugins, setPlugins] = useState<PluginView[]>([])
  const [config, setConfig] = useState<Config | null>(null)
  const [, setHealth] = useState<any>(null)
  const [pendingPaths, setPendingPaths] = useState<string[]>([])
  const [, setNow] = useState(0) // heartbeat for live-updating timers
  const [searchQ, setSearchQ] = useState('')
  const [searchHits, setSearchHits] = useState<SearchHit[]>([])
  const [searchBusy, setSearchBusy] = useState(false)
  const pendingRef = useRef<string[]>([])

  const runSearch = async (q: string) => {
    if (!q.trim()) {
      setSearchHits([])
      return
    }
    setSearchBusy(true)
    try {
      const r = await fetch(`http://127.0.0.1:8771/search?q=${encodeURIComponent(q)}`)
      const data = (await r.json()) as SearchHit[]
      setSearchHits(data)
      setTab('search')
    } catch (e) {
      console.error(e)
    } finally {
      setSearchBusy(false)
    }
  }

  const refresh = async () => {
    setJobs(await api.listJobs().catch(() => []))
    setPlugins(await api.listPlugins().catch(() => []))
    setHealth(await api.health().catch(() => null))
  }

  useEffect(() => {
    refresh()
    api.getConfig().then(setConfig).catch(console.error)
    const stop = subscribeEvents((ev) => {
      setJobs((cur) => {
        const idx = cur.findIndex((j) => j.id === ev.job.id)
        if (idx === -1) return [ev.job, ...cur]
        const next = [...cur]
        next[idx] = ev.job
        return next
      })
    })
    const stopFiles = subscribeFilesReceived((paths) => {
      // Prefill DropZone with received paths and switch to that tab.
      const next = [...pendingRef.current, ...paths]
      pendingRef.current = next
      setPendingPaths(next)
      setTab('transform')
    })
    // Tick every second so "0.4s" -> "1.4s" while running. Cheap: just
    // bumps a state that other components already re-render from.
    const heartbeat = window.setInterval(() => setNow((n) => n + 1), 1000)
    return () => {
      stop()
      stopFiles()
      window.clearInterval(heartbeat)
    }
  }, [])

  return (
    <div className="app">
      <header className="hdr">
        <div className="hdr-title">Mac Transformer</div>
        <nav className="hdr-tabs">
          {(['transform', 'queue', 'plugins', 'settings'] as Tab[]).map((t) => (
            <button
              key={t}
              onClick={() => setTab(t)}
              className={t === tab ? 'tab active' : 'tab'}
            >
              {t}
            </button>
          ))}
        </nav>
        <div className="hdr-status">
          <input
            className="search-box"
            placeholder="🔍 search extracted text…"
            value={searchQ}
            onChange={(e) => setSearchQ(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === 'Enter') runSearch(searchQ)
            }}
          />
          {searchBusy && <span className="muted"> …</span>}
        </div>
      </header>

      <main className="body">
        {tab === 'transform' && config && (
          <DropZone
            config={config}
            prefillPaths={pendingPaths}
            onConsumePrefill={() => {
              pendingRef.current = []
              setPendingPaths([])
            }}
            onSubmit={() => setTab('queue')}
          />
        )}
        {tab === 'queue' && <QueueView jobs={jobs} />}
        {tab === 'search' && (
          <div className="search-results">
            {searchHits.length === 0 && (
              <div className="muted">No matches for "{searchQ}"</div>
            )}
            {searchHits.map((h, i) => (
              <div key={i} className="search-hit">
                <div className="search-hit-hdr">
                  <span>{h.job_id}</span>
                  <span className="muted">{h.path}</span>
                </div>
                <div
                  className="search-hit-snip"
                  dangerouslySetInnerHTML={{
                    __html: h.snippet.replace(
                      new RegExp(escapeRegExp(searchQ), 'gi'),
                      (m) => `<mark>${m}</mark>`
                    ),
                  }}
                />
              </div>
            ))}
          </div>
        )}
        {tab === 'plugins' && <PluginsView plugins={plugins} onReload={refresh} />}
        {tab === 'settings' && config && (
          <SettingsView
            config={config}
            onSave={async (c) => {
              const saved = await api.setConfig(c)
              setConfig(saved)
            }}
          />
        )}
      </main>
    </div>
  )
}

function escapeRegExp(s: string): string {
  return s.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
}

export default App
