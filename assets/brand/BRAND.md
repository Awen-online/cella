# Cardano Curia: Brand Spec

Portable brand reference for Cardano Curia and related products (e.g. a
standalone Cella product). The single source of truth for tokens is
`brand.css` in this same folder; treat this document as the human-readable
digest, and `brand.css` as the implementation. Copy this entire `brand-kit/`
folder into a new product to inherit the brand wholesale.

---

## 1. Concept & voice

Roman deliberative authority applied to Cardano governance. The visual
language is **inscription-on-stone** (carved capitals, gold leaf, laurel,
marble/parchment), deliberately *warm and classical* to stake out a niche
away from the cool-blue "another protocol" crowd.

- **Wordmark convention:** Latin `V`-for-`U`: "Cvria Cardani" / "Cvria".
- **Tagline:** *Integritas ante omnia* ("Integrity above all").
- **Tone:** formal, civic, deliberative: "the Senate / the Mandate / the
  Record," not "dashboard / feed / app."

## 2. The mark

A **laurel wreath** (constant) surrounding the **official Cardano starburst**
(30-circle vector from cardano.org brand assets). Only the wreath's *finish*
changes by role:

| Role | File | Finish |
|---|---|---|
| **Primary** (≥64px, light bg) | `logo/v3-gold-leaf.svg` | Textured matte gold leaf, gradient `#E6B660 → #C9892A → #8E5C18` + fine grain |
| **Small** (<64px, favicons, app icons) | `logo/v2-gold-solid.svg` | Solid `#C9892A` |
| **Monochrome** (print, emboss, etch) | `logo/v4-ink.svg` | Single ink `#1A1E3A` |
| **Dark mode** (Cella login, dark UI) | `logo/v9-ivory-on-forum.svg` | Ivory wreath `#FAF7EE` on `#0A0E27`, bright-gold starburst `#F5D27A` |

**Selection rule:** light + ≥64px → v3; light + <64px → v2; dark surface →
v9; one-color reproduction → v4. Alternates (`v1` legacy bronze, `v5`
unified-blue, `v6` antique-gold, `v7` marble) require a written rationale.

The wreath is a single traced SVG path reused across all variants; only
`fill` (and v3's filter) change. Vector source and the traced original live
under `logo/_source/`.

## 3. Color tokens

```css
/* Cardano blue family */
--cc-blue:        #0033AD;  /* primary */
--cc-blue-bright: #1E5BFF;  /* CTAs / interactive accent / links */
--cc-blue-deep:   #0A1A57;  /* hover, deep ink */
--cc-blue-tint:   #E3EBFA;  /* surfaces, callouts */

/* Gold leaf family */
--cc-gold:        #C9892A;  /* mid gold, solid fill, eyebrows */
--cc-gold-bright: #F5D27A;  /* highlight */
--cc-gold-deep:   #7A5418;  /* shadow, hairline rules */
--cc-gold-ink:    #5E3F08;  /* gold used as small body type */

/* Neutrals: marble / parchment */
--cc-parchment:   #FAF7EE;  /* page bg */
--cc-marble:      #F4ECD8;  /* card bg / alt section */
--cc-veined:      #ECE2C5;  /* divider tint */
--cc-ivory:       #FFFCF4;  /* highest surface (cards) */

/* Inks */
--cc-ink:         #1A1E3A;  /* primary text */
--cc-ink-soft:    #4A4F70;  /* body text */
--cc-ink-mute:    #7A7F99;  /* meta */
--cc-forum:       #0A0E27;  /* dark-mode bg / footer */
--cc-forum-veil:  #131A40;  /* dark-mode card */

/* Semantic */
--cc-success: #1F8A56;
--cc-danger:  #B43A2F;
--cc-warn:    #C58A1A;
```

Elementor system-color mapping on the WP site: primary `#0033AD`, secondary
`#C9892A`, text `#1A1E3A`, accent `#1E5BFF`.

## 4. Typography

```css
--cc-font-display: 'Cinzel', 'Trajan Pro', 'Times New Roman', serif;    /* headlines, wordmark, buttons, eyebrows */
--cc-font-body:    'EB Garamond', 'Cormorant Garamond', Georgia, serif;  /* body copy */
--cc-font-ui:      'Inter', system-ui, 'Segoe UI', sans-serif;           /* UI chrome */
--cc-font-mono:    'JetBrains Mono', ui-monospace, Consolas, monospace;  /* data, ids, hashes */
```

Google Fonts load string (used site-wide):

```
https://fonts.googleapis.com/css2?family=Cinzel:wght@500;600;700;800;900&family=EB+Garamond:ital,wght@0,400;0,500;0,600;1,400&family=Inter:wght@400;500;600;700&family=JetBrains+Mono:wght@400;500&display=swap
```

Key rules (Cinzel reads thin, so always pin weights explicitly):

- **Display / h1–h3:** Cinzel **900**, uppercase, `letter-spacing: 0.04–0.06em`.
- **h4–h6:** Cinzel 800.
- **Tagline:** Cinzel 700, uppercase, `letter-spacing: 0.42em`, blue
  (gold-bright on dark surfaces).
- **Eyebrow:** Cinzel 700, `0.78rem`, `letter-spacing: 0.32em`, gold.
- **Body:** EB Garamond 400, 18px base, line-height 1.55.
- **Type scale:** display-xl `clamp(3rem,6vw+1rem,6rem)`, display-l
  `clamp(2.25rem,4vw+1rem,4rem)`, display-m `clamp(1.75rem,2vw+1rem,2.5rem)`.

## 5. Components / primitives

- **Buttons** (`.cc-btn`): pill (`border-radius:999px`), Cinzel 600,
  uppercase, `letter-spacing:0.18em`.
  - Primary: blue gradient `#1E5BFF → #0033AD`, ivory text.
  - Gold: `#F5D27A → #C9892A`, ink text.
  - Ghost: transparent, gold-deep border.
- **Card** (`.cc-card`): ivory bg, `--cc-veined` border, radius 18px, soft
  shadow; hover lifts to a gold border.
- **Section** (`.cc-section`): vertical rhythm `clamp(3rem,6vw,6rem)`;
  `.cc-section-marble` and `.cc-section-forum` (dark) variants.
- **Gold rule** (`.cc-rule`): centered ornament with gradient hairlines; the
  signature divider.
- **Geometry:** radii 6/10/18px; rules are 1px `--cc-gold-deep`.
- **Shadows:** always soft, never harsh (`--cc-shadow-1/2/gold`).
- **Focus:** 2px `--cc-blue-bright` outline, 3px offset; keep for a11y.

## 6. Reusing this in a standalone product

1. Copy this entire `brand-kit/` folder. It is self-contained: SVG marks
   (`logo/`), `brand.css` (tokens + base styles + primitives), the brand-kit
   reference page (`index.html`), and originals under `logo/_source/`.
2. Load the Google Fonts string above, then `brand.css`, then your app CSS.
3. For dark UI (the Cella login already uses this), use the `v9` mark +
   `--cc-forum` background + gold-bright accents.
4. Site identity strings: name "Cardano Curia", tagline / description
   "Integritas ante omnia".
5. Keep `brand.css` as the single source of truth; don't fork the tokens. If
   the brand evolves, update `brand.css` here first, then propagate.
