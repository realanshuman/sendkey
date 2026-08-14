# SendKey brand

The identity in one line: **a key that travels once.** It is sent, it opens
one door, and it is gone. Everything below exists to say that quietly.

The visual register is calm and precise, in the tradition of modern developer
tools: dark graphite surfaces, hairline borders, one accent, generous space.
The product handles dangerous material; the interface should feel like a
steady hand.

## Name

- Product name in prose: **SendKey**. One word, capital S, capital K.
- Wordmark and code: **sendkey**. Always lowercase.
- Never "Send Key", "SendKeys", or "send-key".

## The mark

A key dissolving into dots as it travels. Send, key, and burn after reading
in a single glyph.

```
   ⌾――――  ● ·
      |
```

- Bow (the ring) on the left, shaft pointing right, one tooth below.
- The shaft breaks into two dots that fade in the direction of travel: the
  key is mid flight, already starting to vanish.
- Always monoline: round caps, round joins, consistent stroke.
- Always filled with the iris gradient (periwinkle into violet, left to right).
- Source files: `sendkey/public/assets/logo.svg` (transparent, for use next
  to the wordmark) and `sendkey/public/assets/mark.svg` (on a dark rounded
  tile, used as the favicon and as a standalone avatar).
- Clear space: keep at least half the mark's height empty on every side.
- Do not rotate it, outline it, recolor it, or place it on a busy background.

## The wordmark

Set "sendkey" in the UI stack at weight 600 with letter spacing -0.01em,
in `--ink`, at the same optical height as the mark. The mark carries the
color; the wordmark never does.

## Color

Dark only. Cool graphite, not warm black: secrets pass through at night.
One hue in the whole system: the iris. If something is iris, it matters.

| Token | Value | Use |
| --- | --- | --- |
| `--bg` | `#08090b` | page canvas |
| `--bg-2` | `#0c0d10` | recessed bands, inputs |
| `--surface` | `#101114` | cards |
| `--surface-2` | `#16171b` | chips, nested surfaces |
| `--line` | `#1f2126` | hairline borders |
| `--line-2` | `#2e3138` | emphasized borders, controls |
| `--ink` | `#f7f8f9` | headings, primary text |
| `--ink-2` | `#9a9fa9` | body text |
| `--ink-3` | `#666b76` | captions, fine print |
| `--iris` | `#8b93ff` | the accent: the key, links, focus |
| `--ok` | `#4cc38a` | success glyphs only |
| `--bad` | `#ff5d5f` | error text only |

Gradients:

- Mark fill: `#a5adff → #7d6cf0`, left to right, in userSpaceOnUse units
  (bounding box units vanish on straight strokes).
- Headline highlight, one phrase per page at most:
  `linear-gradient(96deg, #aab1ff, #cdb9ff, #8b93ff)`.

Rules of use:

- The iris means "this is the key" or "this is the action": the fragment in
  a link, a focus ring, a step number. It never decorates.
- Primary buttons are light (`--ink` background, near black text), in the
  manner of modern developer tools. The accent is too precious to spend on
  every button.
- Success and error stay in text plus a `✓` glyph, so meaning never depends
  on color perception alone.

## Type

System stack, tuned rather than imported. No webfonts: the strict CSP allows
no external requests, and native rendering is faster than any download.

- UI and headings: `ui-sans-serif, system-ui, -apple-system, "Segoe UI", Roboto, sans-serif`
- Secrets, links, keys, labels, code: `ui-monospace, "SF Mono", SFMono-Regular, Menlo, Consolas, monospace`
- Display headings: weight 600, letter spacing -0.045em, line height 1.05.
- Body: 15px, line height 1.65. Small text never drops below 11.5px.
- Anything a machine will consume (a link, a key, a secret, a byte count)
  is always mono.
- Eyebrow labels are mono, 12px, uppercase, letter spaced 0.14em, iris.

## Voice

Plain, calm, and specific. The reader may be a developer or someone's
parent; both deserve sentences they can parse once.

- Short sentences. One idea per sentence.
- No em dashes. Use a period or a comma instead.
- Say what happens, not what we promise: "browsers never send the fragment"
  beats "we take your privacy seriously".
- Name the limits honestly. The FAQ says what SendKey does not protect
  against.
- Never fear-monger. The product removes worry; the copy should too.

## Motion

Motion exists to show that something is alive or in transit, never to
impress.

- The composer's status dot pulses (the encryptor is live).
- Cards and buttons respond in under 200ms with opacity or 1px translation.
- Nothing else moves. Everything respects `prefers-reduced-motion`.

## Surfaces

- The landing page leads with the product itself: the composer is in the
  hero, working, not a screenshot of it.
- Sections sit on `--bg`; alternating chapters recess to `--bg-2` between
  hairlines. Cards sit on `--surface`.
- Grids are drawn with 1px hairlines (grid gap over `--line`), not with
  floating boxes and shadows.
- One soft iris glow may sit behind a page's focal point. Never two.
