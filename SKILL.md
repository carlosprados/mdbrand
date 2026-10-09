---
name: mdbrand
description: Generate branded documents from Markdown with one command — A4 PDF reports, internal notes and letters with the client's logo on the cover and header; 16:9 slide decks for technical talks with printable speaker notes; the same document as a branded .docx for Word or Google Docs. D2 diagrams and Vega-Lite charts are rendered and sized legibly, tables and charts read YAML/JSON/CSV data files, citations resolve from a .bib, {{words}} counts to an essay's limit, and every trap that would print a defective PDF stops the build naming the fix. Ships worked examples to start from (mdbrand example). Use INSTEAD of hand-rolling pandoc/XeLaTeX invocations, LaTeX preambles, reference.docx files or diagram renders. Load when asked for a PDF, Word/.docx or Google Docs version, report, propuesta, oferta, informe, nota interna, memo, carta, ensayo, monografía, presentación, charla, slides or deck from Markdown; when a document needs a cover, letterhead or logo; when a diagram, chart or data table must go into a PDF; or when a pandoc PDF build misbehaves (missing characters, illegible or page-eating figures, blank logo).
---

# mdbrand — branded documents from Markdown

Never assemble a pandoc command line, a LaTeX preamble or a diagram render by
hand: that path rediscovers the same traps every time. mdbrand holds them, and
a build that would print a defective document fails naming the fix.

## The loop

1. **Start from the nearest worked example**, never from an empty file. Its
   front matter is right and commented, and it shows the syntax of everything
   it uses.

   ```sh
   mdbrand example                    # what there is, and what each is for
   mdbrand example carta ./out        # writes ./out/carta.md (+ any data/, refs.bib)
   mdbrand example charla --show      # read one without writing anything
   ```

2. **Adapt it.** Replace the content, keep the shape. Set `mdbrand.brand` to
   the identity asked for (`mdbrand brand list` shows the installed bundles);
   leave `brand: none` when no identity was asked for — it builds anywhere.
3. **Build once:** `mdbrand build out/carta.md`. Everything comes from the
   front matter; there are no flags to remember. `--watch` is for a person
   beside a PDF viewer and never returns — do not run it in the foreground.
4. **Act on the exit status**, not on the wording of the message:

   | Status | Who acts | What to do |
   |---|---|---|
   | 0 | — | Built. Read stderr anyway: warnings (`  ! …`) do not stop a build. |
   | 1 | The document or bundle | The message names the fix. Apply it to the `.md` (or the bundle) and build again. |
   | 2 | The command line | Fix the command: a flag, an argument, a file that is not there. |
   | 3 | The machine | A tool, font or bundle is missing. Run `mdbrand doctor` and give the user its install command; do not edit the document around the gap. |

5. **Look at the pages before calling it done.** A zero exit says the build
   ran, not that the document is right.

   ```sh
   pdftoppm -f 1 -l 2 -r 60 -png out/carta.pdf p   # then read p-1.png, p-2.png
   ```

6. **Report** the paths and the result line the build printed — it gives the
   pages and the word count with its criterion. Quote that count; never count
   words yourself.

## Which example

| Asked for | Example | Style and what it shows |
|---|---|---|
| A report, proposal or offer for a client | `informe` | `report`: cover with reference and confidential label, table of contents, diagram, chart, table, code |
| An internal note or memo; a Word or Google Docs version | `nota` | `note`, `formats: [pdf, docx]`: title block, header from page 1 |
| A formal letter | `carta` | `letter`: letterhead, recipient lines, place and date, subject, greeting, signature |
| Numbers kept in a spreadsheet, YAML or JSON | `datos` | `{{data…}}` values, ```` ```table ```` blocks, a CSV linked whole, a chart reading the same data |
| An essay with a word limit; anything with citations | `ensayo` | `{{words}}` on the cover, `{.nocount}`, `bibliography:` and `[@key, p. 12]` |
| Slides for a talk; speaker notes | `charla` | `slides`, `formats: [pdf, notes]`: sections, figures for the frame, columns, pauses, notes |

They combine: a proposal quoting a price list is `informe` with the data and
table blocks of `datos`.

## Commands

```sh
mdbrand build doc.md              # build from the front matter; -q prints only the result
mdbrand example [name] [dir]      # worked examples, carried in the binary
mdbrand new doc.md --style note   # a bare scaffold, when no example is near
mdbrand data doc.md               # every {{data…}} name the document can use
mdbrand diagrams doc.md           # figure sizes and label point size, without a build
mdbrand doctor                    # the toolchain, and the install command for what's missing
mdbrand brand list|show|validate|new
```

`mdbrand <cmd> --help` is the complete manual; read it rather than guess a flag.
Flags (`--brand`, `--style`, `--to`, `-o`, `--work`) override the front matter
for one build; a document should carry its own settings.

This file is embedded in the binary. If it looks out of step with the tool,
`mdbrand skill install --force` rewrites it from the installed binary, and
`mdbrand version` names that binary.

## Front matter

```yaml
---
title: "…"
subtitle: "…"                     # optional
author: "Departamento de Tecnología — Amplía Soluciones S.L."
date: "9 de octubre de 2026"
lang: es-ES                       # hyphenation, Tabla/Figura, 1.250,5 and charts in Spanish
toc: true                         # optional
numbersections: true              # optional; pandoc's own keys pass through
bibliography: refs.bib            # optional; one path or a list. Turns citations on
csl: apa.csl                      # optional, with a bibliography
mdbrand:
  brand: amplia                   # bundle name; none = built-in defaults
  style: report                   # report | note | letter | slides
  reference: "Oferta AS-2164-26"  # report cover, optional
  confidential: "Confidencial"    # report: cover + footers; note: title + footers; letter: under the date; slides: footer
  short_title: "Oferta Acme"      # the running header's title when the real one is too long
  formats: [pdf, docx]            # pdf (default) | docx | notes, in any combination
  wordcount: all                  # or {base: ib, include: [tables]}; default ib
  data: [precios/, extra.yaml]    # where {{data…}} reads; default data/ beside the document
  # letter only: to: [lines], place:, greeting:, signature: | (one line per line)
---
```

A key under `mdbrand:` that no option reads stops the build with "did you
mean". A title too long for the running header stops it asking for
`short_title`.

- `report` — cover with logo, header with logo from page 2, optional ToC.
- `note` — no cover; title block plus header from page 1.
- `letter` — letterhead; `title` prints as the subject line.
- `slides` — a 16:9 beamer deck. See *Slides*.

## Word count

Every build prints the count; `{{words}}` in the body, front matter or a
caption puts it in the document, grouped for `lang` (4.512 in es). The default
criterion is the IB's (Extended Essay, TOK): prose, lists, headings, block
quotes and content footnotes count; code, figures and captions, tables, math,
citations, citation-only notes and the bibliography do not. A heading marked
`{.nocount}` drops its section (appendix, abstract with its own limit), and so
does a `::: {.nocount}` div. `mdbrand build --help` lists the parts a
`wordcount: {base, include}` can add. Near a hard limit, leave a margin: word
processors disagree by a percent or two.

## Data

Values the document quotes (specs, prices, a client's name) live in `data/`
beside it — YAML, JSON, CSV, TSV — and print with `{{data.file.key}}` in the
prose, captions or front matter (quote that scalar):

- `{{data.maquinas[m5.large].ram}}` — brackets for keys with dots
- `{{data.equipo.0.nombre}}` — an index
- `{{data.ec2[t3.micro].vcpu}}` — a record by its `id` field

Values print literally, Markdown characters included; tag a YAML value `!md`
to have it read as Markdown. A wrong key stops the build listing the real ones.
Name anchor-only entries with a leading `_` (`_base: &base {…}`). Run
`mdbrand data doc.md` before writing placeholders instead of opening the files.
No loops or conditions: prepare that content upstream.

Never type a table whose rows are in a data file:

````markdown
```table
source: maquinas              # data path; a mapping's key becomes the id column
where: {familia: m5}          # equality only; or rows: [id, id] to pick and order
columns: {id: Tipo, ram: {label: RAM, unit: GiB}, precio: {label: "€/h", decimals: 3}}
sort: -ram                    # one field; - descends
total: [ram, precio]          # optional: an exact Total row
transpose: true               # optional: records become columns
caption: Instancias m5
```
````

Or `![Caption](data/file.csv)` alone on a line for a whole file. A CSV's
separator, BOM and decimal commas are handled; Latin-1 and ragged rows are
refused. A missing field fails the build: write `field: ~` for an empty cell.
A chart reads the same records with `"data": {"name": "maquinas"}` — prefer it
to a url when a table shows the same numbers.

Every table, data or hand-written, is kept together (no page break leaves fewer
than three rows on a side) and labelled *Tabla* in Spanish. Do not add
`\needspace` or raw LaTeX for either.

## Citations

`bibliography:` is the whole switch: `@key` and `[@key, p. 42]` resolve, and
the list lands where the document puts `::: {#refs}` (under a `# Referencias
{.nocount}` heading), or at the end. `csl:` picks the style. Paths resolve
against the document. A missing `.bib` or a key with no entry stops the build:
pandoc alone exits 0 and prints `(key?)` in the middle of a sentence.

## Diagrams and charts

D2 for architecture, sequence, state and flow; Vega-Lite for data. Inline fence
or side file, both rendered, sized and placed:

````markdown
```d2 caption="Arquitectura"
direction: right
api -> cola -> worker
```

![Latencia p95](diagrams/latencia.vl.json)
````

Fences: `d2`, `vegalite`/`vega`/`vl`. Attributes: `caption="…"`, `width=120mm`,
`scale=0.4`.

- **Sizing is the tool's.** Each figure is placed at the widest size that fits
  the measure, keeps its labels inside the brand's `min_text_pt..max_text_pt`
  and stays under `max_height_mm`. When that is impossible the build fails with
  fixes in order of preference. Never "fix" it by scaling the whole figure
  down; `mdbrand diagrams doc.md` shows the numbers without a build.
- **Do not write font sizes into a .d2.** mdbrand sets them on a copy of the
  source; declaring any `font-size` turns that off.
- **Only the root `direction:` shapes a d2 layout** — inside a container d2
  ignores it. `vars: { d2-config: { layout-engine: elk } }` packs some graphs
  tighter than dagre; look at the result before keeping it.
- **Colours come from the brand** where the source is silent: bars and shapes
  in the primary and its tints, lines darkened to 3:1 if needed, axes in the
  text colour. Do not paste colours or a d2 theme to brand a figure — what the
  source sets wins, so a hard-coded colour is what stops it following the
  bundle. `figures: {palette: [...]}` in brand.yaml fixes series colours.
- **Faces:** chart text is set in the body face when fontconfig has it; d2
  labels print in fontconfig's default sans (d2 only reads TrueType files).
- **Chart data** may be a file: `"data": {"url": "data/ventas.csv"}` resolves
  against the document (fence) or the spec (side file). A missing file or an
  `https://` URL stops the build — `vl2svg` would draw empty axes and exit 0.
  In a decimal-comma `lang` numbers print as that language writes them
  (`30.000`), and in Spanish so do month names.
- **Pictures:** an `.svg` made elsewhere is treated as a figure (sized, checked
  for legibility); PNG, JPEG and PDF keep their markup. A missing picture stops
  the build by name.
- **No `|md|` blocks in d2**: they become `<foreignObject>`, which the PDF
  route drops in silence, so the build refuses them.

## Code blocks

A block is set at the largest size whose longest line fits the measure,
stepping down to `\footnotesize`. A fence **with a language** wraps what still
does not fit, marked with an arrow; a **bare fence** is never wrapped, because
it may hold an ASCII diagram. So put the language on real code and leave it off
pictures. A block still too wide is warned about with the columns it has and
the columns that fit.

## Coloured words

`[words]{.accent}` sets them in the bundle's `colors.accent` (its primary
unless it names one), in the PDF and the `.docx`. That is the only colour:
`color=`, `colour=`, `style=` and a `::: {.accent}` block stop the build,
because pandoc drops them in silence. Use it for a phrase, not a paragraph.
Under 4.5:1 against white a deck stops and a page warns, naming a darker shade
to put in the bundle.

## Slides

`style: slides` makes a 16:9 deck (160 × 90 mm) from the same Markdown, bundle
and figures. Start from `mdbrand example charla`.

- `#` opens a section with a cover slide; `##` is one slide. Use `##` for every
  slide: text under a `#` alone becomes a slide titled by the section.
- `:::: columns` / `::: column` for two columns.
- `. . .` on a line of its own pauses; `::: incremental` around a list reveals
  it point by point. A slide with pauses is one slide: one number, judged by
  its fullest step.
- `::: notes` holds what to say; it never prints on a slide. With `notes` in
  `formats` (`formats: [pdf, notes]`, or `--to pdf,notes`) the build also
  writes `talk-notes.pdf`: every slide at half size beside its note, two to an
  A4 sheet, pauses shown whole. A note has room for 17 lines; a longer one
  stops the build by the slide's title.
- The footer numbers slides out of the total (`8 / 24`), covers included.
- Figures get 140 × 48 mm at most and a band of 8.5–14 pt: draw them wide and
  low (`direction: right`), with few edge labels.
- **A slide that does not fit stops the build**, by title and millimetres.
  Split it with another `##` or cut it. With one figure on the slide the error
  names the exact `width=NNmm` to put on its block, or says no width keeps its
  labels legible.
- A deck refuses `docx`; a page refuses `notes`.
- The bundle may add `slides: {background, foreground, logo, art, logo_width,
  logo_width_cover, diagrams: {min_text_pt, max_text_pt}}`; colours are held
  to WCAG contrast and a failing pair stops the deck.

## Word and Google Docs

`formats: [pdf, docx]` (or `--to pdf,docx`) writes a branded `.docx` beside the
PDF from the same document: cover, header, numbered captions, figures at their
PDF size, tables that never break a word, a filled table of contents. The PDF
stays the reference; offer the `.docx` when someone needs to edit or comment.

- A named bundle needs `fonts.office: {body, display, mono}` — faces the
  readers have (Google Docs: Google Fonts only). Without it the build stops and
  prints the block to add. Never put a licensed face there.
- A picture linked as PDF stops a `.docx` build: export it as SVG or PNG.
- Google Docs: `gog drive upload x.docx --convert-to doc`.
- It is one-way: carry edits made in Word or Docs back into the `.md`
  (`pandoc x.docx -t markdown` helps to see them).

## Brand bundles

`brands_dir` (see `mdbrand config`) holds one directory per identity:
`brand.yaml` plus a logo. `mdbrand brand new <name>` creates one;
`mdbrand brand validate <name>` catches what would print wrong — an SVG that
merely wraps a bitmap, artwork outside the `viewBox`, a `data:` URI that
converts to a blank page, a display font whose path moved, colours that are not
6-digit hex, slide colours under WCAG contrast.

- `requires: "0.17"` names the mdbrand a bundle needs; an older binary stops
  and names the release to upgrade to. A key no setting reads stops every build
  with that bundle, by its dotted path and the nearest real key: fix the key,
  never delete `requires:` to get past it.
- Tune `colors` (primary, accent, link, text, rule), `fonts.body`,
  `fonts.display`, `fonts.fallback`, `fonts.office`, `page.margin`,
  `page.linestretch`, `page.logo_width_*` and the `diagrams` numbers.
- `logo_secondary:` adds a second mark on the cover only (co-branding).
- **Do not set `page.headheight`.** mdbrand sizes the header from the logo's
  real height; a declared one too small fails the build with both fixes.
- **Never put a licensed font or a client logo in a repository that can be
  read anonymously.** A private or internal bundle may carry its font
  (`fonts.display.path: [fonts/otf]`, relative to the bundle); a public one
  leaves it out and lets fontconfig find an installed copy.

## Hard rules

- **A missing glyph fails the build**, naming the character and the font — a
  font without `☐` prints nothing at all. Fix the text, change `fonts.body`, or
  declare a `fonts.fallback` that covers it. `--allow-missing-glyphs` only when
  the holes are truly wanted.
- **The text layer must read as the text.** A PDF that prints "(SD1)" and
  copies as private-use characters fails the build: plagiarism checkers and
  screen readers read that layer.
- **`header-includes`, `include-before` and `include-after` are refused**, not
  applied: mdbrand owns the preamble, and pandoc would drop them in silence.
  Anything a document needs there belongs in the bundle. The rest of the front
  matter reaches pandoc untouched.
- **XeLaTeX only**, and mdbrand runs it: never call pandoc's PDF engine or d2's
  own PDF export by hand.

## When a build looks wrong

`--work ./out` keeps everything: the generated `.tex` and preamble, the
rewritten Markdown, every figure as SVG and PDF, and the full XeLaTeX log. Read
the log before theorising. `pdffonts x.pdf` says which faces were embedded;
`pdfinfo x.pdf` gives page size and count.

An overfull line is reported on stderr with how far it ran and the text it
could not fit — enough to find it in the Markdown. Do not discard stderr and
then call the document finished.

## Dependencies

`mdbrand doctor` checks all of these and prints, for anything missing, the
install command and the project's page.

- Always: [pandoc](https://pandoc.org), XeLaTeX from
  [TeX Live](https://tug.org/texlive/) (beamer and pgf for decks), and
  [rsvg-convert](https://gitlab.gnome.org/GNOME/librsvg), 2.41 or later: 2.40,
  the usual Windows build, prints d2's masks as black boxes and the build stops
  (exit 3). On Windows take MSYS2's: `pacman -S mingw-w64-x86_64-librsvg`.
- Documents with figures: [d2](https://d2lang.com) for diagrams;
  [vega-cli](https://github.com/vega/vega/tree/main/packages/vega-cli) and
  vega-lite for charts — 6.4.0 or later in a decimal-comma language.
- Recommended: `pdftotext` from [poppler](https://poppler.freedesktop.org/),
  which reads each PDF back to check its text layer.
- Fonts: the bundle's body face, found through fontconfig. A named bundle whose
  face is absent stops the build (exit 3); `brand: none` falls back to Latin
  Modern and builds on a bare machine.
