# Sonara — design system, dashboard, and landing page

High-fidelity, self-contained HTML prototypes for **Sonara** — the ambient
clinical documentation and prescription assistant specified in the product PRD
(v1.0, Aug 2026). Everything runs from a file open or any static host: no
build step, no backend, no dependencies beyond Google Fonts.

| File | What it is |
|---|---|
| `design-system.html` | The dashboard design system: tokens, color/type/spacing rules, every component the product needs (consent module, pipeline stepper, transcript, note fields with provenance, Rx composer, allergy hard stop, banners, audit ledger…), safety patterns, accessibility bar, and product voice. |
| `app.html` | The dashboard, end-to-end against PRD V1.0. Interactive prototype with mock data — see the demo script below. |
| `index.html` | The marketing landing page, built from scratch on the brand kit (deliberately not a rework of the earlier draft). |
| `tokens.css` | Canonical token sheet for the production build (maps 1:1 onto a Tailwind theme). |

## Design decisions worth knowing

- **The brand kit is law.** Petrol carries the product; Bricolage Grotesque /
  Inter Tight / IBM Plex Mono; sentence case; tabular numerals everywhere a
  number can change.
- **Signal red means one thing.** `#D8452F` marks "the mic is live" and
  nothing else. The kit left errors/destructive undefined, so the system adds
  two families: **Amber** (caution: consent, low confidence, free-text drugs)
  and **Brick** (hard stops: allergy blocks, destructive actions, failures).
  Brick never animates; only Signal pulses.
- **16px floor** on body text and inputs (PRD §11.5); 14px only for metadata,
  11–12px only for mono labels. Contrast pairs are measured and documented in
  the design system's color section.
- **Safety is rendered, not asserted.** Consent gates the record button;
  unsigned prescriptions are visibly unsigned; the signing sheet shows full
  content; signed records lock with addenda-only corrections; the allergy
  block demands a typed, logged reason; blocked front-desk reads still write
  audit entries.

## Demo script for `app.html`

1. Enter as **Dr. A. Menon** (or via the email + OTP path).
2. Open **Priya Nair (token 07)** → read the consent script (EN/हिन्दी/मराठी),
   tick consent → **Start recording** → watch the live diarized transcript →
   **End visit** → pipeline runs → draft case sheet + tags appear.
3. Hover note sections to see **provenance** (source transcript lines light
   up). Edit any section — autosave indicator. Try **Swap DR ↔ PT** and the
   per-line **Fix** (clinic vocabulary).
4. Compose the Rx: type `croc` → Enter; try a free-text line. **Review &
   sign** shows the full content; sign → locked note, delivery
   (print/WhatsApp/email, each logged), addenda.
5. Open **Rakesh Bhosale (token 08)** (penicillin allergy on file), draft his
   visit, then add `mox` → the **allergy hard stop** with typed-reason
   override.
6. **Demo scenarios** menu (top bar): connection drop, unclear audio
   (low-confidence path), 20-minute warning, mid-visit consent withdrawal,
   day reset — the PRD §5.3 recovery paths.
7. Switch persona (avatar menu): **Dr. R. Iyer** shows the
   verification-pending gate (drafting works, signing locked);
   **Sunita Pawar** shows front-desk role separation (queue + demographics
   only, 403s logged).
8. Check **Patients** (fuzzy search: `priya nai`), a profile (structured
   allergies, timeline, JSON/PDF export, delete-with-retention), the
   **Audit log**, and **Settings** (retention, team & verification,
   vocabulary, DPDP).

## Scope notes

Scope follows PRD V1.0 deliberately: manual prescription composing only — the
"Sonara suggests" panel is **not** in the dashboard (deferred to V1.5 per
§3.2) but its component is specified in the design system's "Reserved for
V1.5" section so it won't need a new visual language later.
