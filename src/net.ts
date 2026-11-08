import dgram from 'node:dgram'
import net from 'node:net'
import os from 'node:os'

const SSDP_MULTICAST_ADDR = '239.255.255.250'
const SSDP_PORT = 1900
const MULTICAST_TTL = 4

/** Interface prefixes we never want to advertise or send discovery from. */
const IGNORED_IFACES = ['lo', 'wg', 'tun', 'docker', 'br-', 'veth', 'virbr', 'podman', 'cni']

function isUsableInterface(name: string): boolean {
  return !IGNORED_IFACES.some(prefix => name === prefix || name.startsWith(prefix))
}

/**
 * Find the IPv4 address Sonos speakers should use to reach this machine.
 * Picks the first non-internal address on a "real" interface.
 */
export function detectLanIp(): string {
  const interfaces = os.networkInterfaces()
  for (const [name, addresses] of Object.entries(interfaces)) {
    if (!addresses || !isUsableInterface(name)) continue
    for (const address of addresses) {
      if (address.family === 'IPv4' && !address.internal) return address.address
    }
  }
  return ''
}

/** IPv4 /24 networks this machine is attached to (SSDP fallback scope). */
export function getLocalSubnets(): string[] {
  const subnets: string[] = []
  const interfaces = os.networkInterfaces()
  for (const [name, addresses] of Object.entries(interfaces)) {
    if (!addresses || !isUsableInterface(name)) continue
    for (const address of addresses) {
      if (address.family !== 'IPv4' || address.internal || !address.cidr) continue
      const prefix = Number.parseInt(address.cidr.split('/')[1] || '32', 10)
      if (prefix >= 24) {
        const octets = address.address.split('.')
        subnets.push(`${octets[0]}.${octets[1]}.${octets[2]}.0/24`)
      }
    }
  }
  return subnets
}

function probePort(ip: string, port: number, timeoutMs: number): Promise<boolean> {
  return new Promise<boolean>(resolve => {
    const socket = new net.Socket()
    let settled = false
    const finish = (open: boolean) => {
      if (settled) return
      settled = true
      socket.destroy()
      resolve(open)
    }
    socket.setTimeout(timeoutMs)
    socket.once('connect', () => finish(true))
    socket.once('error', () => finish(false))
    socket.once('timeout', () => finish(false))
    socket.connect(port, ip)
  })
}

/**
 * Fallback discovery: probe port 1400 across local /24 networks. Needed when
 * SSDP multicast is blocked (VPNs, some switches/AP isolation).
 */
export async function scanSonosIps(timeoutMs = 400): Promise<string[]> {
  const targets = new Set<string>()
  for (const subnet of getLocalSubnets()) {
    const prefix = subnet.split('/')[0]!.split('.').slice(0, 3).join('.')
    for (let host = 1; host <= 254; host++) targets.add(`${prefix}.${host}`)
  }

  const list = [...targets]
  const results = await Promise.all(list.map(ip => probePort(ip, 1400, timeoutMs)))
  return list.filter((_, index) => results[index])
}

/**
 * SSDP M-SEARCH for ZonePlayer devices. Returns the IPs that answered.
 * Resolves after `timeoutMs` even if no device responds.
 */
export function discoverSonosIps(timeoutMs = 3000): Promise<string[]> {
  return new Promise<string[]>((resolve, reject) => {
    const socket = dgram.createSocket({ type: 'udp4', reuseAddr: true })
    const ips = new Set<string>()
    let settled = false

    const finish = (error?: Error): void => {
      if (settled) return
      settled = true
      try {
        socket.close()
      } catch {
        /* already closed */
      }
      if (error) reject(error)
      else resolve([...ips])
    }

    socket.on('message', (message, remote) => {
      const text = message.toString('utf8', 0, Math.min(message.length, 2048))
      if (text.includes('Sonos') || text.toLowerCase().includes('rincon')) {
        ips.add(remote.address)
      }
    })

    socket.on('error', error => finish(error))

    const lanIp = detectLanIp()

    socket.bind(0, lanIp || undefined, () => {
      try {
        socket.setBroadcast(true)
        socket.setMulticastTTL(MULTICAST_TTL)
        if (lanIp) socket.setMulticastInterface(lanIp)
      } catch {
        /* best effort */
      }

      try {
        socket.addMembership(SSDP_MULTICAST_ADDR, lanIp || undefined)
      } catch {
        /* some interfaces refuse multicast membership */
      }

      const search = Buffer.from(
        'M-SEARCH * HTTP/1.1\r\n' +
          `HOST: ${SSDP_MULTICAST_ADDR}:${SSDP_PORT}\r\n` +
          'MAN: "ssdp:discover"\r\n' +
          'MX: 2\r\n' +
          'ST: urn:schemas-upnp-org:device:ZonePlayer:1\r\n' +
          '\r\n',
      )

      const send = (address: string) => {
        socket.send(search, 0, search.length, SSDP_PORT, address, error => {
          if (error) {
            // A failing broadcast on one interface is not fatal.
          }
        })
      }

      send(SSDP_MULTICAST_ADDR)
      send('255.255.255.255')
    })

    setTimeout(() => finish(), timeoutMs)
  })
}
