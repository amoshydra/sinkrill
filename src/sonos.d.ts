/** Minimal typings for the untyped `sonos` package (v1.14.x). */
declare module 'sonos' {
  export class Sonos {
    constructor(host: string, port?: number)
    host: string
    port: number
    play(): Promise<void>
    pause(): Promise<void>
    stop(): Promise<void>
    setVolume(volume: number): Promise<void>
    getVolume(): Promise<number>
    getCurrentState(): Promise<string>
    currentTrack(): Promise<{
      title?: string
      artist?: string
      album?: string
      uri?: string
    }>
    deviceDescription(): Promise<{
      UUID?: string
      roomName?: string
      displayName?: string
      modelName?: string
      modelNumber?: string
    }>
    setAVTransportURI(options: {
      uri: string
      metadata?: string
      onlySetUri?: boolean
    }): Promise<boolean>
    startListening(options?: { ignoreSubscriptionErrors?: boolean }): Promise<void>
    stopListening(): Promise<void>
    on(event: string, listener: (...args: unknown[]) => void): this
  }
}
