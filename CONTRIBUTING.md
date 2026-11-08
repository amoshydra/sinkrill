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

## Releasing

Releases are automated with [Release Please](https://github.com/googleapis/release-please):

- Use [Conventional Commits](https://www.conventionalcommits.org/) (`feat:`,
  `fix:`, `feat!:`).
- Every push to `main` updates or opens a release PR titled
  `chore(main): release x.y.z`.
- Merging that PR creates the `vX.Y.Z` tag, the GitHub Release, and the
  `thrum-linux-amd64/arm64` binaries with checksums.
- The first release is bootstrapped from `0.0.0` in
  `.release-please-manifest.json`, so it becomes `0.1.0`.

Release PRs opened with `GITHUB_TOKEN` do not trigger CI (GitHub suppresses
token-created events); the merge commit on `main` does.

The AUR `PKGBUILD` is updated manually per release: bump `pkgver`, run
`updpkgsums`, rebuild to regenerate `.SRCINFO`, and push to the AUR.

## Reporting bugs

Include:

- your distro, PipeWire/PulseAudio version, and (if used) ffmpeg version
- `thrum version`
- `thrum devices`
- the log output (`thrum -v`)
- `curl -s http://127.0.0.1:8567/status` while running
