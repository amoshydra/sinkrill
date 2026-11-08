// Latency probe: measures how many seconds of audio Sonos has buffered but not
// yet played. We know exactly how many bytes we delivered; Sonos reports its
// playback position. Latency ~= deliveredSeconds - position.
import http from 'node:http'
import { spawn } from 'node:child_process'
import { Sonos } from 'sonos'

const FORMAT = process.argv[2] === 'mp3' ? 'mp3' : 'wav'
const SECONDS = Number(process.argv[3] || 25)
const PORT = FORMAT === 'wav' ? 18590 : 18591
const SINK_IP = '192.168.0.188'
const LAN_IP = '192.168.0.29'
const SR = 48000
const CH = 2
const BITS = 16
const BYTES_PER_SEC = FORMAT === 'wav' ? SR * CH * (BITS / 8) : 24000

function wavHeader() {
  const b = Buffer.alloc(44)
  const ba = CH * (BITS / 8)
  b.write('RIFF', 0); b.writeUInt32LE(0xffffffff, 4); b.write('WAVE', 8)
  b.write('fmt ', 12); b.writeUInt32LE(16, 16); b.writeUInt16LE(1, 20)
  b.writeUInt16LE(CH, 22); b.writeUInt32LE(SR, 24); b.writeUInt32LE(SR * ba, 28)
  b.writeUInt16LE(ba, 32); b.writeUInt16LE(BITS, 34); b.write('data', 36)
  b.writeUInt32LE(0xffffffff, 40)
  return b
}

const clients = new Map() // res -> { sent, id }
let nextId = 1

const ffArgs =
  FORMAT === 'wav'
    ? ['-hide_banner', '-loglevel', 'error', '-nostdin', '-f', 'pulse', '-fragment_size', '4800',
       '-i', '@DEFAULT_MONITOR@', '-ac', '2', '-ar', '48000', '-c:a', 'pcm_s16le',
       '-flush_packets', '1', '-avioflags', 'direct', '-f', 's16le', 'pipe:1']
    : ['-hide_banner', '-loglevel', 'error', '-nostdin', '-f', 'pulse', '-fragment_size', '4800',
       '-i', '@DEFAULT_MONITOR@', '-ac', '2', '-c:a', 'libmp3lame', '-b:a', '192k',
       '-compression_level', '0', '-write_xing', '0', '-id3v2_version', '0',
       '-flush_packets', '1', '-avioflags', 'direct', '-f', 'mp3', 'pipe:1']

const ff = spawn('ffmpeg', ffArgs, { stdio: ['ignore', 'pipe', 'pipe'] })
ff.stderr.on('data', d => process.stderr.write(String(d)))
ff.stdout.on('data', chunk => {
  for (const [res, state] of clients) {
    if (res.writableLength > 512 * 1024) continue
    try { res.write(chunk); state.sent += chunk.length } catch { clients.delete(res) }
  }
})

const server = http.createServer((req, res) => {
  if (!(req.url || '').startsWith(FORMAT === 'wav' ? '/stream.wav' : '/stream.mp3')) {
    res.writeHead(404); res.end(); return
  }
  res.useChunkedEncodingByDefault = false
  res.writeHead(200, {
    'Content-Type': FORMAT === 'wav' ? 'audio/wav' : 'audio/mpeg',
    Connection: 'close',
    'Cache-Control': 'no-cache, no-store',
  })
  if (FORMAT === 'wav') res.write(wavHeader())
  res.socket?.setNoDelay(true)
  const state = { sent: 0, id: nextId++ }
  clients.set(res, state)
  console.log(`[conn ${state.id}] connected from ${req.socket.remoteAddress}`)
  req.on('close', () => { clients.delete(res); console.log(`[conn ${state.id}] closed after ${(state.sent / BYTES_PER_SEC).toFixed(1)}s of audio`) })
})
server.on('connection', s => s.setNoDelay(true))
server.requestTimeout = 0
server.timeout = 0
server.listen(PORT, '0.0.0.0')

setTimeout(async () => {
  const speaker = new Sonos(SINK_IP)
  const metadata =
    '<DIDL-Lite xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:upnp="urn:schemas-upnp-org:metadata-1-0/upnp/" xmlns="urn:schemas-upnp-org:metadata-1-0/DIDL-Lite/">' +
    `<item id="-1" parentID="-1" restricted="true"><dc:title>latency probe</dc:title><upnp:class>${FORMAT === 'wav' ? 'object.item.audioItem.musicTrack' : 'object.item.audioItem.audioBroadcast'}</upnp:class></item></DIDL-Lite>`
  const uri = FORMAT === 'wav' ? `http://${LAN_IP}:${PORT}/stream.wav` : `x-rincon-mp3radio://${LAN_IP}:${PORT}/stream.mp3`
  const t0 = Date.now()
  await speaker.setAVTransportURI({ uri, metadata })
  await speaker.play().catch(() => {})
  console.log(`tuned ${FORMAT} -> ${uri}`)

  const samples = []
  let playingAt = null
  for (let i = 0; i < SECONDS * 10; i++) {
    await new Promise(r => setTimeout(r, 100))
    let state
    for (const [res, s] of clients) {
      if (!state || s.sent > state.sent) state = { ...s, writableLength: res.writableLength }
    }
    if (!state) continue
    const pos = await speaker.currentTrack().then(t => t.position).catch(() => null)
    const st = await speaker.getCurrentState().catch(() => '')
    if (st === 'playing' && playingAt === null) playingAt = Date.now() - t0
    if (pos === null || st !== 'playing') continue
    const delivered = (state.sent - state.writableLength) / BYTES_PER_SEC
    samples.push({ delivered, pos, lag: delivered - pos })
  }

  const sorted = [...samples].sort((a, b) => a.lag - b.lag)
  const min = sorted[0]
  const p50 = sorted[Math.floor(sorted.length / 2)]
  const p90 = sorted[Math.floor(sorted.length * 0.9)]
  console.log(`\n=== ${FORMAT.toUpperCase()} ===`)
  console.log(`playback started after: ${playingAt === null ? 'n/a' : playingAt + ' ms'}`)
  console.log(`samples: ${samples.length}`)
  console.log(`lag min: ${min ? min.lag.toFixed(2) + ' s (delivered ' + min.delivered.toFixed(2) + ', pos ' + min.pos + ')' : 'n/a'}`)
  console.log(`lag p50: ${p50 ? p50.lag.toFixed(2) + ' s' : 'n/a'}`)
  console.log(`lag p90: ${p90 ? p90.lag.toFixed(2) + ' s' : 'n/a'}`)
  console.log('estimate: playback latency is between min and min+1 s (position is whole seconds)')
  process.exit(0)
}, 1500)
