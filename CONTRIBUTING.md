# Contributing

Thanks for improving `sonos-sink`.

## Setup

```bash
git clone https://github.com/amoshydra/sonos-sink.git
cd sonos-sink
make build
./sonos-sink --no-tune    # verify the stream without touching a speaker
```

A real Sonos speaker is needed to test the end-to-end path. For the stream
alone:

```bash
ffprobe http://127.0.0.1:8567/stream.wav
```

## Guidelines

- Keep the default path low-latency: WAV PCM, no warm-up buffer, no resampling.
- If a change affects buffering or latency, mention it in the PR and, when
  possible, measure it with `./sonos-sink probe wav 30`.
- Run `make fmt`, `make vet` and `make test` before pushing (CI runs all three).
- Keep the HTTP stream Icecast-compatible: no `Content-Length`, no chunked
  transfer encoding, `Connection: close`.
- Do not add cloud services, accounts, or telemetry. This tool is LAN-only.
- Prefer the standard library. The binary should stay dependency-free.

## Reporting bugs

Include:

- your distro, PipeWire/PulseAudio version, and (if used) ffmpeg version
- `sonos-sink version`
- `sonos-sink devices`
- the log output (`sonos-sink -v`)
- `curl -s http://127.0.0.1:8567/status` while running
