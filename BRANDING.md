# Thrum brand guide

## The name

**Thrum** · /θrʌm/ · *noun*

> A continuous low sound — the hum of a machine, or a room full of music.

The name is the product: a quiet, continuous stream that fills a room. It is a
real word, so it needs no invented backstory, but it is uncommon enough to be
ownable and easy to search for alongside "Linux audio".

**Tagline:** *Send your PC's sound to the speakers in the room.*

**Short description:** A low-latency PC-to-Sonos audio sink for Linux.

**Elevator pitch:** Thrum turns your computer's audio output into a speaker on
the network. Pick it in the audio selector and the sound comes out of Sonos.

## Voice

Calm, precise, and warm. It is a system daemon, not a lifestyle app.

- Say "your PC's sound", not "content" or "media".
- Lead with what it does, then why it is fast.
- No exclamation marks, no emoji in product copy, no growth-hacker verbs.
- Be honest about limits. Thrum is ~0.1 s, not 0 ms; say so.
- Never imply official Sonos affiliation.

## Naming conventions

| Thing | Value |
|-------|-------|
| Product | **Thrum** |
| Binary / command | `thrum` |
| Repository | `github.com/amoshydra/thrum` |
| Go module | `github.com/amoshydra/thrum` |
| systemd user unit | `thrum.service` |
| AUR package | `thrum` |
| Virtual output device | **Thrum** (`thrum` sink, `thrum.monitor`) |
| Default station name | `Thrum` |
| Latency probe | `thrum probe` |

## Logo

The mark is a pulse: a warm dot emitting three arcs, drawn with a gradient that
runs from amber (the hum) to cyan (the signal). It reads as both a sound wave
and a network beacon.

| File | Use |
|------|-----|
| [`assets/logo.svg`](assets/logo.svg) | App icon, avatar, favicon source |
| [`assets/wordmark.svg`](assets/wordmark.svg) | Horizontal lockup on light backgrounds |
| [`assets/banner.svg`](assets/banner.svg) | README / release header |

Usage:

- Keep clear space around the mark equal to at least the dot's diameter.
- Minimum size: 24 px for the mark, 120 px for the wordmark.
- Do not recolor the gradient, add a drop shadow, or place the wordmark on a
  busy photo.
- On dark backgrounds use the standalone mark; the wordmark is ink-colored.

## Palette

| Token | Hex | Use |
|-------|-----|-----|
| Ink | `#0F172A` | Logo background, dark headers |
| Paper | `#F8FAFC` | Text on dark, light surfaces |
| Hum | `#F59E0B` | Primary accent, the dot |
| Wave | `#22D3EE` | Secondary accent, signal arcs |
| Muted | `#94A3B8` | Taglines, secondary text |
| Success | `#10B981` | "playing" / healthy states |

The signature gradient is **Hum → Wave**: `linear-gradient(135deg, #F59E0B, #22D3EE)`.

## Typography

Use the system UI stack for documentation and the CLI:

```
Inter, "Segoe UI", system-ui, -apple-system, sans-serif
```

Code and terminal output use the platform monospace stack. No display fonts:
the product should feel native on any Linux desktop.

## Trademark

Thrum is an independent project and is **not affiliated with, endorsed by, or
sponsored by Sonos, Inc.** "Sonos" is a trademark of Sonos, Inc. and is used
only to describe compatibility.
