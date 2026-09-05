import { useEffect, useRef, useState } from 'react'
import { Artifact } from '../api'

interface Props {
  artifact: Artifact
  jobId: string
}

// AudioPlayer renders inline controls + a bar-graph waveform for audio
// artifacts. Uses HTML5 <audio> for playback and WebAudio's
// AnalyserNode + Canvas for the visualization. Handles play/pause, seek,
// volume, and the browser's built-in media keys via the MediaSession API.
export function AudioPlayer({ artifact, jobId }: Props) {
  const audioRef = useRef<HTMLAudioElement | null>(null)
  const canvasRef = useRef<HTMLCanvasElement | null>(null)
  const rafRef = useRef<number | null>(null)
  const [playing, setPlaying] = useState(false)
  const [duration, setDuration] = useState(0)
  const [current, setCurrent] = useState(0)

  const src = `http://127.0.0.1:8771/jobs/${encodeURIComponent(jobId)}/artifacts/${encodeURIComponent(basename(artifact.path))}`

  useEffect(() => {
    if (!audioRef.current) return

    // MediaSession — surfaces play/pause/next-track for media keys and
    // Now Playing widget.
    if ('mediaSession' in navigator) {
      const title = basename(artifact.path).replace(/\.[^.]+$/, '')
      navigator.mediaSession.metadata = new MediaMetadata({
        title,
        artist: 'Mac Transformer',
        album: title.split('-')[0] ?? title,
      })
      navigator.mediaSession.setActionHandler('play', () => audioRef.current?.play())
      navigator.mediaSession.setActionHandler('pause', () => audioRef.current?.pause())
      navigator.mediaSession.setActionHandler('seekbackward', (e) => {
        if (audioRef.current) audioRef.current.currentTime -= e.seekOffset ?? 10
      })
      navigator.mediaSession.setActionHandler('seekforward', (e) => {
        if (audioRef.current) audioRef.current.currentTime += e.seekOffset ?? 10
      })
    }
  }, [artifact.path])

  useEffect(() => {
    // Waveform. Uses AnalyserNode → freq domain → bar chart. Lightweight —
    // no offline analysis, just what's happening at the play cursor.
    if (!audioRef.current || !canvasRef.current) return
    const ctx = new AudioContext()
    let src: MediaElementAudioSourceNode
    try {
      src = ctx.createMediaElementSource(audioRef.current)
    } catch {
      return // some browsers block cross-instance connect
    }
    const analyser = ctx.createAnalyser()
    analyser.fftSize = 128
    src.connect(analyser).connect(ctx.destination)

    const data = new Uint8Array(analyser.frequencyBinCount)
    const canvas = canvasRef.current
    const g = canvas.getContext('2d')!

    const draw = () => {
      analyser.getByteFrequencyData(data)
      const w = canvas.width
      const h = canvas.height
      g.clearRect(0, 0, w, h)
      const barW = w / data.length
      for (let i = 0; i < data.length; i++) {
        const barH = (data[i] / 255) * h
        g.fillStyle = 'rgba(94,157,255,0.8)'
        g.fillRect(i * barW, h - barH, barW - 1, barH)
      }
      rafRef.current = requestAnimationFrame(draw)
    }
    draw()

    return () => {
      if (rafRef.current !== null) cancelAnimationFrame(rafRef.current)
      ctx.close().catch(() => {})
    }
  }, [src])

  const fmt = (t: number) => {
    if (!isFinite(t)) return '0:00'
    const m = Math.floor(t / 60)
    const s = Math.floor(t % 60)
    return `${m}:${s < 10 ? '0' : ''}${s}`
  }

  return (
    <div className="audio-player">
      <audio
        ref={audioRef}
        src={src}
        onPlay={() => setPlaying(true)}
        onPause={() => setPlaying(false)}
        onEnded={() => setPlaying(false)}
        onLoadedMetadata={(e) => setDuration((e.target as HTMLAudioElement).duration)}
        onTimeUpdate={(e) => setCurrent((e.target as HTMLAudioElement).currentTime)}
        preload="metadata"
      />
      <button
        className="btn small"
        onClick={() => (playing ? audioRef.current?.pause() : audioRef.current?.play())}
      >
        {playing ? '❚❚' : '▶'}
      </button>
      <input
        type="range"
        min={0}
        max={duration || 0}
        step={0.1}
        value={current}
        onChange={(e) => {
          if (audioRef.current) audioRef.current.currentTime = parseFloat(e.target.value)
        }}
        className="audio-scrub"
      />
      <span className="audio-time muted mono">
        {fmt(current)} / {fmt(duration)}
      </span>
      <canvas ref={canvasRef} width={200} height={32} className="audio-canvas" />
    </div>
  )
}

function basename(p: string): string {
  const i = p.lastIndexOf('/')
  return i >= 0 ? p.slice(i + 1) : p
}
