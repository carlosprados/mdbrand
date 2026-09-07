# mdbrand

Markdown in, branded A4 PDF out, in one command.

```console
$ mdbrand build informe.md
informe.pdf  (7 pages, brand amplia, style report)
  fig arquitectura.d2      38×63 mm   text 12.0pt
  fig latencia.vl.json    127×60 mm   text 12.0pt
```

pandoc and XeLaTeX do the typesetting. mdbrand supplies the part you otherwise
rebuild from scratch every time: a cover with your logo, a running header with
your logo, D2 diagrams and Vega-Lite charts rendered and **sized so their text
is legible on paper**, and a build that refuses to hand you a PDF with holes in
it.

The identity lives in a *brand bundle* outside the tool — a directory with a
`brand.yaml`, a logo and colours — so the same document publishes under another
identity by changing one word.

## Why it exists

Every A4 document meant re-deriving the same decisions and re-discovering the
same traps. They are now the tool's behaviour, not something to remember:

| The trap | What mdbrand does |
|---|---|
| `pdflatex` chokes on the Unicode these documents are full of | XeLaTeX always; the engine is not a flag |
| A font without the glyph prints **nothing**, and only the LaTeX log knows (`DejaVu Serif` has no `☐` or `⏱`; Latin Modern no `↔` or `≈`) | mdbrand runs XeLaTeX itself, reads the log, and **fails** naming each missing character and font |
| A d2 `\|md\|` block becomes a `<foreignObject>` and `rsvg-convert` drops it silently | Detected before rendering; the build stops and says why |
| d2's own PDF export downloads a Playwright driver from URLs that 404 | Never used: source → SVG → `rsvg-convert` → PDF |
| `vl2pdf` writes points equal to the spec's pixels, so charts come out 33% off the SVG route | Same SVG route for both, so 1px = 0.75pt everywhere |
| A diagram scaled to fit takes its text down with it — under ~5 cm tall on A4 nothing is readable | Every figure is placed inside a legibility band and the build fails, with the fix, if it cannot be |
| A vector logo that is really a 120×51 px PNG in an SVG coat, or artwork outside the `viewBox` that converts to a blank page | `mdbrand brand validate` diagnoses both |
| A dark-themed diagram landing on white paper because the SVG asked the reader's OS | Both d2 themes pinned light; no dark-mode rules injected into Vega output |

## Install

### From a release

Every `v*` tag builds and publishes binaries automatically, for Linux, macOS and
Windows on amd64 and arm64:

```sh
v=v0.1.0
curl -fsSL "https://github.com/carlosprados/mdbrand/releases/download/$v/mdbrand_${v}_linux_amd64.tar.gz" | tar xz
install -Dm755 mdbrand ~/.local/bin/mdbrand
```

Checksums are attached to each release as `mdbrand_<version>_checksums.txt`.

### From source

Go 1.26 or newer:

```sh
go install github.com/carlosprados/mdbrand@latest
```

Or from a clone:

```sh
git clone https://github.com/carlosprados/mdbrand
cd mdbrand && make install     # builds and installs into ~/.local/bin
```

Then check the toolchain — `doctor` prints the install command for anything
absent:

```sh
mdbrand doctor
```

### Dependencies — Linux

Required for every build:

| Tool | Debian / Ubuntu | Fedora | Arch |
|---|---|---|---|
| pandoc | `apt install pandoc` | `dnf install pandoc` | `pacman -S pandoc` |
| XeLaTeX | `apt install texlive-xetex` | `dnf install texlive-xetex` | `pacman -S texlive-xetex` |
| rsvg-convert | `apt install librsvg2-bin` | `dnf install librsvg2-tools` | `pacman -S librsvg` |

LaTeX packages: `fancyhdr`, `geometry`, `fontspec`, `etoolbox`, `microtype`,
`caption`, `xcolor`. On Debian/Ubuntu:

```sh
apt install texlive-latex-base texlive-latex-recommended texlive-latex-extra texlive-xetex
```

Required only if a document contains diagrams:

| Tool | Install |
|---|---|
| d2 | `curl -fsSL https://d2lang.com/install.sh \| sh -s --` (or `brew install d2`) |
| vl2svg | `npm i -g vega-cli vega-lite` |

Fonts: the body font named in the bundle must be installed and must cover the
glyphs you type. `Inter` is a good default (`apt install fonts-inter`, or
[rsms.me/inter](https://rsms.me/inter/)). A licensed display font is referenced
by path and never copied anywhere.

### Dependencies — Windows

Phase 2: not yet verified on Windows. The pieces exist — MiKTeX or TeX Live
ships `xelatex`, pandoc and d2 have Windows builds, `librsvg` is the awkward one
and usually arrives through MSYS2. Expect to install:

```powershell
winget install JohnMacFarlane.Pandoc
winget install MiKTeX.MiKTeX
scoop install d2
npm i -g vega-cli vega-lite
# rsvg-convert: MSYS2  ->  pacman -S mingw-w64-x86_64-librsvg
```

Paths in `brand.yaml` are absolute, so a bundle written on Linux needs its
`fonts.display.path` adjusted. Reports of what actually breaks are welcome.

## Use

### One document, no flags

Front matter carries everything, so the command stays the same forever:

```yaml
---
title: "Iniciativas de Inteligencia Artificial"
subtitle: "Qué tendremos, cómo las usaremos y en qué se basa"
author: "Departamento de Tecnología — Amplía Soluciones S.L."
date: "7 de septiembre de 2026"
lang: es-ES
toc: true
toc-depth: 2
mdbrand:
  brand: amplia
  style: report          # report | note | letter
  reference: "Oferta AS-2164-26"
  confidential: "Confidencial"
---
```

```sh
mdbrand new informe.md --title "…" --toc   # scaffold, front matter already right
mdbrand build informe.md                   # build
```

Flags override the front matter when you need a one-off: `--brand`, `--style`,
`-o`, `--work` (keep the LaTeX, figures and log for inspection).

### Styles

| Style | Shape |
|---|---|
| `report` | Cover page with logo, orange rule, title, subtitle, author and date; running header with logo from page 2; optional table of contents |
| `note` | No cover: title block at the top of page 1, header from page 1. For internal notes |
| `letter` | Letterhead, recipient block, place and date, subject, greeting; signature appended after the body |

`letter` takes extra front matter:

```yaml
mdbrand:
  style: letter
  to: ["Nombre del destinatario", "Empresa", "Dirección"]
  place: "Madrid"
  greeting: "Estimado equipo:"
  signature: |
    Carlos Javier Prados Hijón
    Amplía Soluciones S.L.
```

### Diagrams and charts

Write them inline, or link a side file. Both are rendered, placed and sized:

````markdown
```d2 caption="Arquitectura del motor"
**.style.font-size: 32
(** -> **)[*].style.font-size: 32
mesa: Mesa de Teleservicios
og: OpenGate { trainer; inferencer }
mesa -> og.trainer: listas
```

![Latencia por percentil](diagrams/latencia.vl.json)
````

Fence languages: `d2`, and `vegalite` / `vega` / `vl`. Side files: `*.d2`,
`*.vl.json`, `*.vl.yaml`, `*.vega.json`. Per-figure attributes on the fence or
after the link: `caption="…"`, `width=120mm`, `scale=0.4`.

**The font-size globs matter.** `**` reaches shapes nested inside containers;
`*` matches one level only, so without the double star a nested box keeps d2's
16px default and lands at 8px once the SVG is halved. Every `.d2` needs both
lines.

**How a figure gets its size.** The width is the largest that satisfies all
three bounds from the bundle: the text measure, `max_text_pt` (so a diagram does
not shout over the body text) and `max_height_mm` (so it does not eat the page).
Then the smallest label must still print at `min_text_pt` or more. If it cannot,
the build fails and tells you to raise the font size in the source and lower the
scale by the same factor — which shrinks the layout whitespace instead of the
text — or to split the figure.

Iterate on a diagram without rebuilding the document:

```console
$ mdbrand diagrams informe.md
brand amplia · measure 166.0 mm · text band 8.0–12.0 pt · max height 150 mm

arquitectura.d2               37.7 × 62.8   mm   text 12.0 pt
latencia.vl.json             126.7 × 60.2   mm   text 12.0 pt
```

## Brand bundles

A bundle is a directory. It lives **outside** any code repository, because a
corporate logo and a commercially licensed font must not be committed or
redistributed — the display font is referenced by absolute path and never
copied in.

```
<brands dir>/amplia/
  brand.yaml
  logo.svg
```

```sh
mdbrand brand new amplia        # scaffold it, then it tells you what to drop in
mdbrand brand validate          # diagnose every bundle
mdbrand brand list
mdbrand brand show amplia       # resolved settings, defaults included
mdbrand brand path amplia
```

`brand.yaml`:

```yaml
name: amplia
display_name: Amplía Soluciones S.L.
logo: logo.svg                 # vector; an SVG wrapping a PNG will look soft

colors:
  primary: "F68E1B"            # rules and accents — 6-digit hex, no '#'
  text: "5D6266"               # cover and header type
  rule: "C8CCCE"               # hairlines

fonts:
  body: Inter                  # fontconfig family
  display:                     # optional; cover and header only
    family: Gotham
    path: /path/to/gotham/otf  # by path — never copied into the bundle
    regular: Gotham-Light.otf
    bold: Gotham-Medium.otf

page:
  papersize: a4
  margin: 22mm
  linestretch: 1.125
  logo_width_cover: 46mm
  logo_width_header: 16mm

diagrams:
  d2_theme: 0                  # light: paper has no prefers-color-scheme
  d2_scale: 0.5                # pairs with **.style.font-size: 32
  min_text_pt: 8
  max_text_pt: 12
  max_height_mm: 150

footer: ""                     # optional line under the cover rule
```

A missing display font is not an error: the cover and header fall back to the
body font, so a bundle stays usable on a machine that does not have the licensed
face installed.

## Configuration

```sh
mdbrand config          # what is in effect and where it came from
mdbrand config init     # write a starter config file
```

`$XDG_CONFIG_HOME/mdbrand/config.yaml` (default `~/.config/mdbrand/config.yaml`):

```yaml
brands_dir: /home/you/Dropbox/3-Resources/brands
brand: amplia
style: report
```

Resolution order, later wins: built-in default → config file → `MDBRAND_*`
environment → flag. So `MDBRAND_BRANDS_DIR=/tmp/brands mdbrand build x.md`
works, and so does `--brands-dir`.

## Troubleshooting

**`the font has no glyph for N character(s)`** — the text uses a character the
body font lacks; it would print as nothing. Change the character (`[ ]` beats a
missing `☐`, and takes a ballpoint tick better) or set a `fonts.body` that
covers it. `--allow-missing-glyphs` proceeds anyway.

**`would print its smallest label at 6.2pt`** — the figure cannot be placed
legibly. Raise the font size in the source and lower the scale by the same
factor, or split it.

**`renders a <foreignObject>`** — a d2 `|md|` block. Keep labels short and put
the prose in the document.

**`converted to an empty PDF`** — the logo's artwork sits outside its `viewBox`,
or it uses the invalid `data:img/` MIME type. `mdbrand brand validate` says
which. If the artwork is off-canvas, find its bounding box with
`inkscape --query-all file.svg` and rewrite the root `viewBox` around it.

**`N line(s) overflow the measure`** — a wide table or an unbreakable URL. Note
that a Markdown formatter which re-normalises pipe tables to the content width
can strangle columns in the PDF; widen the separators before rebuilding.

**Something looks wrong in the LaTeX** — `--work ./out` keeps `preamble.tex`,
`before.tex`, `after.tex`, the rewritten Markdown, every figure and the full
XeLaTeX log.

## For agents

`mdbrand --help` and `mdbrand <command> --help` are written to be the whole
manual: an agent can drive the tool from help output without reading this file.
There is also a Claude Code skill in [`SKILL.md`](SKILL.md).

## Licence

MIT — see [LICENSE](LICENSE). The tool is MIT; the brand bundles you point it at
are yours, and neither logos nor licensed fonts are part of this repository.
