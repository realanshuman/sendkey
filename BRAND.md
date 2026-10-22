# SendKey brand

Sixth edition: **keyhole buddy**. A warm cream page, white cards with
24px corners, pill buttons, rounded headings, and one character: a coral
keyhole with eyes that peeks over the card you are working in and reacts
to what is happening there. Illustrations are outlined in ink and sit on
yellow, lilac and mint panels.

The register is playful but polished: a friendly utility that is still
obviously safe to paste a password into. Real drawings and a real
character, never decoration for its own sake: no background grids, no
glows, no blurred shadows used as glow, no gradient accents.

## Name

- Product name in prose and in the wordmark: **SendKey**. One word,
  capital S, capital K.
- Never "Send Key", "SendKeys", or "send-key".

## The buddy

The buddy is the mark, the favicon and the mascot. It is a keyhole
silhouette with a face.

### Anatomy

Drawn on a **120 x 132** box. One path is the body; everything else sits
on top of it.

| Part | Geometry | Fill |
| --- | --- | --- |
| Body | `M60 6a46 46 0 0 1 26 84l10 34a6 6 0 0 1-6 8H30a6 6 0 0 1-6-8l10-34A46 46 0 0 1 60 6z` (a round head of radius 46 at (60, 52), a short neck, a skirt that flares to the foot) | coral `#ff6a4d`, in both themes |
| Eye whites | circles r 12.5 at (44, 50) and (76, 50) | white |
| Pupils | circles r 6 at the eye centres, free to move up to 6.5 units | ink `#221d17` |
| Cheeks | ellipses 7 x 4.5 at (30, 68) and (90, 68) | `#ff9f8a` |
| Mouth | a small smile, `M53 71q7 6 14 0`, 3.5 stroke, round caps | ink |
| Sparkles | two four point stars beside the head, delighted only | yellow, ink outline |

Face lines (shut eyes, happy eyes, x eyes, mouths) are all 3.5 units of
ink with round caps and joins. The buddy never gets arms, legs, a hat,
or a second color for its body.

### Where it lives

The buddy peeks over the top edge of the card the person is working in.
The card sits in front of its lower body, so only the head shows: about
80 of the 132 units, which keeps the eyes, cheeks and mouth visible.

- Composer (landing, `/send`, `/drop`): 104px wide, 40px in from the
  card's right edge. 90px wide on phones.
- Receive card (`/s`) and ask card (`/ask`, `/a`): 96px wide, centred.
- Landing burn demo: 80px wide, over the side panel, reacting to the demo.
- Numbers, "What we cannot count": 84px, eyes shut, still. There is
  nothing for it to see.
- Landing call to action: the one place the whole body shows, standing on
  the bottom edge of the yellow panel.

The pattern in markup is a positioned wrapper (`.composer` or `.peek`)
with top padding equal to the visible head, the buddy absolutely
positioned at its top, and the card at `z-index: 1`. The buddy has
`pointer-events: none` and never shifts layout.

### Expressions

The face is one SVG holding every part; `data-mood` on the `<svg>` picks
which parts show (rules in `brand.css`). No mood attribute means idle.

| Mood | Face | When |
| --- | --- | --- |
| idle | open eyes, small smile | default; composer at rest; ask owner waiting |
| curious | pupils up, small "o" mouth | receive gate, passphrase gate, answering an ask |
| shy | eyes shut (two soft curves), smile | while `#pass` has focus, on the composer and the receive passphrase gate |
| happy | arched eyes, open grin, one hop | link created, secret or file revealed, ask answered or opened, burn demo opened |
| delighted | happy face plus two sparkles, two hops | the receipt line turns `.arrived` (someone opened your link), an ask answer arrives |
| sad | little x eyes, small frown | gone and dead states, burn demo dead |
| oops | pupils down, wobbly mouth, head shake | for about a second when an error line appears |

### Behaviour

`assets/buddy.js` is presentation only. It reads state the page already
shows: which section is visible, `.arrived` on a status line, focus on
`#pass`, the burn demo's `.is-on` pane and `.is-dead` class. It watches
them with one `MutationObserver` plus focus events, and never calls into
`app.js`, `view.js`, `ask.js` or `burn.js`.

- Pupils follow a mouse pointer (fine pointers only, never a finger),
  at most 3.5 units, and only while the buddy is on screen.
- An occasional blink, every 2.6 to 6.8 seconds, each buddy on its own
  clock, only when its eyes are open.
- A hop on happy, two on delighted, a head shake on oops.
- Under `prefers-reduced-motion` none of that moves: no follow, no blink,
  no hop, no shake. The expressions still change, since a different face
  is information rather than motion.

### Logo files

Under `sendkey/public/assets/`:

- `mark.svg`: the favicon. The buddy on a square box with slightly larger
  eyes so they survive 16px. No cheeks, no mouth.
- `logo.svg`: the full face, for light surfaces.
- `logo-inv.svg`: the same face with a 6 unit cream sticker edge, for dark
  or busy surfaces. The body stays coral.

Inline in the nav and footer the logo is a small buddy (27 x 30, no
cheeks or mouth, pupils looking slightly down and right) beside the
wordmark set in the rounded face at 20px / 700, 10px apart.

## Color

Every value is a token on `:root` in `brand.css`, redefined for dark mode
under `:root[data-theme="dark"]` and under the system dark query.

| Token | Light | Dark | Use |
| --- | --- | --- | --- |
| `--bg` | `#fbf6ee` | `#17130f` | page: cream, then warm near-black |
| `--surface` | `#ffffff` | `#211b16` | cards, the CLI band |
| `--subtle` | `#f6efe4` | `#2a231d` | recessed fields, link boxes, side panels |
| `--line` | `#ece2d3` | `#342b23` | card borders, dividers |
| `--line-2` | `#dccfbb` | `#4a3e33` | input borders |
| `--text` | `#221d17` | `#f6eee3` | headings, emphasis |
| `--text-2` | `#5b5248` | `#c9bcab` | body copy |
| `--text-3` | `#766b5f` | `#a39582` | captions, placeholders (4.5:1 on page and card) |
| `--coral` | `#ff6a4d` | `#ff6a4d` | the buddy, illustration fills, focus border. Never small text |
| `--coral-ink` | `#c2391d` | `#ff8a70` | coral text: links, the key in a URL (5:1 or better) |
| `--coral-display` | `#e0482b` | `#ff7a5c` | the hero's coral line only, 3.8:1 (AA for large text) |
| `--yellow` | `#ffd75e` | `#f0c64f` | art panel, tiles, the waiting dot, the closing panel |
| `--lilac` | `#cbbcff` | `#b9a8f5` | art panel, tiles, the fingerprint tile |
| `--mint` | `#a9e5c8` | `#96d6b7` | art panel, tiles, ticks, the arrived dot |
| `--ok` / `--err` | `#1f7a4d` / `#b42318` | `#7ddca8` / `#ff9b85` | success and error text, badges |

- Illustration ink (`#221d17`) and paper (white) are pinned in both
  themes, so art panels, tiles and the buddy read as light stickers on a
  dark page.
- The primary button is ink with white type in light mode and cream with
  ink type in dark mode. One primary per surface. The closing panel is
  yellow in both themes, so its buttons are pinned to ink and white.
- The terminal is the one dark surface in both themes: warm ink, with the
  palette as syntax colors (yellow strings, lilac flags, coral key).
- theme.js stamps the stored or system theme on `<html>` before first
  paint; `theme-color` is `#fbf6ee` light and `#17130f` dark.

## Type

- Headings, card titles, the wordmark, big numbers, FAQ questions:
  `ui-rounded, "SF Pro Rounded", "Avenir Next", "Nunito",
  "Segoe UI Variable Display", "Segoe UI", system-ui`. Weight 700,
  tracking -0.02em to -0.035em.
- Safari on Apple platforms renders SF Pro Rounded. Chromium does not
  resolve `ui-rounded` or SF Pro Rounded by name, so on a Mac it lands on
  Avenir Next Bold: a friendly geometric face with round bowls that keeps
  the same voice. Windows lands on Segoe UI Variable Display. Nothing is
  downloaded.
- Body text, labels, buttons: the system sans, 15px / 1.6.
- Monospace only for real codes: links, keys, commands, endpoints, byte
  counts.
- Sentence case everywhere. No all-caps labels, no eyebrows, no numbered
  headings, no bars or hairlines beside headings.

## Shape

- Cards: 24px radius, a 1.5px `--line` border and at most one soft
  shadow (`0 10px 24px -18px`), never a glow.
- Fields: 14px radius, 1.5px border, a coral border and a solid 4px soft
  coral ring on focus.
- Buttons and badges are pills. Icon tiles are 56px squares with 17px
  corners.
- Keyboard focus is a 2.5px solid outline in the text color.

## Illustrations

- Outlined drawings: ink strokes (2.5 on the 160 x 110 panel art, 2 on
  32 x 32 tile icons), round caps and joins, white paper, fills taken
  from the palette (coral, yellow, lilac, mint).
- Mode cards (Mural style): a white card with a colored panel on top.
  Yellow holds the chat bubble with a flame, lilac the page with a
  padlock, mint the envelope with a key.
- The six guarantees each get a small tile: key, flame, stopwatch,
  padlock, closed eye, code window. The numbers wall reuses the style:
  link, open eye, flame, locked box.
- Small interface icons stay 24px line icons drawn with a 2 stroke in
  `currentColor`. Checklists use a mint sticker tick.

## Voice

Plain, warm, specific. Short sentences. No em dashes. Say what happens,
not what we promise. Name the limits honestly. The buddy never speaks:
it reacts.

## Surfaces

- Landing: the working composer beside the headline, the buddy on top of
  it. Three mode cards, the burn demo watched by the buddy, six tiled
  guarantees, the CLI band, the FAQ as rounded cards, a yellow closing
  panel with the whole buddy, then a three column footer.
- `/send` and `/drop`: the title above, the composer with the buddy, and
  three dot chips underneath. `/drop` turns the drop strip into a lilac
  panel with the file drawing.
- `/s` receive and `/ask`: one card under the buddy holding the title and
  every state. Revealing burns, and the page says so before the click,
  in bold.
- After a link is made, a live receipt line sits under it: a pulsing
  yellow dot while nobody has opened it, a steady mint one and a clock
  time once someone has, and the buddy is delighted.
- API: a left aligned reference with pill verbs (coral POST, mint GET),
  rounded endpoint cards and small flag badges.
- Numbers: four tiled counters, two pill bars in coral, yellow and lilac,
  and the shut-eyed buddy over what we cannot count.
- Errors are soft red blocks with plain sentences; the buddy winces.
