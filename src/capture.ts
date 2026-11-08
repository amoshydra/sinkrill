import { spawn, type ChildProcess } from 'node:child_process'
import { EventEmitter } from 'node:events'
import { log } from './log.js'

export interface CaptureOptions {
  /** PulseAudio/PipeWire source to record. `@DEFAULT_MONITOR@` captures the default output. */
  source: string
  /** MP3 bitrate in kbps (ignored for PCM). */
  bitrate: number
  ffmpegPath: string
  /** `pcm` produces raw 48 kHz/16-bit stereo for a WAV stream; `mp3` encodes. */
  format?: 'mp3' | 'pcm'
  /** Delay before restarting ffmpeg after an unexpected exit. */
  restartDelayMs?: number
}

export const PCM_SAMPLE_RATE = 48000
export const PCM_CHANNELS = 2
export const PCM_BITS = 16

/**
 * Runs ffmpeg in the background, converting a live PulseAudio/PipeWire source
 * to a continuous MP3 stream on stdout.
 *
 * Events:
 *  - `data`      (chunk: Buffer)  encoded MP3 bytes
 *  - `exit`      (info)           ffmpeg exited unexpectedly
 *  - `restarted` ()               a new ffmpeg process started after a crash
 *  - `fatal`     (message: string) unrecoverable problem (e.g. ffmpeg not installed)
 */
export class AudioCapture extends EventEmitter {
  readonly options: Required<CaptureOptions>
  private child: ChildProcess | null = null
  private stopping = false
  private fatal = false
  private restartTimer: NodeJS.Timeout | null = null
  private startedAt = 0
  private lastStderr = ''
  private producedData = false

  restartCount = 0
  running = false

  constructor(options: CaptureOptions) {
    super()
    this.options = {
      source: options.source,
      bitrate: options.bitrate,
      ffmpegPath: options.ffmpegPath,
      format: options.format ?? 'mp3',
      restartDelayMs: options.restartDelayMs ?? 2000,
    }
  }

  get uptimeMs(): number {
    return this.running ? Date.now() - this.startedAt : 0
  }

  start(): void {
    if (this.child) return
    this.stopping = false
    this.spawnFfmpeg(false)
  }

  private ffmpegArgs(): string[] {
    const { source, bitrate, format } = this.options
    const input = [
      '-hide_banner',
      '-loglevel',
      'warning',
      '-nostdin',
      // Live capture from PulseAudio (PipeWire's pulse server also speaks this).
      // A small fragment size keeps the capture buffer short (~12 ms).
      '-f',
      'pulse',
      '-fragment_size',
      '2400',
      '-i',
      source,
      '-ac',
      String(PCM_CHANNELS),
    ]

    if (format === 'pcm') {
      // Uncompressed 48 kHz/16-bit stereo: no encoder buffer, and Sonos does
      // not apply radio-style pre-buffering to a WAV stream.
      return [
        ...input,
        '-ar',
        String(PCM_SAMPLE_RATE),
        '-c:a',
        'pcm_s16le',
        '-flush_packets',
        '1',
        '-avioflags',
        'direct',
        '-f',
        's16le',
        'pipe:1',
      ]
    }

    return [
      ...input,
      // Encode at the source rate: no resampler in the path.
      '-c:a',
      'libmp3lame',
      '-b:a',
      `${bitrate}k`,
      '-compression_level',
      '0',
      // These matter for live audio: no Xing/ID3 header to compute, and every
      // packet is flushed straight to the pipe instead of sitting in AVIO.
      '-write_xing',
      '0',
      '-id3v2_version',
      '0',
      '-flush_packets',
      '1',
      '-avioflags',
      'direct',
      '-f',
      'mp3',
      'pipe:1',
    ]
  }

  private spawnFfmpeg(isRestart: boolean): void {
    const { ffmpegPath, source, bitrate, format } = this.options
    if (!isRestart) {
      const target =
        format === 'pcm'
          ? `PCM ${PCM_SAMPLE_RATE} Hz / ${PCM_BITS}-bit stereo`
          : `MP3 ${bitrate} kbps`
      log.info(`capturing ${source} -> ${target} via ${ffmpegPath}`)
    }

    const child = spawn(ffmpegPath, this.ffmpegArgs(), {
      stdio: ['ignore', 'pipe', 'pipe'],
    })

    this.child = child
    this.running = true
    this.startedAt = Date.now()
    this.lastStderr = ''
    this.emit(isRestart ? 'restarted' : 'started')

    child.stdout?.on('data', (chunk: Buffer) => {
      this.producedData = true
      this.emit('data', chunk)
    })

    child.stderr?.on('data', (chunk: Buffer) => {
      this.lastStderr = (this.lastStderr + chunk.toString()).slice(-4000)
      for (const line of chunk.toString().split('\n')) {
        if (line.trim()) log.debug(`ffmpeg: ${line.trim()}`)
      }
    })

    child.on('error', error => {
      const code = (error as NodeJS.ErrnoException).code
      if (code === 'ENOENT') {
        this.fatal = true
        const message = `ffmpeg not found at "${ffmpegPath}". Install ffmpeg or pass --ffmpeg <path>.`
        log.error(message)
        this.emit('fatal', message)
      } else {
        log.error(`ffmpeg process error: ${error.message}`)
      }
    })

    child.on('close', (code, signal) => {
      this.running = false
      this.child = null
      if (this.stopping) {
        this.emit('stopped')
        return
      }

      const details = this.lastStderr.trim().split('\n').slice(-3).join(' | ')
      log.warn(
        `ffmpeg exited (code=${code ?? 'null'}, signal=${signal ?? 'none'})` +
          (details ? `: ${details}` : ''),
      )
      this.emit('exit', { code, signal })

      // Before the first byte, startup code handles the failure and exits.
      if (!this.fatal && this.producedData) this.scheduleRestart()
    })
  }

  private scheduleRestart(): void {
    if (this.restartTimer || this.stopping) return
    // If it died immediately something is wrong with the source; back off a bit.
    const uptime = this.startedAt ? Date.now() - this.startedAt : 0
    const delay = uptime < 1500 ? Math.max(this.options.restartDelayMs, 5000) : this.options.restartDelayMs
    log.info(`restarting capture in ${delay}ms`)
    this.restartTimer = setTimeout(() => {
      this.restartTimer = null
      if (this.stopping) return
      this.restartCount += 1
      this.spawnFfmpeg(true)
    }, delay)
  }

  async stop(): Promise<void> {
    this.stopping = true
    if (this.restartTimer) {
      clearTimeout(this.restartTimer)
      this.restartTimer = null
    }

    const child = this.child
    if (!child) return

    await new Promise<void>(resolve => {
      const force = setTimeout(() => {
        child.kill('SIGKILL')
        resolve()
      }, 2000)

      child.once('close', () => {
        clearTimeout(force)
        resolve()
      })

      child.kill('SIGTERM')
    })
  }
}
