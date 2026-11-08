# sonos-sink

[![CI](https://github.com/amoshydra/sonos-sink/actions/workflows/ci.yml/badge.svg)](https://github.com/amoshydra/sonos-sink/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/amoshydra/sonos-sink)](https://github.com/amoshydra/sonos-sink/releases)
[![Go](https://img.shields.io/github/go-mod/go-version/amoshydra/sonos-sink)](go.mod)
[![License](https://img.shields.io/github/license/amoshydra/sonos-sink)](LICENSE)

Stream this computer's audio output to Sonos speakers over your LAN — an
AirPlay-like "PC audio" sink for Linux, in a single static binary.

It defaults to uncompressed **WAV PCM** because Sonos gives a WAV file stream
far less buffering than an internet-radio style MP3 stream: **~0.1 s** buffered
instead of **~4.8 s** (measured; see [Latency](#latency)).

```bash
sonos-sink                 # auto-detect the only Sonos speaker and start streaming
sonos-sink "Living Room"   # pick a room by name
sonos-sink --virtual-sink  # appear as a "Sonos" output device in your audio selector
sonos-sink --no-tune       # only serve the stream; tune Sonos yourself
```

Press `Ctrl+C` to stop playback and shut down.

## Why

- **Looks like a speaker.** `--virtual-sink` creates a real PipeWire/PulseAudio
  output device, so you pick **Sonos** in your audio settings or per-app.
- **Low latency by default.** WAV over plain HTTP avoids Sonos's internet-radio
  pre-buffer: ~0.1 s buffered, measured, instead of ~4.8 s with MP3.
- **No Node, no npm, no Icecast.** One static binary; it shells out to
  `parec`/`pactl` (already part of your audio stack) and optionally `ffmpeg`.
- **No cloud, no account.** Audio never leaves your LAN.

## How it works

```
┌──────────────┐  PipeWire/PulseAudio  ┌───────────────┐  WAV/PCM over HTTP  ┌──────────┐
│  PC audio    │ ────────────────────► │    parec      │ ──────────────────► │  Sonos   │
│ (any app)    │  monitor source       │ s16le 48 kHz  │  ~1.5 Mbps          │ speaker  │
└──────────────┘                       └───────┬───────┘                     └────▲─────┘
                                               │                                  │
                                       ┌───────▼────────┐      http:// (file)      │
                                       │ stream server  │─────────────────────────┘
                                       │ /stream.wav    │   SetAVTransportURI
                                       └────────────────┘
```

1. `parec` records the monitor of your current audio output
   (`@DEFAULT_MONITOR@`), or of the virtual **Sonos** device created by
   `--virtual-sink`. If `parec` is missing, `ffmpeg -f pulse` is used instead.
2. The bytes are served as a continuous 48 kHz/16-bit stereo WAV stream.
3. Sonos is told to play `http://<lan-ip>:<port>/stream.wav`. Because that looks
   like a file rather than internet radio, Sonos skips its multi-second
   pre-buffer.
4. `--format mp3` encodes with ffmpeg and uses `x-rincon-mp3radio://` instead
   (radio handling, ~192 kbps, much more buffering).

There is **no warm-up buffer** by default, so a newly connected speaker starts
at the current live position. Add `--warmup 500` if you have flaky Wi-Fi and
would rather trade latency for robustness.

## Requirements

- **Linux** with PipeWire or PulseAudio
- `parec` and `pactl` (in `libpulse` on Arch, `pulseaudio-utils` on Debian)
- `ffmpeg` only if you use `--format mp3`
- Go 1.22+ only if you build from source

```bash
# Arch
sudo pacman -S libpulse          # ffmpeg is optional (MP3 mode)

# Debian/Ubuntu
sudo apt install pulseaudio-utils
```

## Install

### Binary (recommended)

Download the latest release, verify the checksum, and put it on your `PATH`:

```bash
curl -LO https://github.com/amoshydra/sonos-sink/releases/latest/download/sonos-sink-linux-amd64
curl -LO https://github.com/amoshydra/sonos-sink/releases/latest/download/sonos-sink-linux-amd64.sha256
sha256sum -c sonos-sink-linux-amd64.sha256

install -Dm755 sonos-sink-linux-amd64 ~/.local/bin/sonos-sink
sonos-sink --help
```

Use `sonos-sink-linux-arm64` on ARM machines.

### Go

```bash
go install github.com/amoshydra/sonos-sink@latest
```

Make sure `$(go env GOPATH)/bin` is on your `PATH`.

### Arch (AUR)

A `PKGBUILD` is in [`contrib/aur`](contrib/aur). Once the package is in the AUR:

```bash
yay -S sonos-sink
```

### From source

```bash
git clone https://github.com/amoshydra/sonos-sink.git
cd sonos-sink
make            # builds ./sonos-sink
make install    # installs to /usr/local/bin (PREFIX=/... to override)
```

## Usage

```bash
# Let sonos-sink find the speaker (works when there is exactly one)
sonos-sink

# Choose a room by name, or a speaker by IP
sonos-sink "Living Room"
sonos-sink --device 192.168.1.42

# List what's on the network
sonos-sink devices

# Stop playback without starting a stream
sonos-sink stop "Living Room"

# Serve only: prints the URL to paste into the Sonos app
sonos-sink --no-tune

# Capture a specific source instead of the default output
sonos-sink --source alsa_output.usb-Generic_USB_Audio-00.analog-stereo.monitor
```

### Options

| Option | Default | Description |
|--------|---------|-------------|
| `-d, --device <name\|ip>` | auto | Sonos room name (partial match) or IP address |
| `-s, --source <source>` | `@DEFAULT_MONITOR@` | PulseAudio/PipeWire source to record |
| `-p, --port <port>` | `8567` | HTTP port (`0` picks a free one) |
| `-f, --format <wav\|mp3>` | `wav` | `wav` = lowest latency (~1.5 Mbps); `mp3` = ~192 kbps, more buffering |
| `-b, --bitrate <kbps>` | `192` | MP3 bitrate, only used with `--format mp3` |
| `--warmup <ms>` | `0` | Replay this much recent audio to a new listener (higher latency) |
| `--host <ip>` | auto | LAN IP advertised to Sonos |
| `--bind <address>` | `0.0.0.0` | HTTP bind address |
| `--ffmpeg <path>` | `ffmpeg` | ffmpeg binary |
| `--parec <path>` | `parec` | parec binary |
| `--volume <0-100>` | unchanged | Set the speaker volume on start |
| `--keepalive` | off | Re-tune if the speaker stops by itself |
| `--no-tune` | off | Don't control Sonos; only serve the stream |
| `--virtual-sink [name]` | off | Create an output device (default `sonos`) and capture it |
| `--station <name>` | `PC Audio` | Name shown on the speaker |
| `-v, --verbose` | off | Verbose logs |

### HTTP endpoints

| Path | Description |
|------|-------------|
| `/stream.wav` | Live WAV PCM stream (default) |
| `/stream.mp3` | Live MP3 stream (`--format mp3`) |
| `/stream` | Alias for whichever format is active |
| `/status` | JSON: device, source, format, listeners, capture state |
| `/health` | `{"ok":true}` |

Point VLC, a browser, or anything else at `http://<lan-ip>:8567/stream.wav`
(or `/stream.mp3` with MP3 mode) to preview locally.

## Latency

The defaults are tuned for the lowest latency this approach allows:

- **WAV PCM over plain HTTP** — Sonos treats it as a file, so it skips the
  internet-radio pre-buffer (the biggest single win)
- no warm-up replay (`--warmup 0`)
- ~20 ms capture latency (`parec --latency-msec=20`)
- no resampling or encoder delay in WAV mode
- `TCP_NODELAY` on the audio socket and direct packet writes

Measured with the built-in probe: **~0.10 s** of buffered audio on the Sonos
side for WAV, versus **~4.8 s** for MP3 radio streaming. Add the capture side
(~20–30 ms) and you are at roughly **0.1 s end-to-end**.

That is good enough for casual video and most desktop audio. It is not a
wired-grade path; if you need the absolute minimum:

| Path | Latency | Notes |
|------|---------|-------|
| HDMI to a Sonos soundbar | ~30 ms | Arc/Beam/Ray/Playbar/Playbase |
| Line-in on Port/Amp/Five/Play:5/Era + adapter | 75 ms min | set *Audio Delay → Low*; grouped rooms add delay |
| Bluetooth (Roam/Move/Era 100/300) | ~150–300 ms | not under 100 ms |
| AirPlay 2 | ~2 s | worse than WAV mode |

`--format mp3` is the fallback for weak Wi-Fi or very old speakers: ~192 kbps
instead of ~1.5 Mbps, at the cost of Sonos's radio buffer.

WAV mode advertises a 4 GB "file", so after roughly 6 hours of playback the
speaker may stop. `--keepalive` re-tunes automatically.

## Use it as a selectable output device

By default `sonos-sink` mirrors your current output, so the PC and Sonos both
play. Add `--virtual-sink` and it creates a real output device named **Sonos**
in PipeWire/PulseAudio:

```bash
sonos-sink --virtual-sink
```

Now choose **Sonos** as the output in your desktop audio settings (or per-app in
`pavucontrol` → Playback, or with `wpctl set-default <id>`). Only audio routed
to that device goes to the speaker; everything else keeps playing locally. The
device is removed again when `sonos-sink` exits.

```bash
sonos-sink --virtual-sink sonos          # explicit name
sonos-sink --virtual-sink my-tv-sink     # any valid sink name
```

If you'd rather own the device yourself (so it survives restarts), create it
once and capture its monitor:

```bash
pactl load-module module-null-sink \
  sink_name=sonos \
  sink_properties=device.description=Sonos

sonos-sink --source sonos.monitor
```

## Control

`sonos-sink` runs in the foreground. `Ctrl+C` stops the speaker, kills the
capture process, removes the virtual device, and exits.

| What | How |
|------|-----|
| Start / stop | `sonos-sink` / `Ctrl+C` (or `sonos-sink stop "Living Room"`) |
| Volume | Sonos app, or `--volume 40` on start |
| Pause / resume | Sonos app; long pauses may drop the stream connection, then restart `sonos-sink` (or run with `--keepalive`) |
| Change room | stop, then `sonos-sink "Other Room"` |
| Inspect | `curl http://<lan-ip>:8567/status` |
| Preview | open `http://<lan-ip>:8567/stream.wav` in VLC or a browser |

## Run as a user service

Copy [`contrib/systemd/sonos-sink.service`](contrib/systemd/sonos-sink.service)
to `~/.config/systemd/user/`, edit the room name, then:

```bash
systemctl --user daemon-reload
systemctl --user enable --now sonos-sink
journalctl --user -u sonos-sink -f
```

## Troubleshooting

**No Sonos devices found**

- Make sure the speakers are powered on and on the same network.
- SSDP multicast is often blocked by VPNs, guest Wi-Fi/AP isolation, or some
  switches. `sonos-sink` falls back to scanning the local `/24` on port `1400`,
  but a VPN that captures LAN traffic can still hide the speakers. Try
  `sonos-sink devices --timeout 8000`, or pass `--device <ip>` directly.
- If the scan also fails, find the speaker's IP in your router, then use
  `--device <ip>`.

**Sonos says it can't connect / nothing plays**

- The speaker must be able to reach `<your-lan-ip>:<port>` directly. Check your
  firewall (e.g. `ufw allow 8567/tcp`) and that `--host` is the LAN interface's
  address, not a VPN or Docker address.
- Make sure the port isn't already in use; use `--port 0` to pick a free one.
- Try `sonos-sink --no-tune` and open
  `http://<lan-ip>:<port>/stream.wav` from another device to verify reachability.

**No sound, but the stream is running**

- `@DEFAULT_MONITOR@` records the current output. If audio is routed to a
  different device, pass `--source <that-device>.monitor`.
- List sources with `pactl list short sources`.
- DRM-protected playback (Netflix, some streaming apps) may produce silence in
  the capture. That's a platform restriction, not a bug.

**The speaker stops after a while**

- Live streams stop themselves when the connection hiccups. Restart with
  `--keepalive` to let `sonos-sink` re-tune automatically. Note that this can
  also undo a manual stop from the Sonos app.

**Capture restarts in the logs**

- `parec`/`ffmpeg` is restarted automatically if it exits, and the speaker is
  retuned. Check the warnings for the reason (usually an invalid `--source`).

## Development

```bash
make test        # go test ./...
make vet         # go vet ./...
make build       # ./sonos-sink
make fmt
```

Measure how much audio Sonos buffers (stop the running service first; the probe
retunes the speaker):

```bash
./sonos-sink probe wav 30
./sonos-sink probe mp3 30
```

The project started as a Node/TypeScript CLI and was rewritten in Go for a
single static binary. The old implementation is in git history.

## License

MIT
