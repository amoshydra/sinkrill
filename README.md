![Sinkrill](assets/banner.svg)

[![CI](https://github.com/amoshydra/sinkrill/actions/workflows/ci.yml/badge.svg)](https://github.com/amoshydra/sinkrill/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/amoshydra/sinkrill)](https://github.com/amoshydra/sinkrill/releases)
[![Go](https://img.shields.io/github/go-mod/go-version/amoshydra/sinkrill)](go.mod)
[![License](https://img.shields.io/github/license/amoshydra/sinkrill)](LICENSE)

**Sinkrill** streams your Linux PC's audio output to Sonos speakers over your LAN —
an AirPlay-like "PC audio" sink in a single static binary.

> **sinkrill** *(noun)*: *rill* is a small stream. A sinkrill is the stream that
> runs from your audio sink to the room.

It defaults to uncompressed **WAV PCM**, because Sonos gives a WAV file stream
far less buffering than an internet-radio style MP3 stream: **~0.1 s** buffered
instead of **~4.8 s** (measured; see [Latency](#latency)).

```bash
sinkrill                                # stream this PC to the room, with the control panel
sinkrill --virtual-sink                 # recommended: add a "Sinkrill" output device
sinkrill "Living Room" --virtual-sink   # pick a room
sinkrill --plain                        # keep the scrolling log instead of the panel
sinkrill --no-tune                      # only serve the stream; tune the speaker yourself
```

One command does both jobs: it starts the stream, and in a terminal it draws a
live panel for volume, mute, play/pause, and retune. Nothing non-interactive
changes — a service, `nohup`, a pipe, or a redirect gets plain logs instead of
escape codes.

`--virtual-sink` is still the recommended way to listen: it creates a real
output device, so only the apps you route to it reach the room, with no echo.
Without the flag, Sinkrill mirrors your current output instead: the PC and
Sonos both play, and no new device appears.

Press `Ctrl+C` (or `q` in the panel) to stop playback and shut down.

## Why

- **Looks like a speaker.** `--virtual-sink` creates a real PipeWire/PulseAudio
  output device called **Sinkrill**, so you pick it in your audio settings or
  per-app.
- **Stream and controls together.** In a terminal, one command starts the audio
  and opens the panel. A service, pipe, or redirect keeps plain logs instead.
- **Low latency by default.** WAV over plain HTTP avoids Sonos's internet-radio
  pre-buffer: ~0.1 s buffered, measured, instead of ~4.8 s with MP3.
- **No Node, no npm, no Icecast.** One static binary; it shells out to
  `parec`/`pactl` (already part of your audio stack) and optionally `ffmpeg`.
- **No cloud, no account.** Audio never leaves your LAN.

## Two audio sources, one command

The only real choice is what gets captured. Both options start the same way:
one command, stream plus panel.

### Output device (recommended)

`--virtual-sink` creates a PipeWire/PulseAudio output named **Sinkrill**:

```bash
sinkrill --virtual-sink
```

Pick **Sinkrill** in your audio settings (or per-app in `pavucontrol`), and only
the audio routed to it goes to Sonos. The device disappears when Sinkrill exits.

### Mirror the current output

Without the flag, Sinkrill captures whatever is playing on your default output:

```bash
sinkrill
```

The PC and Sonos both play, and no new device appears. Use this when you just
want the room to hear what the PC is playing.

In a terminal, both commands also open the control panel — the stream and its
controls in one window. Add `--plain` to keep the scrolling log instead; that is
also what happens automatically when the output is not a terminal.

## How it works

```
                     PipeWire/PulseAudio
 +---------------+                          +---------------+
 |   PC audio    | -----------------------> |     parec     |
 |   (any app)   |      monitor source      |  s16le 48kHz  |
 +---------------+                          +-------+-------+
                                                    |
                                                    |  WAV/PCM over HTTP
                                                    |  ~1.5 Mbps
                                                    |
 +---------------+                          +-------+-------+
 |     Sonos     |      http:// (file)      | stream server |
 |    speaker    |<-----------------------  |  /stream.wav  |
 +---------------+                          +---------------+
                      SetAVTransportURI
```

1. `parec` records the monitor of your current audio output
   (`@DEFAULT_MONITOR@`), or of the virtual **Sinkrill** device created by
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

### Platform support

Sinkrill is **Linux-only** today. Sonos discovery, control, and HTTP streaming
are already portable — audio capture is the part that is not.

| Platform | Status |
|-----------|--------|
| Linux (PipeWire/PulseAudio) | **Supported** — `parec`, with `ffmpeg -f pulse` as a fallback |
| Windows | Builds, but no capture backend yet. Needs WASAPI loopback. |
| macOS | Builds, but no capture backend yet. Needs CoreAudio taps or a virtual device such as BlackHole. |
| Android / Termux | Not viable. Android does not expose the system audio mix to apps; only `MediaProjection`, from inside a real APK, can capture playback. |

On an unsupported platform the binary exits with a clear message rather than
falling over later. A port needs one new file in
[`internal/capture`](internal/capture) with a build tag and an entry in
`platformBackends`; the capture hot path stays untouched.

## Install

### Binary (recommended)

Download the latest release, verify the checksum, and put it on your `PATH`:

```bash
curl -LO https://github.com/amoshydra/sinkrill/releases/latest/download/sinkrill-linux-amd64
curl -LO https://github.com/amoshydra/sinkrill/releases/latest/download/sinkrill-linux-amd64.sha256
sha256sum -c sinkrill-linux-amd64.sha256

install -Dm755 sinkrill-linux-amd64 ~/.local/bin/sinkrill
sinkrill --help
```

Use `sinkrill-linux-arm64` on ARM machines.

### Go

```bash
go install github.com/amoshydra/sinkrill@latest
```

Make sure `$(go env GOPATH)/bin` is on your `PATH`.

### Arch (AUR)

A `PKGBUILD` is in [`contrib/aur`](contrib/aur). Once the package is in the AUR:

```bash
yay -S sinkrill
```

Until then, build it straight from the repo — it pins the released `v0.2.0`
tarball:

```bash
git clone https://github.com/amoshydra/sinkrill.git
cd sinkrill/contrib/aur
makepkg -si
```

### From source

```bash
git clone https://github.com/amoshydra/sinkrill.git
cd sinkrill
make            # builds ./sinkrill
make install    # installs to /usr/local/bin (PREFIX=/... to override)
```

## Usage

```bash
# One command: stream plus the control panel
sinkrill --virtual-sink                 # recommended audio source
sinkrill "Living Room" --virtual-sink   # pick a room
sinkrill --device 192.168.1.42 --virtual-sink

# Mirror the current output instead (no new device)
sinkrill
sinkrill "Living Room"

# Keep the scrolling log instead of the panel
sinkrill --virtual-sink --plain

# List what's on the network
sinkrill devices

# Stop playback without starting a stream
sinkrill stop "Living Room"

# Volume, without touching the stream or restarting anything
sinkrill volume                 # print it
sinkrill volume 40              # set it
sinkrill volume +5              # nudge up (also: up 10)
sinkrill volume -5              # nudge down (also: down)
sinkrill volume mute            # unmute to undo

# Point the speaker back at the running stream (manual keepalive)
sinkrill retune

# What the running instance is doing
sinkrill status

# Panel only: watch a stream that is already running
sinkrill tui

# Control a different room without starting anything
sinkrill tui --attach --device "Kitchen"

# Serve only: prints the URL to paste into the Sonos app
sinkrill --no-tune

# Diagnose silence, capture, and routing
sinkrill doctor

# Capture a specific source instead of the default output
sinkrill --source alsa_output.usb-Generic_USB_Audio-00.analog-stereo.monitor
```

Starting a second copy while a stream is live exits immediately and names the
running instance, rather than printing a bare `bind: address already in use`.
Use `sinkrill status` to inspect it, or `--port 0` to run another stream on a
free port.

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
| `--virtual-sink [name]` | off | Create the **Sinkrill** output device (recommended); default name `sinkrill` |
| `--station <name>` | `<hostname> streaming audio` | Name shown on the speaker |
| `--plain` | off | Log to the terminal instead of showing the panel |
| `-v, --verbose` | off | Verbose logs |

### HTTP endpoints

| Path | Description |
|------|-------------|
| `/stream.wav` | Live WAV PCM stream (default) |
| `/stream.mp3` | Live MP3 stream (`--format mp3`) |
| `/stream` | Alias for whichever format is active |
| `/status` | JSON: device, source, format, listeners, queue, capture state |
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
- a byte-bounded listener queue (~125 ms) that drops old audio instead of
  drifting behind, plus `TCP_NODELAY` and a small send buffer

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

WAV is a RIFF container with 32-bit size fields, so the header can only
advertise a 4 GB stream — about **6 h 12 min** at 192 kB/s. When the speaker
reaches that length it treats the track as finished and stops; the stream is
still running. `--keepalive` re-tunes within ~20 s, or run `sinkrill retune`
to restart the clock. MP3 mode uses a radio scheme with no length at all, so
it has no such horizon.

## Output device mode (recommended)

`--virtual-sink` creates a real output device called **Sinkrill** in
PipeWire/PulseAudio:

```bash
sinkrill --virtual-sink
```

Now choose **Sinkrill** as the output in your desktop audio settings (or per-app in
`pavucontrol` → Playback, or with `wpctl set-default <id>`). Only audio routed
to that device goes to the speaker; everything else keeps playing locally. The
device is removed again when Sinkrill exits.

Until something is routed to it, the device is silent. The panel makes that
visible: while no application is connected to it, the `Hint` line names the
device you need to pick.

```bash
sinkrill --virtual-sink sinkrill          # explicit name
sinkrill --virtual-sink my-tv-sink     # any valid sink name
```

If you'd rather own the device yourself (so it survives restarts), create it
once and capture its monitor:

```bash
pactl load-module module-null-sink \
  sink_name=sinkrill \
  sink_properties=device.description=Sinkrill

sinkrill --source sinkrill.monitor
```

## Control

Sinkrill runs in the foreground. `Ctrl+C` stops the speaker, kills the capture
process, removes the virtual device, and exits.

| What | How |
|------|-----|
| Start / stop | `sinkrill --virtual-sink` / `Ctrl+C`, `q` in the panel, or `sinkrill stop "Living Room"` |
| Volume | `sinkrill volume 40` while running, `sinkrill volume +5` / `mute`, or `--volume 40` on start |
| Panel | Opens with the stream in a terminal. `sinkrill tui` attaches to a running stream from another terminal; `--plain` keeps the scrolling log instead |
| Re-tune | `sinkrill retune` — points the room back at the running stream, e.g. after another device took it |
| Leftovers | `sinkrill cleanup` — removes the output device and stray capture process after a `kill -9` or crash |
| Pause / resume | The panel, or the Sonos app; long pauses may drop the stream connection, then restart `sinkrill` (or run with `--keepalive`) |
| Change room | stop, then `sinkrill "Other Room" --virtual-sink` |
| Inspect | `sinkrill status` (or `curl http://<lan-ip>:8567/status`) |
| Preview | open `http://<lan-ip>:8567/stream.wav` in VLC or a browser |

### Control panel

Run `sinkrill` in a terminal and the stream and panel come up together: room,
volume, capture health, and buffering in one window. There is no second
command to start and no `tui` flag to remember. `sinkrill tui` opens the same
panel without starting anything, which is useful when the stream is already
running in another terminal or under a service.

```text
 sinkrill  homedesktop streaming audio
 -------------------------------------------------------------------------
  Room      Den
  State     playing
  Playing   this stream
  Volume    [====================    ]  82
  Stream    wav, 1536 kbps
  Capture   running, last data 19 ms ago, 0 restarts
  Buffer    0 B queued, 0 dropped, 1 listener
 -------------------------------------------------------------------------
  up/down 2   left/right 10   m mute   space play/pause   s stop
  r retune    n/b track       q quit
  ready
```

When the Sinkrill device exists but no application has been routed to it, the
panel says so instead of leaving you with silence and no clue:

```text
  Hint      no app is using "sinkrill" yet; pick it as your output device
```

`up`/`down` nudge the volume by 2, `left`/`right` by 10 (`+`, `-`, `[`, `]`
do the same). `n`/`b` skip within the room's queue, which only does something
when the room is playing an album or playlist rather than this stream. `r`
points the room back at this stream, the same as `sinkrill retune`. `q`, `Esc`,
and `Ctrl+C` all leave the panel. A panel that started the stream also stops it:
the speaker, the capture process, and the virtual device are all released before
the terminal is handed back. A panel that attached leaves the stream running.
While it runs, the stream's own log lines appear in the panel instead of
printing over it.

`sinkrill tui` on its own attaches to the instance already running — started in
another terminal or as a service — and leaves it running when the panel closes.
Add a room or any stream option and it starts the stream itself, exactly as
`sinkrill` would:

```bash
sinkrill tui "Living Room" --virtual-sink
```

After the panel closes, a short summary stays in the terminal — room, station,
stream URL, and how much was captured — so the useful parts of the log survive:

```text
sinkrill stopped on Den (192.168.0.188)
  station  homedesktop streaming audio
  uri      http://192.168.0.29:8567/stream.wav
  captured 12 MB, 1 listener
```

Any start command accepts `--plain` to skip the panel and watch the full log
as it happens.

To drive a room that the running instance is not tuned to — say your stream is
on Den and you want Kitchen to join it — add `--attach`, which starts nothing:

```bash
sinkrill tui --attach --device "Kitchen"
```

`r` then re-points Kitchen at the running stream. The panel never changes the
stream's formatting, capture, or port.


## Run as a user service

The unit runs `/usr/bin/sinkrill`, which is where the Arch package puts it. If
you installed the binary somewhere else, point `ExecStart` at it.

A service has no terminal, so it logs to the journal and never draws the panel.
That is automatic; there is no flag to set.

```bash
mkdir -p ~/.config/systemd/user
cp contrib/systemd/sinkrill.service ~/.config/systemd/user/

# only if you did not install to /usr/bin:
sed -i "s|/usr/bin/sinkrill|$HOME/.local/bin/sinkrill|" \
  ~/.config/systemd/user/sinkrill.service

# set the room name
$EDITOR ~/.config/systemd/user/sinkrill.service

systemctl --user daemon-reload
systemctl --user enable --now sinkrill
journalctl --user -u sinkrill -f
```

## If you hear nothing

Check the panel first. If it shows:

```text
  Hint      no app is using "sinkrill" yet; pick it as your output device
```

then the stream is healthy and silent because no application is routed to the
virtual device. Pick **Sinkrill** as the output in your sound settings. This is
the most common cause, and nothing needs fixing.

If the panel shows something else, or you are running without it, run:

```bash
sinkrill doctor
```

It checks the server, the capture, the output device, and the speaker, and
prints the next command to run.

If the capture is **stalled** (it can happen after a PipeWire/PulseAudio
hiccup), restart Sinkrill. The 5-second watchdog normally recovers this
automatically, but a restart always recreates the sink and capture:

```bash
systemctl --user restart sinkrill
# or, if running in a terminal: Ctrl+C, then start it again
```

The watchdog only fires when the capture delivers **zero bytes**. Silence is
still PCM data — an idle PC produces silent frames continuously — so "nothing
is playing" does not trigger it.

Then check the three volume/routing stages:

```bash
pactl info | grep 'Default Sink'      # should be: sinkrill
pactl get-sink-mute sinkrill             # should be: no
pactl set-sink-volume sinkrill 100%      # keep Sinkrill at 100%, control level on Sonos
```

And check the speaker: the Sonos app should show **playing**, not muted, with a
reasonable volume.

Test the whole path with a generated tone (this works on Arch too; the
`/usr/share/sounds/alsa/...` path is Debian/Ubuntu-only):

```bash
ffmpeg -f lavfi -i "sine=frequency=440:duration=2" -ar 48000 -ac 2 /tmp/sinkrill-tone.wav
paplay --device=sinkrill /tmp/sinkrill-tone.wav
```

If `paplay` says `open(): No such file or directory`, the *file* is missing —
that is not a Sinkrill or device error. If the tone is silent on the speaker, run
`sinkrill doctor` again.

## Troubleshooting

**The "Sinkrill" output device is still in my audio settings**

That happens when sinkrill was killed with `SIGKILL` or crashed, so its cleanup
never ran: the null sink and the capture process attached to it both survive.
A later run finds the sink already there and leaves it alone, because it did
not create it.

```bash
sinkrill cleanup
```

That stops any stray capture process for the device and unloads the sink. It
refuses while a running instance is actually using that device. `Ctrl+C` and
`SIGTERM` both clean up properly, so this is only needed after a hard kill.

**No Sonos devices found**

- Make sure the speakers are powered on and on the same network.
- SSDP multicast is often blocked by VPNs, guest Wi-Fi/AP isolation, or some
  switches. Sinkrill falls back to scanning the local `/24` on port `1400`, but a
  VPN that captures LAN traffic can still hide the speakers. Try
  `sinkrill devices --timeout 8000`, or pass `--device <ip>` directly.
- If the scan also fails, find the speaker's IP in your router, then use
  `--device <ip>`.

**Sonos says it can't connect / nothing plays**

- The speaker must be able to reach `<your-lan-ip>:<port>` directly. Check your
  firewall (e.g. `ufw allow 8567/tcp`) and that `--host` is the LAN interface's
  address, not a VPN or Docker address.
- Make sure the port isn't already in use; use `--port 0` to pick a free one.
- Try `sinkrill --no-tune` and open
  `http://<lan-ip>:<port>/stream.wav` from another device to verify reachability.

**No sound, but the stream is running**

- `@DEFAULT_MONITOR@` records the current output. If audio is routed to a
  different device, pass `--source <that-device>.monitor`.
- List sources with `pactl list short sources`.
- DRM-protected playback (Netflix, some streaming apps) may produce silence in
  the capture. That's a platform restriction, not a bug.

**It feels laggy**

- Check `curl -s http://127.0.0.1:8567/status | grep -E 'queuedBytes|droppedChunks'`.
  `queuedBytes` should stay near 0; a large queue means a listener is behind.
- Make sure you are in WAV mode (`format: "wav"`), not MP3.
- Sonos's own buffer is ~0.1 s; a wired line-in or HDMI is the only way lower.

**The speaker stops after a while**

- Live streams stop themselves when the connection hiccups. Restart with
  `--keepalive` to let Sinkrill re-tune automatically. Note that this can also
  undo a manual stop from the Sonos app.

**The log says `capture delivered no bytes ... restarting`**

- That is the watchdog recovering a stalled `parec`. It should recover in a few
  seconds. If it repeats, check `pactl list short sources` and run `sinkrill doctor`.
- This is not triggered by silence: an idle sink still delivers silent PCM
  frames. It only fires when the capture process produces zero bytes.

**Capture restarts in the logs**

- `parec`/`ffmpeg` is restarted automatically if it exits, and the speaker is
  retuned. Check the warnings for the reason (usually an invalid `--source`).

## Development

```bash
make test        # go test ./...
make vet         # go vet ./...
make build       # ./sinkrill
make fmt
```

Measure how much audio Sonos buffers (stop the running service first; the probe
retunes the speaker):

```bash
./sinkrill probe wav 30
./sinkrill probe mp3 30
```

Releases are managed by [Release Please](CONTRIBUTING.md#releasing): merge the
release PR to tag `vX.Y.Z` and publish the binaries.

The project started as a Node/TypeScript CLI, was rewritten in Go for a single
static binary, and is now called Sinkrill. The old implementations are in git
history.

See [`BRANDING.md`](BRANDING.md) for the name, logo, palette, and voice.

## Trademark

Sinkrill is an independent project and is **not affiliated with, endorsed by, or
sponsored by Sonos, Inc.** "Sonos" is a trademark of Sonos, Inc. and is used
only to describe compatibility.

## License

MIT
