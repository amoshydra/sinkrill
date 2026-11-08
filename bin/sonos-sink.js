#!/usr/bin/env node
import { existsSync } from 'node:fs'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

const here = path.dirname(fileURLToPath(import.meta.url))
const cli = path.join(here, '..', 'dist', 'cli.js')

if (!existsSync(cli)) {
  console.error('sonos-sink is not built yet. Run "pnpm build" (or "npm run build") first.')
  process.exit(1)
}

await import(cli)
