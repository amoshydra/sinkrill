import http from 'node:http'
import type { AddressInfo } from 'node:net'
import type { BroadcastHub } from './broadcast.js'
import { PCM_BITS, PCM_CHANNELS, PCM_SAMPLE_RATE } from './capture.js'
import { log } from './log.js'

export interface StreamServerOptions {
  port: number
  bind: string
  hub: BroadcastHub
  bitrate: number
  /** `wav` serves raw PCM with a WAV header; `mp3` serves the encoded stream. */
  format: 'mp3' | 'wav'
  stationName: string
  /** Extra fields exposed on /status. */
  status: () => Record<string, unknown>
}

export interface StreamServerInfo {
  /** The port actually bound (useful when asked for port 0). */
  port: number
  close: () => Promise<void>
}

function sendJson(res: http.ServerResponse, status: number, body: unknown): void {
  const payload = JSON.stringify(body, null, 2)
  res.writeHead(status, {
    'Content-Type': 'application/json',
    'Content-Length': Buffer.byteLength(payload),
    'Cache-Control': 'no-store',
  })
  res.end(payload)
}

/**
 * 44-byte PCM WAV header with an "unknown" length. Sonos happily plays this
 * as a file stream, which avoids the multi-second internet-radio buffer.
 */
function wavHeader(): Buffer {
  const bytesPerSample = PCM_BITS / 8
  const blockAlign = PCM_CHANNELS * bytesPerSample
  const byteRate = PCM_SAMPLE_RATE * blockAlign
  const header = Buffer.alloc(44)
  header.write('RIFF', 0)
  header.writeUInt32LE(0xffffffff, 4)
  header.write('WAVE', 8)
  header.write('fmt ', 12)
  header.writeUInt32LE(16, 16)
  header.writeUInt16LE(1, 20) // PCM
  header.writeUInt16LE(PCM_CHANNELS, 22)
  header.writeUInt32LE(PCM_SAMPLE_RATE, 24)
  header.writeUInt32LE(byteRate, 28)
  header.writeUInt16LE(blockAlign, 32)
  header.writeUInt16LE(PCM_BITS, 34)
  header.write('data', 36)
  header.writeUInt32LE(0xffffffff, 40)
  return header
}

/**
 * Serve the live MP3 stream. Uses Icecast-style framing (no Content-Length,
 * connection-close instead of chunked encoding) which Sonos handles reliably.
 */
export async function startStreamServer(options: StreamServerOptions): Promise<StreamServerInfo> {
  const { hub } = options

  const server = http.createServer((req, res) => {
    try {
      const url = new URL(req.url || '/', 'http://localhost')
      const path = url.pathname

      const isWav = options.format === 'wav'
      const streamPath = isWav ? '/stream.wav' : '/stream.mp3'

      if (path === streamPath || path === '/stream' || path === '/') {
        if (req.method !== 'GET' && req.method !== 'HEAD') {
          res.writeHead(405, { Allow: 'GET, HEAD' })
          res.end()
          return
        }

        const streamBitrate = isWav
          ? (PCM_SAMPLE_RATE * PCM_CHANNELS * PCM_BITS) / 1000
          : options.bitrate

        // Sonos dislikes chunked transfer encoding on radio streams.
        res.useChunkedEncodingByDefault = false
        res.writeHead(200, {
          'Content-Type': isWav ? 'audio/wav' : 'audio/mpeg',
          'Cache-Control': 'no-cache, no-store, must-revalidate',
          Pragma: 'no-cache',
          Connection: 'close',
          'icy-name': options.stationName,
          'icy-br': String(streamBitrate),
          'icy-pub': '0',
          'Access-Control-Allow-Origin': '*',
        })

        if (req.method === 'HEAD') {
          res.end()
          return
        }

        // Send headers now; don't wait for the first audio chunk.
        res.flushHeaders()
        // MP3/PCM packets are small: Nagle would add tens of ms per write.
        res.socket?.setNoDelay(true)

        if (isWav) res.write(wavHeader())

        const remote = req.socket.remoteAddress || 'unknown'
        const id = hub.add({
          label: `stream client ${remote}`,
          write: chunk => res.write(chunk),
          end: () => res.end(),
          buffered: () => res.writableLength,
        })

        const cleanup = () => hub.remove(id)
        req.on('close', cleanup)
        res.on('close', cleanup)
        res.on('error', cleanup)
        return
      }

      if (path === '/status') {
        sendJson(res, 200, {
          ...options.status(),
          listeners: hub.clientCount,
          listenerBytes: hub.totalBytes,
        })
        return
      }

      if (path === '/health') {
        sendJson(res, 200, { ok: true })
        return
      }

      res.writeHead(404, { 'Content-Type': 'text/plain' })
      res.end('Not found\n')
    } catch (error) {
      log.error(`HTTP handler error: ${error instanceof Error ? error.message : String(error)}`)
      if (!res.headersSent) res.writeHead(500)
      res.end()
    }
  })

  // Live streams are long-lived; disable Node's built-in request/socket timeouts.
  server.requestTimeout = 0
  server.timeout = 0
  server.headersTimeout = 0

  // Low latency for the audio path: never coalesce small MP3 packets.
  server.on('connection', socket => {
    socket.setNoDelay(true)
  })

  await new Promise<void>((resolve, reject) => {
    const onError = (error: Error) => reject(error)
    server.once('error', onError)
    server.listen(options.port, options.bind, () => {
      server.off('error', onError)
      resolve()
    })
  })

  const address = server.address() as AddressInfo
  const streamPath = options.format === 'wav' ? '/stream.wav' : '/stream.mp3'
  log.info(`stream server listening on http://${options.bind}:${address.port}${streamPath}`)

  const close = async (): Promise<void> => {
    hub.endAll()
    await new Promise<void>(resolve => {
      const force = setTimeout(() => {
        server.closeAllConnections()
        resolve()
      }, 3000)
      server.close(() => {
        clearTimeout(force)
        resolve()
      })
    })
  }

  return { port: address.port, close }
}
