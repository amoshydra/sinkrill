import net from 'node:net'
import { Sonos } from 'sonos'
import { discoverSonosIps, scanSonosIps } from './net.js'
import { withTimeout } from './util.js'
import { log } from './log.js'

export interface SonosDeviceInfo {
  ip: string
  name: string
  model: string
  uuid: string
}

const DESCRIPTION_TIMEOUT_MS = 4000

async function describe(ip: string): Promise<SonosDeviceInfo | null> {
  try {
    const speaker = new Sonos(ip)
    const description = await withTimeout(
      speaker.deviceDescription(),
      DESCRIPTION_TIMEOUT_MS,
      `device description for ${ip}`,
    )
    return {
      ip,
      name: description.roomName || description.displayName || ip,
      model: description.modelName || '',
      uuid: description.UUID || '',
    }
  } catch (error) {
    log.debug(`could not describe ${ip}: ${error instanceof Error ? error.message : String(error)}`)
    return null
  }
}

/** Discover every Sonos device on the LAN, sorted by room name. */
export async function listDevices(timeoutMs = 3000): Promise<SonosDeviceInfo[]> {
  let ips = await discoverSonosIps(timeoutMs)
  log.debug(`SSDP found ${ips.length} candidate(s): ${ips.join(', ') || 'none'}`)

  if (ips.length === 0) {
    log.info('SSDP found nothing; scanning the local network on port 1400...')
    ips = await scanSonosIps()
    log.debug(`scan found ${ips.length} candidate(s): ${ips.join(', ') || 'none'}`)
  }

  const described = await Promise.all(ips.map(describe))
  return described
    .filter((device): device is SonosDeviceInfo => device !== null)
    .sort((a, b) => a.name.localeCompare(b.name))
}

/**
 * Resolve a `--device` selector (room name, partial name, IP, or nothing).
 * With no selector, requires exactly one discovered device.
 */
export async function resolveDevice(selector?: string): Promise<SonosDeviceInfo> {
  if (selector && net.isIP(selector) === 4) {
    const described = await describe(selector)
    return described ?? { ip: selector, name: selector, model: '', uuid: '' }
  }

  log.info('Searching for Sonos devices...')
  const devices = await listDevices()

  if (devices.length === 0) {
    throw new Error(
      'No Sonos devices found. Make sure this computer is on the same LAN as the speakers, ' +
        'or pass --device <ip>.',
    )
  }

  if (!selector) {
    if (devices.length === 1) return devices[0]!
    throw new Error(
      'More than one Sonos device found:\n' +
        devices.map(device => `  - ${device.name} (${device.ip})`).join('\n') +
        '\nPick one with --device <name|ip>.',
    )
  }

  const query = selector.toLowerCase()
  const exact = devices.find(device => device.name.toLowerCase() === query)
  if (exact) return exact

  const partial = devices.find(device => device.name.toLowerCase().includes(query))
  if (partial) return partial

  throw new Error(
    `No device matched "${selector}". Found:\n` +
      devices.map(device => `  - ${device.name} (${device.ip})`).join('\n'),
  )
}
