# Sinkrill brand guide

## The name

**Sinkrill** · /ˈsɪŋk.rɪl/ · *noun*

> *Sink* + *rill* — a rill is a small stream. The stream that runs from your
> audio sink to the room.

Sinkrill is a coined compound, not a dictionary word, so it carries no borrowed
meaning and no prior owners. It describes what the tool literally does: it
catches audio at a virtual sink and runs it to the speakers. It is also unique
across npm, AUR, PyPI, crates.io, Homebrew, and the US trademark register, which
makes it easy to own and easy to search for.

The join is deliberate, but check it before shortening: `sink` + `rill` also
parses as **krill**. That is treated below as a feature, not an accident.

## Mascot

The companion mark is a **krill** — a small shrimp riding the current. It is a
pun on the name and a brief for the product: small, quiet, and carried by the
stream rather than running the show.

- Use it in release notes, social avatars, and the README footer. Not in the CLI.
- Keep it line-art or single-colour, in Hum `#F59E0B`, so it sits beside the pulse
  mark without competing.
- It must never replace the pulse mark as the app icon; the pulse is the
  identifier, the krill is the personality.

**Tagline:** *Send your PC's sound to the speakers in the room.*

**Short description:** A low-latency PC-to-Sonos audio sink for Linux.

**Elevator pitch:** Sinkrill turns your computer's audio output into a speaker on
the network. Pick it in the audio selector and the sound comes out of Sonos.

## Voice

Calm, precise, and warm. It is a system daemon, not a lifestyle app.

- Say "your PC's sound", not "content" or "media".
- Lead with what it does, then why it is fast.
- No exclamation marks, no emoji in product copy, no growth-hacker verbs.
- Be honest about limits. Sinkrill is ~0.1 s, not 0 ms; say so.
- Never imply official Sonos affiliation.

## Naming conventions

| Thing | Value |
|-------|-------|
| Product | **Sinkrill** |
| Binary / command | `sinkrill` |
| Repository | `github.com/amoshydra/sinkrill` |
| Go module | `github.com/amoshydra/sinkrill` |
| systemd user unit | `sinkrill.service` |
| AUR package | `sinkrill` |
| Virtual output device | **Sinkrill** (`sinkrill` sink, `sinkrill.monitor`) |
| Default station name | `<hostname> streaming audio` (e.g. `homedesktop streaming audio`) |
| Latency probe | `sinkrill probe` |

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

Sinkrill is an independent project and is **not affiliated with, endorsed by, or
sponsored by Sonos, Inc.** "Sonos" is a trademark of Sonos, Inc. and is used
only to describe compatibility.
