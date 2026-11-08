import { AudioCapture } from './capture.js'
import { BroadcastHub } from './broadcast.js'
import { startStreamServer, type StreamServerInfo } from './server.js'
import { detectLanIp } from './net.js'
import { resolveDevice, type SonosDeviceInfo } from './devices.js'
import { buildStreamUri, SonosSpeaker, type StreamFormat } from './sonos-remote.js'
import { ensureVirtualSink, type VirtualSinkHandle } from './virtual-sink.js'
import { errorMessage } from './util.js'
import { log } from './log.js'

export interface SinkOptions {
  /** Sonos room name or IP. Auto-detected when omitted. */
  device?: string
  /** PulseAudio/PipeWire source to capture. */
  source: string
  /** HTTP port (0 = pick a free one). */
  port: number
  /** MP3 bitrate in kbps. */
  bitrate: number
  /** LAN IP advertised to Sonos. Auto-detected when omitted. */
  host?: string
  /** HTTP bind address. */
  bind: string
  ffmpegPath: string
  /** Re-tune the speaker if it stops on its own. */
  keepalive: boolean
  /** Set the speaker's volume on start. */
  volume?: number
  /** When false, only serve the stream; never touch Sonos. */
  tune: boolean
  stationName: string
  /** `wav` is uncompressed PCM (lowest latency); `mp3` is compressed (~192 kbps). */
  format: StreamFormat
  /** Create/use a virtual null sink with this name and capture its monitor. */
  virtualSink?: string
  /** Recent audio replayed to a newly connected listener, in ms (0 = lowest latency). */
  warmupMs: number
}

const FIRST_DATA_TIMEOUT_MS = 8000
const KEEPALIVE_INTERVAL_MS = 10_000
const KEEPALIVE_COOLDOWN_MS = 20_000

function waitForFirstAudio(capture: AudioCapture): Promise<void> {
  return new Promise<void>((resolve, reject) => {
    const onData = () => {
      cleanup()
      resolve()
    }
    const onExit = () => {
      cleanup()
      reject(new Error('ffmpeg exited before producing any audio'))
    }
    const onFatal = (message: string) => {
      cleanup()
      reject(new Error(message))
    }
    const timer = setTimeout(() => {
      cleanup()
      reject(
        new Error(
          'No audio data arrived from the source. Check that the PulseAudio/PipeWire ' +
            'source exists (try `pactl list short sources`) or pass --source.',
        ),
      )
    }, FIRST_DATA_TIMEOUT_MS)

    const cleanup = () => {
      clearTimeout(timer)
      capture.off('data', onData)
      capture.off('exit', onExit)
      capture.off('fatal', onFatal)
    }

    capture.once('data', onData)
    capture.once('exit', onExit)
    capture.once('fatal', onFatal)
  })
}

/**
 * Capture the computer's audio and serve it to a Sonos speaker as a live
 * radio stream. Resolves once the process is shut down with Ctrl+C/SIGTERM.
 */
export async function runSink(options: SinkOptions): Promise<void> {
  const lanIp = options.host || detectLanIp()
  if (!lanIp) {
    throw new Error('Could not detect a LAN IPv4 address. Pass --host <ip>.')
  }

  const device: SonosDeviceInfo | null = options.tune ? await resolveDevice(options.device) : null
  const speaker = device ? new SonosSpeaker(device.ip) : null

  // With --virtual-sink, capture apps routed to a dedicated output device
  // instead of whatever is playing on the default output.
  let virtualSink: VirtualSinkHandle | null = null
  let source = options.source
  if (options.virtualSink) {
    virtualSink = await ensureVirtualSink(options.virtualSink)
    source = virtualSink.monitorSource
    if (virtualSink.created) {
      log.info(`created output device "${options.virtualSink}" (select it in your audio selector)`)
    } else {
      log.info(`using existing output device "${options.virtualSink}"`)
    }
  }

  const hub = new BroadcastHub({
    // WAV is 48 kHz/16-bit stereo: ~1536 kbps.
    bitrateKbps: options.format === 'wav' ? 1536 : options.bitrate,
    warmupMs: options.warmupMs,
  })
  const capture = new AudioCapture({
    source,
    bitrate: options.bitrate,
    ffmpegPath: options.ffmpegPath,
    format: options.format === 'wav' ? 'pcm' : 'mp3',
  })

  capture.on('data', (chunk: Buffer) => hub.write(chunk))

  let server: StreamServerInfo | null = null
  let uri = ''
  let tuned = false
  let shuttingDown = false
  let keepaliveTimer: NodeJS.Timeout | null = null
  let lastRetuneAt = 0
  let resolveDone!: () => void
  const done = new Promise<void>(resolve => {
    resolveDone = resolve
  })

  const cleanup = async (): Promise<void> => {
    if (shuttingDown) return
    shuttingDown = true

    if (keepaliveTimer) {
      clearInterval(keepaliveTimer)
      keepaliveTimer = null
    }

    if (speaker && tuned) {
      try {
        await speaker.stop()
        log.info(`stopped playback on ${device?.name} (${device?.ip})`)
      } catch (error) {
        log.debug(`could not stop speaker: ${errorMessage(error)}`)
      }
    }

    await capture.stop()
    await server?.close()
    await virtualSink?.dispose()
  }

  const shutdown = async (signal?: string): Promise<void> => {
    if (shuttingDown) return
    log.info(signal ? `received ${signal}, shutting down` : 'shutting down')
    await cleanup()
    resolveDone()
  }

  const signals: NodeJS.Signals[] = ['SIGINT', 'SIGTERM', 'SIGHUP']
  for (const signal of signals) {
    process.once(signal, () => {
      void shutdown(signal)
    })
  }

  try {
    const firstAudio = waitForFirstAudio(capture)
    capture.start()

    server = await startStreamServer({
      port: options.port,
      bind: options.bind,
      hub,
      bitrate: options.bitrate,
      format: options.format,
      stationName: options.stationName,
      status: () => ({
        station: options.stationName,
        source,
        format: options.format,
        bitrate: options.format === 'wav' ? 1536 : options.bitrate,
        uri,
        device: device ? { name: device.name, ip: device.ip } : null,
        virtualSink: virtualSink
          ? { name: virtualSink.name, createdByUs: virtualSink.created }
          : null,
        warmupMs: options.warmupMs,
        capture: {
          running: capture.running,
          restarts: capture.restartCount,
          uptimeMs: capture.uptimeMs,
        },
      }),
    })

    await firstAudio
    uri = buildStreamUri(lanIp, server.port, options.format)

    log.info(`advertised URI (${options.format}): ${uri}`)
    log.info(`preview locally: http://${lanIp}:${server.port}/${options.format === 'wav' ? 'stream.wav' : 'stream.mp3'}`)

    if (speaker && device) {
      if (options.volume !== undefined) {
        await speaker.setVolume(options.volume)
        log.info(`set volume on ${device.name} to ${options.volume}`)
      }
      await speaker.tuneIn(uri, options.stationName, options.format)
      tuned = true
      log.info(`streaming PC audio to ${device.name} (${device.ip})`)
    } else {
      log.info('running without tuning Sonos (--no-tune). Point a speaker at the URI above.')
    }

    // If ffmpeg crashes and restarts, the speaker has lost the stream. Retune.
    capture.on('restarted', () => {
      if (shuttingDown || !speaker) return
      log.info('capture restarted; retuning speaker')
      speaker.tuneIn(uri, options.stationName, options.format).catch(error => {
        log.warn(`retune failed: ${errorMessage(error)}`)
      })
    })

    capture.on('fatal', message => {
      log.error(message)
      process.exitCode = 1
      void shutdown()
    })

    if (options.keepalive && speaker) {
      keepaliveTimer = setInterval(async () => {
        if (shuttingDown) return
        try {
          const state = await speaker.getCurrentState()
          if (state.toUpperCase() !== 'STOPPED') return
          if (Date.now() - lastRetuneAt < KEEPALIVE_COOLDOWN_MS) return
          lastRetuneAt = Date.now()
          log.info('speaker stopped unexpectedly; retuning (keepalive)')
          await speaker.tuneIn(uri, options.stationName, options.format)
        } catch (error) {
          log.debug(`keepalive check failed: ${errorMessage(error)}`)
        }
      }, KEEPALIVE_INTERVAL_MS)
    }

    log.info('press Ctrl+C to stop')
    await done
  } catch (error) {
    await cleanup()
    for (const signal of signals) process.removeAllListeners(signal)
    throw error
  }
}
