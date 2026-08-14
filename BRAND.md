# SendKey brand

The identity in one line: **a secret is a spark. It travels once, lights up for
one person, and burns out.** Everything below exists to say that clearly.

## Name

- Product name in prose: **SendKey**. One word, capital S, capital K.
- Wordmark and code: **sendkey**. Always lowercase.
- Never "Send Key", "SendKeys", or "send-key".

## The mark

A key whose shaft is an arrow. Send plus key, one glyph.

```
     ⌾――――――→
        |
```

- Bow (the ring) on the left, arrowhead on the right, one tooth below.
- Always drawn monoline: round caps, round joins, consistent stroke.
- Always filled with the ember gradient (amber into orange, 135 degrees).
- Source files: `sendkey/public/assets/logo.svg` (transparent, for use next to
  the wordmark) and `sendkey/public/assets/mark.svg` (on a dark rounded tile,
  used as the favicon and as a standalone avatar).
- Clear space: keep at least half the mark's height empty on every side.
- Do not rotate it, outline it, recolor it, or place it on a busy background.

## Color

Dark only. The product is a place where secrets pass through; it looks like it.
One hue in the whole system: the ember. If something is orange, it matters.

| Token | Hex | Use |
| --- | --- | --- |
| `--bg` | `#0a0908` | page canvas (warm black) |
| `--bg-2` | `#11100e` | recessed bands, inputs |
| `--surface` | `#151311` | cards |
| `--surface-2` | `#1c1916` | chips, nested surfaces |
| `--line` | `#272420` | default borders |
| `--line-2` | `#3a352e` | emphasized borders |
| `--ink` | `#f4f1ec` | headings, primary text |
| `--ink-2` | `#a8a29a` | body text |
| `--ink-3` | `#736d65` | captions, fine print |
| `--ember` | `#ff6a2b` | the accent: actions, the key, the burn |
| `--ember-2` | `#ffae3f` | gradient partner, highlights |
| `--ember-ink` | `#201007` | text on ember backgrounds |

Gradients:

- Ember fill (buttons, the mark): `linear-gradient(135deg, #ffae3f, #ff6a2b)`
- Ember text (one phrase per screen): `linear-gradient(115deg, #ffc24b, #ff7a32, #ff5a2b)`

Rules of use:

- The ember means "this is the moment": the primary action, the decryption key,
  the burn. It never decorates.
- Success and error states stay in ink, marked with a `✓` or `✗` glyph in
  ember, so meaning never depends on color perception alone.

## Type

System stack, tuned rather than imported. No webfonts: the strict CSP allows no
external requests, and nothing here needs a custom face.

- UI and headings: `Inter, ui-sans-serif, system-ui, -apple-system, "Segoe UI", Roboto, sans-serif`
- Secrets, links, keys, code: `ui-monospace, "SF Mono", SFMono-Regular, Menlo, Consolas, monospace`
- Headings are tight: weight 700, letter spacing around -0.04em.
- Anything a machine will consume (a link, a key, a secret) is always mono.

## Voice

Plain, calm, and specific. The reader may be a developer or someone's parent;
both deserve sentences they can parse once.

- Short sentences. One idea per sentence.
- No em dashes. Use a period or a comma instead.
- Say what happens, not what we promise: "browsers never send the fragment"
  beats "we take your privacy seriously".
- Name the limits honestly. The FAQ says what SendKey does not protect against.
- Never fear-monger. The product removes worry; the copy should too.

## Motion

Motion exists to show that something is alive or in transit, never to impress.

- The composer's status dot pulses (the encryptor is live).
- The key's path in the diagram flows (the key is in transit).
- Cards lift 2px on hover. Nothing else moves.
- Everything respects `prefers-reduced-motion`.

## Surfaces

- The landing page leads with the product itself: the composer is in the hero,
  working, not a screenshot of it.
- Recessed bands (`--bg-2`) separate chapters; cards sit on `--surface`.
- Wide graphics (diagrams, the link anatomy) scroll inside their own container
  rather than stretching the page.
