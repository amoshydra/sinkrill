# Contributing

Thanks for improving `sonos-sink`.

## Setup

```bash
pnpm install
pnpm dev -- --no-tune   # runs from source with tsx
pnpm typecheck
pnpm build
```

A real Sonos speaker is needed to test the end-to-end path. `--no-tune` plus
`ffprobe http://127.0.0.1:8567/stream.wav` verifies the stream without touching
a speaker.

## Guidelines

- Keep the default path low-latency: WAV PCM, no warm-up buffer, no resampling.
- If a change affects buffering or latency, mention it in the PR and, when
  possible, measure it with `node scripts/latency-probe.mjs wav 30`.
- `pnpm typecheck` and `pnpm build` must pass (CI runs both).
- Keep the HTTP stream Icecast-compatible: no `Content-Length`, no chunked
  transfer encoding, `Connection: close`.
- Do not add cloud services, accounts, or telemetry. This tool is LAN-only.

## Reporting bugs

Include:

- your distro, PipeWire/PulseAudio version, and ffmpeg version
- `sonos-sink --version`
- `sonos-sink devices`
- the log output (`sonos-sink -v`)
- `curl -s http://127.0.0.1:8567/status` while running
