# Contributing

Thanks for improving Thrum.

## Setup

```bash
git clone https://github.com/amoshydra/thrum.git
cd thrum
make build
./thrum --no-tune    # verify the stream without touching a speaker
```

A real Sonos speaker is needed to test the end-to-end path. For the stream
alone:

```bash
ffprobe http://127.0.0.1:8567/stream.wav
```

## Guidelines

- Keep the default path low-latency: WAV PCM, no warm-up buffer, no resampling,
  and a bounded per-listener queue.
- If a change affects buffering or latency, mention it in the PR and, when
  possible, measure it with `./thrum probe wav 30`.
- Run `make fmt`, `make vet` and `make test` before pushing (CI runs all three).
- Keep the HTTP stream Icecast-compatible: no `Content-Length`, no chunked
  transfer encoding, `Connection: close`.
- Do not add cloud services, accounts, or telemetry. This tool is LAN-only.
- Prefer the standard library. The binary should stay dependency-free.
- Use the Thrum name and voice from [`BRANDING.md`](BRANDING.md) in docs.

## Reporting bugs

Include:

- your distro, PipeWire/PulseAudio version, and (if used) ffmpeg version
- `thrum version`
- `thrum devices`
- the log output (`thrum -v`)
- `curl -s http://127.0.0.1:8567/status` while running
