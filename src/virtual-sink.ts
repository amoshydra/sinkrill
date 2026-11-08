import { execFile } from 'node:child_process'
import { promisify } from 'node:util'
import { log } from './log.js'
import { errorMessage } from './util.js'

const execFileAsync = promisify(execFile)

export interface VirtualSinkHandle {
  /** PulseAudio/PipeWire sink name. */
  name: string
  /** Monitor source to record. */
  monitorSource: string
  /** True when this process created the sink (and should remove it on exit). */
  created: boolean
  dispose: () => Promise<void>
}

async function pactl(args: string[]): Promise<string> {
  try {
    const { stdout } = await execFileAsync('pactl', args, { timeout: 5000 })
    return stdout.trim()
  } catch (error) {
    const code = (error as NodeJS.ErrnoException).code
    if (code === 'ENOENT') {
      throw new Error('pactl not found. Install pulseaudio-utils (or pipewire-pulse) to use --virtual-sink.')
    }
    throw new Error(`pactl ${args[0]} failed: ${errorMessage(error)}`)
  }
}

function rows(output: string): string[][] {
  return output
    .split('\n')
    .filter(line => line.trim().length > 0)
    .map(line => line.split('\t'))
}

async function sinkExists(name: string): Promise<boolean> {
  const sinks = rows(await pactl(['list', 'short', 'sinks']))
  return sinks.some(parts => parts[1] === name)
}

function validateName(name: string): void {
  if (!/^[A-Za-z0-9_.-]+$/.test(name)) {
    throw new Error(
      `Invalid sink name "${name}". Use letters, digits, dot, dash or underscore.`,
    )
  }
}

/**
 * Make sure a null sink with `name` exists, so it shows up as an output device
 * in the desktop audio selector. Apps routed to it are captured from
 * `<name>.monitor` and streamed to Sonos.
 */
export async function ensureVirtualSink(
  name: string,
  description = 'Sonos',
): Promise<VirtualSinkHandle> {
  validateName(name)

  const existing = await sinkExists(name)
  if (!existing) {
    await pactl([
      'load-module',
      'module-null-sink',
      `sink_name=${name}`,
      `sink_properties=device.description=${description}`,
    ])
  }

  const handle: VirtualSinkHandle = {
    name,
    monitorSource: `${name}.monitor`,
    created: !existing,
    dispose: async () => {
      // Only remove what we created; leave user-managed sinks alone.
      if (existing) return
      try {
        const modules = rows(await pactl(['list', 'short', 'modules']))
        const owner = modules.find(parts =>
          (parts[2] || '').includes(`sink_name=${name}`),
        )
        const id = owner?.[0]
        if (id) {
          await pactl(['unload-module', id])
          log.info(`removed virtual output "${name}"`)
        }
      } catch (error) {
        log.debug(`could not remove virtual sink: ${errorMessage(error)}`)
      }
    },
  }

  return handle
}
