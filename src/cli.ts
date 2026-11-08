#!/usr/bin/env node
import { Command } from 'commander'
import { readFileSync } from 'node:fs'
import path from 'node:path'
import { fileURLToPath } from 'node:url'
import { runSink } from './sink.js'
import { listDevices, resolveDevice } from './devices.js'
import { SonosSpeaker } from './sonos-remote.js'
import { errorMessage } from './util.js'
import { log, setVerbose } from './log.js'

function readVersion(): string {
  try {
    const packagePath = path.join(path.dirname(fileURLToPath(import.meta.url)), '..', 'package.json')
    const parsed = JSON.parse(readFileSync(packagePath, 'utf8')) as { version?: string }
    return parsed.version || '0.0.0'
  } catch {
    return '0.0.0'
  }
}

const VERSION = readVersion()

interface StartOptions {
  device?: string
  source: string
  port: string
  bitrate: string
  format: string
  host?: string
  bind: string
  ffmpeg: string
  volume?: string
  keepalive: boolean
  tune: boolean
  station: string
  warmup: string
  virtualSink?: string | boolean
  verbose?: boolean
}

function parseInteger(value: unknown, label: string, min: number, max: number): number {
  const parsed = Number.parseInt(String(value), 10)
  if (!Number.isInteger(parsed) || parsed < min || parsed > max) {
    throw new Error(`${label} must be an integer between ${min} and ${max}`)
  }
  return parsed
}

const program = new Command()

program
  .name('sonos-sink')
  .description("Stream this computer's audio output to Sonos speakers over your LAN")
  .version(VERSION)
  .showHelpAfterError()
  .addHelpText(
    'after',
    `
Examples:
  $ sonos-sink                          # auto-detect the only Sonos speaker
  $ sonos-sink "Living Room"            # pick a room by name
  $ sonos-sink --device 192.168.1.42    # pick a speaker by IP
  $ sonos-sink --source alsa_output.usb-....monitor
  $ sonos-sink --no-tune                # only serve; tune Sonos manually
  $ sonos-sink devices                  # list speakers on the LAN
  $ sonos-sink stop "Living Room"       # stop playback
`,
  )

program
  .command('start')
  .description('Capture audio and stream it to a Sonos speaker')
  .option('-d, --device <name|ip>', 'Sonos room name or IP (auto-detected if there is only one)')
  .option('-s, --source <source>', 'PulseAudio/PipeWire source to capture', '@DEFAULT_MONITOR@')
  .option('-p, --port <port>', 'HTTP port for the stream (0 picks a free port)', '8567')
  .option('-b, --bitrate <kbps>', 'MP3 bitrate in kbps', '192')
  .option(
    '-f, --format <format>',
    'Stream format: wav (lowest latency, ~1.5 Mbps) or mp3',
    'wav',
  )
  .option(
    '--warmup <ms>',
    'Replay this much recent audio to a new listener (0 = lowest latency)',
    '0',
  )
  .option('--host <ip>', 'LAN IP to advertise to Sonos (auto-detected)')
  .option('--bind <address>', 'HTTP bind address', '0.0.0.0')
  .option('--ffmpeg <path>', 'ffmpeg binary', 'ffmpeg')
  .option('--volume <level>', 'Set the Sonos volume (0-100) on start')
  .option('--keepalive', 'Re-tune Sonos if it stops by itself (can fight a manual stop)')
  .option('--no-tune', 'Only serve the stream; do not control Sonos')
  .option(
    '--virtual-sink [name]',
    'Create/use a virtual output device (default: sonos) and capture it',
  )
  .option('--station <name>', 'Station name shown on the speaker', 'PC Audio')
  .option('-v, --verbose', 'Verbose logging')
  .action(async (raw: StartOptions) => {
    setVerbose(Boolean(raw.verbose))

    const port = parseInteger(raw.port, 'port', 0, 65535)
    const bitrate = parseInteger(raw.bitrate, 'bitrate', 32, 320)
    const warmupMs = parseInteger(raw.warmup, 'warmup', 0, 5000)
    if (raw.format !== 'mp3' && raw.format !== 'wav') {
      throw new Error('format must be "wav" or "mp3"')
    }
    const volume = raw.volume === undefined ? undefined : parseInteger(raw.volume, 'volume', 0, 100)

    let virtualSink: string | undefined
    if (typeof raw.virtualSink === 'string') virtualSink = raw.virtualSink
    else if (raw.virtualSink === true) virtualSink = 'sonos'

    await runSink({
      device: raw.device,
      source: raw.source,
      port,
      bitrate,
      host: raw.host,
      bind: raw.bind,
      ffmpegPath: raw.ffmpeg,
      keepalive: Boolean(raw.keepalive),
      volume,
      tune: raw.tune !== false,
      stationName: raw.station,
      format: raw.format,
      virtualSink,
      warmupMs,
    })
  })

program
  .command('devices')
  .description('List Sonos devices found on the LAN')
  .option('--timeout <ms>', 'Discovery timeout in milliseconds', '3000')
  .option('--json', 'Output JSON')
  .action(async (raw: { timeout: string; json?: boolean }) => {
    const timeout = parseInteger(raw.timeout, 'timeout', 500, 30_000)
    const devices = await listDevices(timeout)

    if (devices.length === 0) {
      log.error('No Sonos devices found. Check that the speakers are powered on and on the same LAN.')
      process.exitCode = 1
      return
    }

    if (raw.json) {
      console.log(JSON.stringify(devices, null, 2))
      return
    }

    const nameWidth = Math.max(4, ...devices.map(device => device.name.length))
    for (const device of devices) {
      console.log(`${device.name.padEnd(nameWidth)}  ${device.ip.padEnd(15)}  ${device.model}`)
    }
  })

program
  .command('stop')
  .description('Stop playback on a Sonos device')
  .option('-d, --device <name|ip>', 'Sonos room name or IP (auto-detected if there is only one)')
  .action(async (raw: { device?: string }) => {
    const device = await resolveDevice(raw.device)
    const speaker = new SonosSpeaker(device.ip)
    await speaker.stop()
    log.info(`stopped ${device.name} (${device.ip})`)
  })

/**
 * Allow `sonos-sink "Kitchen"` and bare `sonos-sink` as shorthand for `start`.
 */
function normalizeArgv(argv: string[]): string[] {
  const commands = new Set(['start', 'devices', 'stop', 'help'])
  if (argv.length === 0) return ['start']

  const first = argv[0]!
  if (commands.has(first)) return argv
  // Leave top-level --help/--version alone so they show the command list.
  if (['-h', '--help', '-V', '--version'].includes(first)) return argv
  if (first.startsWith('-')) return ['start', ...argv]
  return ['start', '--device', first, ...argv.slice(1)]
}

async function main(): Promise<void> {
  try {
    await program.parseAsync(normalizeArgv(process.argv.slice(2)), { from: 'user' })
  } catch (error) {
    log.error(errorMessage(error))
    process.exitCode = 1
  }
}

void main()
