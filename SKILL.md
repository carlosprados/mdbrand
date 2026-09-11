---
name: mdbrand
description: Generate branded A4 PDFs from Markdown with one command — cover and running header with the client's logo, D2 diagrams and Vega-Lite charts rendered and sized legibly for paper, missing-glyph detection. Use INSTEAD of hand-rolling pandoc/XeLaTeX invocations, brand preambles or diagram renders. Load when asked for a PDF, report, propuesta, informe, nota interna, memo or carta from Markdown; when a document needs a cover, letterhead or logo header; when a diagram must go into a PDF; or when a pandoc PDF build misbehaves (missing characters, illegible or page-eating figures, blank logo).
---

# mdbrand — Markdown to a branded A4 PDF

Do not assemble a pandoc command line, a LaTeX preamble or a diagram render by
hand. That path re-derives the same decisions and rediscovers the same traps
every time. Use the CLI; it holds them all.

```sh
mdbrand build informe.md          # everything comes from the front matter
mdbrand new informe.md --toc      # scaffold with the front matter already right
mdbrand doctor                    # toolchain check + the install command for what's missing
mdbrand diagrams informe.md       # figure sizes and label point size, without a build
mdbrand brand list|show|validate|new
```

`mdbrand <cmd> --help` is the full manual. Read it instead of guessing flags.

This document is embedded in the binary. If it looks out of step with the tool,
`mdbrand skill install --force` rewrites it from the installed binary, and
`mdbrand version` says which one that is.

## Front matter is the interface

```yaml
---
title: "…"
subtitle: "…"                     # optional
author: "Departamento de Tecnología — Amplía Soluciones S.L."
date: "7 de septiembre de 2026"
lang: es-ES
toc: true                         # optional
numbersections: true              # optional; pandoc's own key
bibliography: refs.bib            # optional; one path or a list of them
csl: apa.csl                      # optional, alongside a bibliography
mdbrand:
  brand: amplia                   # bundle name; `none` for unbranded
  style: report                   # report | note | letter
  reference: "Oferta AS-2164-26"  # report, optional
  confidential: "Confidencial"    # report, optional
---
```

- `report` — cover with logo, header with logo from page 2, optional ToC.
- `note` — no cover; title block plus header from page 1. Internal notes.
- `letter` — letterhead; needs `to:` (list), `place:`, `greeting:`, `signature:`
  (block scalar, one line per line).

Overrides exist as flags (`--brand`, `--style`, `-o`, `--work`) but a document
should carry its own configuration so the build command never changes.

## Citations

Declaring `bibliography:` is the whole switch: mdbrand then runs pandoc with
`--citeproc`, so `@key` and `[@key, p. 42]` resolve and the list lands wherever
the document puts a `# Referencias` heading. `csl:` picks the style.

**Paths resolve against the document, not the working directory**, and a missing
`.bib` fails the build by name. So does a key with no entry: pandoc reports
those as warnings and still exits 0, which is how `(fml?)` ends up printed in
the middle of a sentence in a PDF nobody re-read.

## Diagrams

D2 for architecture, sequence, state and flow; Vega-Lite for data. Inline fence
or side file, both rendered and placed automatically:

````markdown
```d2 caption="Arquitectura"
**.style.font-size: 32
(** -> **)[*].style.font-size: 32
mesa: Mesa
mesa -> og.trainer: listas
```

![Latencia p95](diagrams/latencia.vl.json)
````

Fences: `d2`, `vegalite`/`vega`/`vl`. Attributes: `caption="…"`, `width=120mm`,
`scale=0.4`.

**Both glob lines are mandatory in every `.d2`.** `**` reaches shapes nested in
containers, `*` does not, so without them a nested box keeps the 16px default
and prints at 8px once the SVG is halved.

mdbrand places each figure at the widest size that fits the measure, keeps its
labels inside the brand's `min_text_pt..max_text_pt` band and stays under
`max_height_mm`. If that is impossible the build fails: raise the font size in
the source and lower the scale by the same factor (shrinks whitespace, not
text), or split the figure. Do not "fix" it by scaling the whole diagram down.

## Dependencies

`mdbrand doctor` checks every one of these and prints the install command plus
the project's own page for whatever is missing.

Always needed: **[pandoc](https://pandoc.org)**
([install](https://pandoc.org/installing.html)) ·
**[XeLaTeX](https://tug.org/texlive/)** from TeX Live, or
[MiKTeX](https://miktex.org) on Windows ·
**[rsvg-convert](https://gitlab.gnome.org/GNOME/librsvg)** from librsvg ·
the LaTeX packages [fancyhdr](https://ctan.org/pkg/fancyhdr),
[geometry](https://ctan.org/pkg/geometry),
[fontspec](https://ctan.org/pkg/fontspec),
[etoolbox](https://ctan.org/pkg/etoolbox),
[microtype](https://ctan.org/pkg/microtype),
[caption](https://ctan.org/pkg/caption),
[xcolor](https://ctan.org/pkg/xcolor).

Only for documents with figures: **[d2](https://d2lang.com)**
([install](https://d2lang.com/tour/install)) for diagrams ·
**[vl2svg](https://vega.github.io/vega-lite/)** from
[vega-cli](https://github.com/vega/vega/tree/main/packages/vega-cli) for charts.

Fonts: the bundle's body font must be installed — [Inter](https://rsms.me/inter/)
is the usual one — and is found through
[fontconfig](https://www.freedesktop.org/wiki/Software/fontconfig/).

Built with [Go](https://go.dev/dl/) 1.26+; prebuilt binaries are attached to
every [release](https://github.com/carlosprados/mdbrand/releases).

## Hard rules

- **XeLaTeX only.** pdflatex cannot take the Unicode. Not configurable.
- **A missing glyph fails the build**, naming the character and font — a font
  without `☐` prints nothing at all and only the log would know. Fix the text
  (`[ ]` also takes a pen tick better) or change `fonts.body`. Override only
  deliberately with `--allow-missing-glyphs`.
- **`header-includes` is refused, not applied.** So are `include-before` and
  `include-after`. mdbrand injects its preamble and cover through pandoc's
  `--include-in-header` and its two siblings, and a variable set on pandoc's
  command line replaces the metadata field of that name — the document's lines
  would vanish in silence. Anything a document needs in the preamble belongs in
  the brand bundle. Everything else in the front matter still reaches pandoc
  untouched (`numbersections`, `lang`, …).
- **No `|md|` blocks in d2.** They become `<foreignObject>`, which
  `rsvg-convert` drops silently. Short labels; prose in the document.
- **Never use d2's own PDF export** — it downloads a Playwright driver from
  dead URLs. The route is source → SVG → `rsvg-convert` → PDF, and it is
  already what mdbrand does.
- **Never put a licensed font or a client logo in a repository that can be read
  anonymously.** A bundle may carry its font (`fonts.display.path: [fonts/otf]`,
  relative paths resolve inside the bundle) only when the bundle's own
  distribution respects that licence — private or internal, never public.
  Otherwise leave it out: candidates expand `~` and `$VARS`, and mdbrand asks
  fontconfig for an installed copy. If the font is absent the cover falls back
  to the body font and the build still succeeds.

## Brand bundles

`brands_dir` (see `mdbrand config`) holds one directory per identity:
`brand.yaml` plus `logo.svg`. Create with `mdbrand brand new <name>`, then
`mdbrand brand validate <name>` — it catches an SVG that merely wraps a bitmap,
artwork outside the `viewBox`, the invalid `data:img/` MIME type that converts
to a blank page, `<foreignObject>`, a display font whose path has moved, and
colours that are not plain 6-digit hex.

Tune per bundle: `colors`, `fonts.body`, `fonts.display`, `page.margin`,
`page.linestretch`, `page.logo_width_cover|header`, and the `diagrams` numbers.

`logo_secondary:` adds a second mark at the right of the **cover only**, on the
same baseline as the first, the way a letterhead sets two identities — a
co-branded proposal, a funding body's emblem, an institution above a federation.
Width is `page.logo_width_cover_secondary`. The running header keeps one mark:
at 16 mm a second one is a smudge, not an identity.

**Do not set `page.headheight`.** mdbrand measures the logo and sizes the header
box from its real height, because `logo_width_header` is a width and a square
mark is four times taller than the old fixed default could hold — it overflowed
the box and printed over the first line of every page. A declared `headheight`
that cannot hold the mark fails the build with the two fixes; `brand validate`
prints the value actually in use.

## When a build looks wrong

`--work ./out` keeps the generated `preamble.tex`, `before.tex`, `after.tex`,
the rewritten Markdown, every figure and the full XeLaTeX log. Read the log
before theorising.
