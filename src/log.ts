export type LogLevel = 'debug' | 'info' | 'warn' | 'error'

let verbose = false

export function setVerbose(enabled: boolean): void {
  verbose = enabled
}

function timestamp(): string {
  return new Date().toISOString().slice(11, 19)
}

function write(level: LogLevel, message: string): void {
  if (level === 'debug' && !verbose) return
  const stream = level === 'warn' || level === 'error' ? process.stderr : process.stdout
  stream.write(`${timestamp()} ${level.toUpperCase().padEnd(5)} ${message}\n`)
}

export const log = {
  debug: (message: string) => write('debug', message),
  info: (message: string) => write('info', message),
  warn: (message: string) => write('warn', message),
  error: (message: string) => write('error', message),
}
