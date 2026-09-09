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

[Go](https://go.dev/dl/) 1.26 or newer:

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

| Tool | What it does here | Debian / Ubuntu | Fedora | Arch |
|---|---|---|---|---|
| **[pandoc](https://pandoc.org)** ([install](https://pandoc.org/installing.html)) | Markdown → LaTeX | `apt install pandoc` | `dnf install pandoc` | `pacman -S pandoc` |
| **[XeLaTeX](https://tug.org/texlive/)** (or [MiKTeX](https://miktex.org)) | LaTeX → PDF, with Unicode | `apt install texlive-xetex` | `dnf install texlive-xetex` | `pacman -S texlive-xetex` |
| **[rsvg-convert](https://gitlab.gnome.org/GNOME/librsvg)** | SVG → vector PDF for figures | `apt install librsvg2-bin` | `dnf install librsvg2-tools` | `pacman -S librsvg` |

LaTeX packages, all on [CTAN](https://ctan.org):
[fancyhdr](https://ctan.org/pkg/fancyhdr),
[geometry](https://ctan.org/pkg/geometry),
[fontspec](https://ctan.org/pkg/fontspec),
[etoolbox](https://ctan.org/pkg/etoolbox),
[microtype](https://ctan.org/pkg/microtype),
[caption](https://ctan.org/pkg/caption) and
[xcolor](https://ctan.org/pkg/xcolor). On Debian/Ubuntu they arrive with:

```sh
apt install texlive-latex-base texlive-latex-recommended texlive-latex-extra texlive-xetex
```

Required only if a document contains diagrams:

| Tool | What it does here | Install |
|---|---|---|
| **[d2](https://d2lang.com)** | Renders `.d2` diagrams ([install guide](https://d2lang.com/tour/install), [language tour](https://d2lang.com/tour/intro)) | `curl -fsSL https://d2lang.com/install.sh \| sh -s --` (or `brew install d2`) |
| **[vl2svg](https://vega.github.io/vega-lite/)** | Renders Vega-Lite charts ([CLI source](https://github.com/vega/vega/tree/main/packages/vega-cli), [chart docs](https://vega.github.io/vega-lite/docs/)) | `npm i -g vega-cli vega-lite` |

Fonts: the body font named in the bundle must be installed and must cover the
glyphs you type. [Inter](https://rsms.me/inter/) is a good default
(`apt install fonts-inter`). Installed fonts are discovered through
[fontconfig](https://www.freedesktop.org/wiki/Software/fontconfig/), so
`fc-cache -f` after dropping files into `~/.local/share/fonts` is all it takes.

### Dependencies — Windows

Phase 2: not yet verified on Windows. The pieces exist — MiKTeX or TeX Live
ships `xelatex`, pandoc and d2 have Windows builds, `librsvg` is the awkward one
and usually arrives through MSYS2. Expect to install:

```powershell
winget install JohnMacFarlane.Pandoc     # https://pandoc.org/installing.html
winget install MiKTeX.MiKTeX             # https://miktex.org
scoop install d2                         # https://d2lang.com/tour/install
npm i -g vega-cli vega-lite              # https://vega.github.io/vega-lite/
# rsvg-convert: MSYS2 -> pacman -S mingw-w64-x86_64-librsvg
#   MSYS2: https://www.msys2.org  ·  librsvg: https://gitlab.gnome.org/GNOME/librsvg
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
numbersections: true
bibliography: refs.bib   # optional: one path, or a list of them
csl: apa.csl             # optional, alongside a bibliography
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

### Citations

Declaring `bibliography:` is the whole switch: pandoc runs with `--citeproc`, so
`@key` and `[@key, p. 42]` resolve and the reference list lands where the
document puts its `# Referencias` heading. `csl:` selects the style.

Paths resolve **against the document**, not against the working directory, and a
`.bib` that is not there fails the build by name. So does a citation key with no
entry: pandoc reports those as warnings and exits 0 anyway, which is how `(fml?)`
ends up printed in the middle of a sentence.

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
  fonts/otf/          optional, and only if you may redistribute the font
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
logo_secondary: cliente.svg    # optional; right of the COVER only, same baseline

colors:
  primary: "F68E1B"            # rules and accents — 6-digit hex, no '#'
  text: "5D6266"               # cover and header type
  rule: "C8CCCE"               # hairlines

fonts:
  body: Inter                  # fontconfig family
  display:                     # optional; cover and header only
    family: Gotham
    regular: Gotham-Light.otf
    bold: Gotham-Medium.otf
    path:                      # candidates; first existing wins, ~ and $VARS expand
      - $MDBRAND_FONT_DIR
      - ~/.local/share/fonts/gotham

page:
  papersize: a4
  margin: 22mm
  linestretch: 1.125
  logo_width_cover: 46mm
  logo_width_cover_secondary: 24mm
  logo_width_header: 16mm       # one mark only: at 16mm a second is a smudge
  # headheight: 22pt            # usually omit: derived from the logo's height
  # headsep: 13pt               # gap between the header rule and the text

diagrams:
  d2_theme: 0                  # light: paper has no prefers-color-scheme
  d2_scale: 0.5                # pairs with **.style.font-size: 32
  min_text_pt: 8
  max_text_pt: 12
  max_height_mm: 150

footer: ""                     # optional line under the cover rule
```

**Leave `headheight` out unless you mean it.** `logo_width_header` is a width,
but what the running header has to reserve is a *height*, and the two differ by
the logo's proportions. mdbrand measures the logo and sets the box itself, so a
square crest gets the room it needs instead of printing across the first line of
every page. Declare `headheight` only to make the header taller than the mark
needs; declare one that cannot hold the mark and the build fails, naming both
ways out. `mdbrand brand validate` reports the value actually used.

**A shared bundle must not carry one machine's absolute path.** `path` is a list
of candidates and the first that exists wins. A **relative** candidate resolves
inside the bundle, so a bundle may ship the font next to `brand.yaml`
(`path: [fonts/otf]`) and a fresh clone then works with nothing installed —
whether you *may* ship it is a licensing question, and a commercial face must
never land in a repository that anyone can read anonymously. Absolute candidates
expand `~` and `$VARS`, so a bundle can name `$MDBRAND_FONT_DIR` and leave the
choice to each machine. If no candidate matches, mdbrand asks **fontconfig**
where that file actually is, which means a font installed the normal way needs
no `path` at all:

```sh
mkdir -p ~/.local/share/fonts/gotham && cp *.otf ~/.local/share/fonts/gotham/
fc-cache -f
```

A missing display font is not an error either: the cover and header fall back to
the body font, so a bundle stays usable on a machine that does not have the
licensed face installed — which is most machines, by design. `mdbrand brand
validate` prints where it looked and every way to fix it.

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

There is also an agent skill — [`SKILL.md`](SKILL.md) — which states the front
matter, the diagram rules and the traps, so an assistant uses the CLI instead of
improvising a pandoc command line. **It is embedded in the binary**, so a person
who installed a release can install it without cloning anything:

```sh
mdbrand skill install              # ~/.claude/skills/mdbrand/SKILL.md
mdbrand skill install --project    # ./.claude/skills/mdbrand/SKILL.md
mdbrand skill show                 # print it, to pipe elsewhere
mdbrand skill path                 # where install would write
```

Re-run it after upgrading the binary: the installed copy carries the version
that wrote it, so a stale one is visible. Installing refuses to overwrite a
SKILL.md that differs unless you pass `--force`, because in a checkout of this
repository that file is a symlink to the source and writing through it would
edit the repository.

## Licence

MIT — see [LICENSE](LICENSE). The tool is MIT; the brand bundles you point it at
are yours, and neither logos nor licensed fonts are part of this repository.
