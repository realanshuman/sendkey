# SendKey brand

Fourth edition: **1-bit**. Paper and ink, nothing else. Every gray on the
page is a dither pattern telling a small lie with black pixels, and every
illustration is pixel art on a hard grid.

The register is a machine from 1984 that happens to do modern cryptography:
system windows, striped title bars, hard shadows, a blinking status square.
The product stores only noise it cannot read; the visual world agrees, by
having no shades at all.

## Name

- Product name in prose: **SendKey**. One word, capital S, capital K.
- Wordmark and code: **sendkey**, rendered uppercase by the pixel face.
- Never "Send Key", "SendKeys", or "send-key".

## The mark

A keyhole punched clean through a solid plate. Nothing else: no shaft, no
teeth, no foot. The plate is the ink, the keyhole is the paper showing
through, and at every size the mark is one silhouette with one hole in it.

The counter is the whole design, and it tapers. Reading down: a round head
six cells wide, a neck pinched to two, a skirt that opens back out to
four. The skirt must stay narrower than the head. Cut them the same width
and the shape stops reading as a keyhole and starts reading as an
hourglass, which is exactly what the earlier mark did.

- Drawn on a 12 x 13 grid, integer coordinates, `crispEdges`, one path.
  The box is tight to the plate: no padding, so a lockup can size the mark
  by height alone and let the width follow.
- Files under `sendkey/public/assets/`: `logo.svg` (ink plate, for paper),
  `logo-inv.svg` (paper plate, for ink panels), `mark.svg` (favicon: the
  same keyhole through a full-bleed 16 x 16 tile, so the counter survives
  at tab size).
- Inline copies in the page markup use `fill: currentColor`, which is what
  makes the plate invert with the theme rather than needing a second file.
- The lockup pairs the plate at 26px tall with the SENDKEY wordmark in the
  pixel face, 10px apart, baselines optically centered.
- Do not smooth it, outline it, tint it, or rotate it.

## Color

Two colors, plus grays for legibility of secondary text only. Imagery gets
no grays at all: a surface is paper, ink, or a dither of the two.

| Token | Value | Use |
| --- | --- | --- |
| `--paper` | `#f2efe9` | page, cards, window chrome |
| `--paper-2` | `#e7e3d8` | recessed bands |
| `--field` | `#faf8f3` | inputs |
| `--ink` | `#141412` | text, borders, shadows, inverted panels |
| `--ink-2` | `#45453f` | body copy |
| `--ink-3` | `#73736a` | captions |
| `--ink-dim` | `#b9b5a9` | secondary text on ink panels |

Emphasis is inversion. The key phrase of the headline, the open FAQ item,
the fragment in a link, the terminal's key chip: ink panel, paper text.
There is no accent color to spend, so attention is rationed by contrast.

Dark mode is the same system with the two colors traded: the page turns
ink, text and borders turn paper, and every inverted panel comes out light
instead of black, so the tail of the page glows instead of switching off.
The terminal is the one pinned surface: a terminal is dark in both themes,
and only its outer edge follows the theme. The dither strips swap to
paper-pixel variants of the same generated assets. theme.js stamps the
stored or system theme on <html> before first paint, and the nav's square
toggle flips it; the choice persists per browser.

## Dither

The dither is the brand's shading system, generated, never hand-faked:

- `tools/dither/main.go` renders all patterns through a Bayer 8x8 ordered
  matrix into tiny PNGs under `assets/px/` (87 to 206 bytes each).
- `fade.png` dissolves paper into ink; it is the transition into the black
  tail of the page and the texture strip along the top of each page.
- `d06.png`, `d12.png`, `d25.png` are uniform density tiles for texture.
- Tiles render at 2x with `image-rendering: pixelated`, chunky and square.
- Dither is decoration, never background for text. Body copy sits on flat
  paper, full stop.
- The 50% checker (checkboxes, the public-id swatch) is pure CSS
  `repeating-conic-gradient`, no asset needed.

## Type

- Display: **Press Start 2P**, self-hosted woff2 (latin subset, 4.7 KB),
  OFL licensed, license file alongside the font. Used for h1 to h3, the
  wordmark, window titles, step numbers. Always uppercase, generous line
  height (1.45 or looser), sizes kept modest because the face is loud.
- Everything else: the system mono stack. Body 14px / 1.7. Labels, buttons
  and nav links are bold, uppercase, letter spaced.
- The strict CSP allows no external requests; the pixel face ships from our
  own origin or not at all.

## Chrome

- Borders are 2px ink, radius 0, everywhere.
- Shadows are hard offsets of solid ink (or paper on ink panels): 3px for
  controls, 6px for windows and grids. Nothing blurs.
- Cards that hold the product are System-style windows: striped title bar,
  square close box, plated title, `.win-bar` + `.win-body`.
- Grids are ruled like printed tables: 2px ink gaps between paper cells.
- Buttons press: hover lifts 1px, active sinks 2px, in steps() so the
  motion snaps rather than glides.
- Focus is a 2px dotted ink outline.
- The status square blinks like a cursor (steps(1), 1.1s). It is the only
  thing on the page that moves on its own. Everything respects
  `prefers-reduced-motion`.

## Voice

Unchanged from the previous edition, because the writing was never the
costume: plain, calm, specific. Short sentences. No em dashes. Say what
happens, not what we promise. Name the limits honestly. The pixels are
allowed to be playful; the sentences are not.

## Surfaces

- The landing page leads with the working composer inside a window titled
  NEW SECRET, tagged "encrypts locally" with the blinking square.
- The receive page is a dialog titled ENCRYPTED MESSAGE. Revealing burns;
  the page says so before the click, in bold, on paper.
- The ask page is a dialog titled SECRET REQUEST, one page for both roles:
  the browser holding the private key sees its ask; anyone else sees the
  answer form. Both render the same 8 x 8 pixel key fingerprint.
- File drops keep the composer's window: a dashed drop strip, a file card
  with a pixel document icon, and a hard-edged progress bar whose fill is
  solid ink. Progress snaps in steps; nothing glides.
- After a link is made, a live receipt line sits under it, reusing the ask
  page's status pattern: a blinking square while nobody has opened it, a
  steady one and a clock time once someone has.
- Errors are inverted ink blocks prefixed with `!!`. No red exists here.
- The page ends in ink: a dithered fade strip, then the CTA and footer on
  black. The machine turns off at the bottom.
