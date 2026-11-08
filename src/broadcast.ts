import { EventEmitter } from 'node:events'
import { log } from './log.js'

export interface HubListener {
  label: string
  write: (chunk: Buffer) => void
  end: () => void
  /** Bytes queued in the listener's socket buffer. */
  buffered: () => number
}

export interface BroadcastHubOptions {
  /** How much recent audio a freshly connected listener receives so it can start instantly. */
  warmupMs?: number
  /** Bitrate of the stream, used to size the warm-up buffer. */
  bitrateKbps?: number
}

/** Drop chunks for a listener that is this far behind (live audio: skipping is correct). */
const MAX_BUFFERED_BYTES = 512 * 1024

/**
 * Fans one MP3 source out to any number of HTTP listeners.
 *
 * With `warmupMs > 0`, the last `warmupMs` of audio is replayed to a newly
 * connected listener so it can start instantly (at the cost of that much
 * extra latency). At `0` listeners start at the current position, which is
 * what you want for A/V sync.
 */
export class BroadcastHub extends EventEmitter {
  private ring: Buffer[] = []
  private ringBytes = 0
  private readonly ringLimit: number
  private readonly clients = new Map<number, HubListener>()
  private nextId = 1

  totalBytes = 0

  constructor(options: BroadcastHubOptions = {}) {
    super()
    const warmupMs = options.warmupMs ?? 0
    const bitrateKbps = options.bitrateKbps ?? 192
    this.ringLimit =
      warmupMs > 0 ? Math.max(4096, Math.round((bitrateKbps * 1000) / 8 * (warmupMs / 1000))) : 0
  }

  get clientCount(): number {
    return this.clients.size
  }

  /** Feed encoded audio from the capture process. */
  write(chunk: Buffer): void {
    this.totalBytes += chunk.length

    if (this.ringLimit > 0) {
      this.ring.push(chunk)
      this.ringBytes += chunk.length
      while (this.ringBytes > this.ringLimit && this.ring.length > 1) {
        const dropped = this.ring.shift()
        if (dropped) this.ringBytes -= dropped.length
      }
    }

    for (const [id, listener] of this.clients) {
      if (listener.buffered() > MAX_BUFFERED_BYTES) {
        log.debug(`listener ${id} is behind; dropping ${chunk.length} bytes`)
        continue
      }
      try {
        listener.write(chunk)
      } catch {
        this.remove(id)
      }
    }
  }

  /** Register a listener and replay the warm-up buffer to it (if enabled). */
  add(listener: HubListener): number {
    const id = this.nextId++
    this.clients.set(id, listener)

    if (this.ringLimit > 0) {
      for (const chunk of this.ring) {
        try {
          listener.write(chunk)
        } catch {
          this.clients.delete(id)
          return id
        }
      }
    }

    log.info(`${listener.label} listening (${this.clients.size} active)`)
    this.emit('listener-change', this.clients.size)
    return id
  }

  remove(id: number): void {
    if (!this.clients.delete(id)) return
    log.info(`listener ${id} disconnected (${this.clients.size} active)`)
    this.emit('listener-change', this.clients.size)
  }

  /** Close every listener connection (used on shutdown). */
  endAll(): void {
    for (const [id, listener] of this.clients) {
      try {
        listener.end()
      } catch {
        /* best effort */
      }
      this.clients.delete(id)
    }
  }
}
