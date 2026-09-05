import { useState } from 'react'
import { api, Job, StepReport, StepStatus } from '../api'
import { PublishForm } from './PublishForm'
import { AudioPlayer } from './AudioPlayer'

function fmtDuration(job: Job): string {
  if (!job.started) return ''
  const start = new Date(job.started).getTime()
  const end = job.ended && job.ended.startsWith('0001-')
    ? Date.now()
    : new Date(job.ended ?? new Date().toISOString()).getTime()
  return `${((end - start) / 1000).toFixed(1)}s`
}

function fmtStepDuration(s: StepReport): string {
  if (!s.started) return ''
  const start = new Date(s.started).getTime()
  const raw = s.ended && !s.ended.startsWith('0001-') ? s.ended : new Date().toISOString()
  const end = new Date(raw).getTime()
  return `${((end - start) / 1000).toFixed(1)}s`
}

const stepIcon: Record<StepStatus, string> = {
  pending: '○',
  running: '◐',
  completed: '✓',
  failed: '✗',
  skipped: '—',
}

export function QueueView({ jobs }: { jobs: Job[] }) {
  // Expand any job that is currently running by default; users can then
  // collapse. Completed/failed jobs start collapsed.
  const [expanded, setExpanded] = useState<Record<string, boolean>>({})
  const [publishingJob, setPublishingJob] = useState<string | null>(null)

  const isOpen = (j: Job) => {
    const explicit = expanded[j.id]
    if (explicit !== undefined) return explicit
    return j.status === 'running'
  }

  const toggle = (id: string) => setExpanded((cur) => ({ ...cur, [id]: !isOpenFallback(cur, id, jobs) }))

  if (jobs.length === 0) return <div className="empty">No jobs yet.</div>

  return (
    <div className="queue">
      {jobs.map((j) => {
        const open = isOpen(j)
        const totalSteps = j.steps?.length ?? 0
        const doneSteps = j.steps?.filter((s) => s.status === 'completed').length ?? 0
        const inFlight = j.steps?.find((s) => s.status === 'running')
        return (
          <div key={j.id} className={`job job-${j.status}`}>
            <div
              className="job-hdr clickable"
              onClick={() => toggle(j.id)}
              role="button"
              aria-expanded={open}
              aria-label={`Toggle details for job ${j.id}`}
            >
              <span className="disclosure">{open ? '▼' : '▶'}</span>
              <span className={`badge badge-${j.status}`}>{j.status}</span>
              <span className="mono job-id">{j.id}</span>
              <span className="job-summary">
                {totalSteps > 0 && (
                  <>
                    {doneSteps}/{totalSteps} steps
                    {inFlight && <span className="muted"> · now: {inFlight.kind}</span>}
                  </>
                )}
              </span>
              <span className="muted">{fmtDuration(j)}</span>
              {(j.status === 'running' || j.status === 'pending') && (
                <button
                  className="btn small"
                  onClick={(e) => {
                    e.stopPropagation()
                    api.cancelJob(j.id).catch(console.error)
                  }}
                  title="Stop this job"
                >
                  Cancel
                </button>
              )}
              {j.status === 'failed' && (
                <button
                  className="btn small"
                  onClick={(e) => {
                    e.stopPropagation()
                    api.retryJob(j.id).catch((err) => alert(err.message))
                  }}
                  title="Re-run failed steps"
                >
                  Retry
                </button>
              )}
            </div>

            {open && (
              <div className="job-body">
                <div className="job-path">{j.request.path}</div>
                <div className="job-outs">
                  <span className="muted">outputs:</span> {j.request.outputs.join(', ')}
                </div>

                {j.steps && j.steps.length > 0 && (
                  <table className="step-tbl">
                    <tbody>
                      {j.steps.map((s, i) => (
                        <tr key={i} className={`step-row step-${s.status}`}>
                          <td className="step-icon" aria-label={s.status}>
                            {stepIcon[s.status] ?? '?'}
                          </td>
                          <td className="step-kind">{s.kind}</td>
                          <td className="step-msg">
                            {s.status === 'running' && s.message ? (
                              <span className="running-msg">
                                <span className="spinner" aria-hidden /> {s.message}
                              </span>
                            ) : s.status === 'completed' ? (
                              <span className="muted">via {s.plugin ?? '?'}</span>
                            ) : s.status === 'failed' ? (
                              <span className="err">{s.error ?? 'failed'}</span>
                            ) : s.status === 'pending' ? (
                              <span className="muted">waiting…</span>
                            ) : null}
                            {s.chain && s.chain.length > 1 && s.status !== 'failed' && (
                              <span className="muted"> ({s.chain.join(' → ')})</span>
                            )}
                          </td>
                          <td className="step-dur muted">{fmtStepDuration(s)}</td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                )}

                {j.artifacts && j.artifacts.length > 0 && (
                  <>
                    <div className="artifacts-hdr">
                      <span className="muted subhead">Artifacts</span>
                      {(j.status === 'completed' || j.status === 'failed') && (
                        <button
                          className="btn small"
                          onClick={() =>
                            setPublishingJob((cur) => (cur === j.id ? null : j.id))
                          }
                        >
                          {publishingJob === j.id ? 'Cancel' : 'Publish…'}
                        </button>
                      )}
                    </div>

                    {publishingJob === j.id && (
                      <PublishForm
                        jobId={j.id}
                        artifacts={j.artifacts}
                        onDone={() => setPublishingJob(null)}
                        onCancel={() => setPublishingJob(null)}
                      />
                    )}

                    <ul className="artifacts">
                      {j.artifacts.map((a) => (
                        <li key={a.path}>
                          <button
                            className="artifact-link mono"
                            onClick={() => api.openArtifact(a.path).catch(console.error)}
                            title="Open with default app"
                          >
                            {a.path}
                          </button>
                          <button
                            className="artifact-reveal"
                            onClick={() => api.revealArtifact(a.path).catch(console.error)}
                            title="Show in Finder"
                            aria-label="Show in Finder"
                          >
                            ⓘ
                          </button>{' '}
                          <span className="muted">
                            ({a.format} · {(a.bytes / 1024).toFixed(1)} KB · via {a.plugin})
                          </span>
                          {['mp3', 'aiff', 'wav'].includes(a.format) && (
                            <AudioPlayer artifact={a} jobId={j.id} />
                          )}
                          {a.publish && a.publish.length > 0 && (
                            <div className="publish-line">
                              {a.publish.map((p, k) => (
                                <span
                                  key={k}
                                  className={`publish-chip ${p.ok ? 'ok' : 'err'}`}
                                  title={p.error || p.detail}
                                >
                                  {p.ok ? '✓' : '✗'}{' '}
                                  {p.target === 'books' && '📚'}
                                  {p.target === 'music' && '🎵'}
                                  {p.target === 'reading_list' && '📖'}
                                  {p.target === 'obsidian' && '🔮'}
                                  {p.target === 'notion' && '📝'}
                                  {p.target === 'tags' && '🏷'}{' '}
                                  {p.detail}
                                </span>
                              ))}
                            </div>
                          )}
                        </li>
                      ))}
                    </ul>
                  </>
                )}
              </div>
            )}
          </div>
        )
      })}
    </div>
  )
}

// isOpenFallback mirrors isOpen for the toggle callback (which needs the
// current expanded map, not stale state from closure).
function isOpenFallback(cur: Record<string, boolean>, id: string, jobs: Job[]): boolean {
  const explicit = cur[id]
  if (explicit !== undefined) return explicit
  const j = jobs.find((x) => x.id === id)
  return j?.status === 'running'
}
