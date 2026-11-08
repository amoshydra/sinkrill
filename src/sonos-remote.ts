import { Sonos } from 'sonos'
import { log } from './log.js'

const RADIO_CLASS = 'object.item.audioItem.audioBroadcast'

function escapeXml(value: string): string {
  return value
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&apos;')
}

/**
 * Sonos treats plain `http://` URIs as files. For MP3 we use the
 * `x-rincon-mp3radio://` scheme to force live-stream handling (and avoid
 * UPnP error 714). For WAV we *want* file handling on a plain http URI:
 * Sonos then skips its multi-second internet-radio buffer.
 */
export type StreamFormat = 'mp3' | 'wav'

export function buildStreamUri(lanIp: string, port: number, format: StreamFormat = 'mp3'): string {
  return format === 'wav'
    ? `http://${lanIp}:${port}/stream.wav`
    : `x-rincon-mp3radio://${lanIp}:${port}/stream.mp3`
}

/** DIDL-Lite metadata describing a radio broadcast (no duration, no seeking). */
export function radioMetadata(title: string, artist = 'sonos-sink'): string {
  return (
    '<DIDL-Lite xmlns:dc="http://purl.org/dc/elements/1.1/"' +
    ' xmlns:upnp="urn:schemas-upnp-org:metadata-1-0/upnp/"' +
    ' xmlns:r="urn:schemas-rinconnetworks-com:metadata-1-0/"' +
    ' xmlns="urn:schemas-upnp-org:metadata-1-0/DIDL-Lite/">' +
    '<item id="-1" parentID="-1" restricted="true">' +
    `<dc:title>${escapeXml(title)}</dc:title>` +
    `<dc:creator>${escapeXml(artist)}</dc:creator>` +
    `<upnp:class>${RADIO_CLASS}</upnp:class>` +
    '<desc id="cdudn" nameSpace="urn:schemas-rinconnetworks-com:metadata-1-0/">' +
    'RINCON_AssociatedZPUDN</desc>' +
    '</item></DIDL-Lite>'
  )
}

/** DIDL-Lite metadata for the WAV file-style stream. */
export function wavMetadata(title: string, uri: string, artist = 'sonos-sink'): string {
  return (
    '<DIDL-Lite xmlns:dc="http://purl.org/dc/elements/1.1/"' +
    ' xmlns:upnp="urn:schemas-upnp-org:metadata-1-0/upnp/"' +
    ' xmlns:r="urn:schemas-rinconnetworks-com:metadata-1-0/"' +
    ' xmlns="urn:schemas-upnp-org:metadata-1-0/DIDL-Lite/">' +
    '<item id="-1" parentID="-1" restricted="true">' +
    `<dc:title>${escapeXml(title)}</dc:title>` +
    `<dc:creator>${escapeXml(artist)}</dc:creator>` +
    '<upnp:class>object.item.audioItem.musicTrack</upnp:class>' +
    `<res protocolInfo="http-get:*:audio/wav:*">${escapeXml(uri)}</res>` +
    '<desc id="cdudn" nameSpace="urn:schemas-rinconnetworks-com:metadata-1-0/">' +
    'RINCON_AssociatedZPUDN</desc>' +
    '</item></DIDL-Lite>'
  )
}

export class SonosSpeaker {
  private readonly speaker: Sonos

  constructor(readonly ip: string) {
    this.speaker = new Sonos(ip)
  }

  getVolume(): Promise<number> {
    return this.speaker.getVolume()
  }

  setVolume(volume: number): Promise<void> {
    return this.speaker.setVolume(volume)
  }

  getCurrentState(): Promise<string> {
    return this.speaker.getCurrentState()
  }

  /** Point the speaker at a live stream and start playback. */
  async tuneIn(uri: string, title: string, format: StreamFormat = 'mp3'): Promise<void> {
    const metadata = format === 'wav' ? wavMetadata(title, uri) : radioMetadata(title)
    await this.speaker.setAVTransportURI({ uri, metadata })
    try {
      await this.speaker.play()
    } catch (error) {
      // Some models start radio streams on their own after SetAVTransportURI.
      log.debug(`play() after tune-in failed: ${error instanceof Error ? error.message : String(error)}`)
    }
    log.info(`tuned in ${this.ip} -> ${uri}`)
  }

  async stop(): Promise<void> {
    await this.speaker.stop()
  }
}
