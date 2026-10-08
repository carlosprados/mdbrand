# CLAUDE.md — working on mdbrand

Instructions for an agent working **on this codebase**. What the tool does and
how to *use* it is in [README.md](README.md) and [SKILL.md](SKILL.md); do not
restate that here.

## What this is, and what it refuses to be

A Go CLI that turns Markdown into a branded A4 PDF: pandoc for Markdown→LaTeX,
XeLaTeX for LaTeX→PDF, a brand bundle for the identity, D2 and Vega-Lite figures
rendered and sized so their text is legible on paper.

It exists because every A4 document used to mean re-deriving the same decisions
and rediscovering the same traps. So the traps are behaviour, not documentation:
**a build that would produce a defective PDF must fail, naming the fix.** Adding
a knob that lets a defect through is the wrong direction.

Non-goals: a general pandoc wrapper, HTML output, a .pptx, slide layouts of
the author's own, a template language for users. It renders documents that look
like the ones in `examples/`, and decks that look like `testdata/slides.md`.

The PDF is the product. `--to docx` writes the same document for readers who
need Word or Google Docs, and it is strictly additive: nothing on the PDF's path
may change for it. A change near `internal/build` proves that by building every
fixture before and after and diffing the generated `.tex`, Markdown and the
PDFs' text — byte for byte. `style: slides` is held to the same rule: a deck
for technical talks, on the PDF's own path, where the page's output does not
move by a byte. The preamble pieces both share live in `partials.tex.tmpl`.

## Layout

```
main.go                  //go:embed SKILL.md (a directive cannot reach outside
                         //  its own package, and SKILL.md belongs at the root)
cmd/                     cobra commands; the help text IS the manual
internal/brand/          brand.yaml: parsing, defaults, validation, font resolution
internal/doc/            front matter, and extracting figures from the body
internal/data/           data/ files and the {{data…}} placeholders
internal/mdtext/         where Markdown prose ends and code or math begins
internal/fig/            diagram source -> SVG -> PDF, and the print sizing maths
internal/tex/            templates/*.tmpl + escaping + page arithmetic;
                         slides.tex.tmpl is the deck's preamble, beside the page's
internal/docx/           reference.docx from the brand, cover and letterhead
                         as OOXML, repairs over pandoc's .docx; a pure leaf
internal/build/          the pipeline: prepare.go is what every output format
                         shares (brand, data, words, sized figures); pdf.go is
                         the PDF's own, and owns the xelatex run and its log;
                         docx.go is the .docx's
internal/run/            external commands, with their output on failure
scripts/torture.sh       builds the fixtures and reads the PDFs and logs
testdata/                torture.md, which must come out clean, and
                         traps/, which must each fail naming the fix
```

Imports run one way, and `.golangci.yml` enforces it with depguard rather than
trusting this paragraph — which, unenforced, had drifted from the code:

- `cmd` is the composition root and may import any package.
- `internal/build` is the only orchestrator. No other internal package imports
  it, and none imports `cmd`, so a second output format can sit beside the
  first without either knowing about the other.
- `os/exec` is imported only by `internal/run`, which is what puts a failed
  tool's own output into the error.

Existing exceptions and complexity debt are listed by name under `exclusions`
in `.golangci.yml`. Remove an entry when its function is next touched; add one
only with the reason written beside it.

## Invariants. Each of these was a real defect; each has a test

Do not relax one without understanding what it cost.

1. **XeLaTeX, always.** pdflatex cannot take the Unicode these documents carry.
   Not a flag, not configurable.
2. **mdbrand runs xelatex itself**, rather than pandoc's `--pdf-engine`, because
   pandoc discards the engine's log and that log is the only place a missing
   glyph is reported. A font without `☐` prints *nothing*.
   → a missing glyph fails the build, naming character and font.
3. **Figures are sized, never merely scaled.** A figure is placed at the widest
   width that keeps its labels inside `min_text_pt..max_text_pt` and its height
   under `max_height_mm`. If that is impossible the build fails with the fix
   (raise the source font size, lower the scale by the same factor). Scaling a
   diagram down takes its text with it — that is the failure, not the remedy.
   → `internal/fig/fig_test.go` pins the px→pt→mm chain.
4. **Source → SVG → `rsvg-convert` → PDF, for both D2 and Vega-Lite.** d2's own
   PDF export downloads a Playwright driver from URLs that 404. `vl2pdf` writes
   points equal to the spec's pixels, so mixing routes puts two charts with the
   same spec width 33% apart on the page.
5. **`<foreignObject>` is refused before rendering.** `rsvg-convert` drops it
   silently, so a d2 `|md|` block would vanish from the PDF with no warning.
6. **Settings resolve flag > front matter > configuration**, through
   `build.Pick`. Inverting this made a document declaring `brand: none` build
   with the reader's configured brand.
   → `internal/build/build_test.go`.
7. **Every face of a display font is verified and resolved independently.**
   Checking only the regular and assuming the bold sits beside it produced a
   build that died inside fontspec *after* `brand validate` had said ok.
   → `internal/brand/brand_test.go`.
8. **No machine's absolute path in anything shared.** `fonts.display.path` is a
   candidate list; relative entries resolve inside the bundle, `~` and `$VARS`
   expand, and fontconfig is the last resort. This bug shipped twice.
   Expansion lives in `internal/paths` alone. Its mirror image is a path that
   silently turns relative: with no home, `configDir`, `skillDir` and the
   default brands directory came out relative, and `config init`, `skill
   install` and `brand new` wrote into the working directory while
   `brand.Load` read a local `acme/` as the bundle. Each now refuses by name.
   And a relative `--brands-dir` reached `rsvg-convert`, which runs in the
   work directory, as a logo path that led nowhere: `brand.Load` makes the
   bundle's directory absolute.
   → `internal/paths/paths_test.go`, `cmd/home_test.go`,
   `internal/brand/brand_test.go`.
9. **Never write through a symlink.** `skill install` uses `os.Lstat`: in a
   checkout the installed skill is a symlink to this repository's `SKILL.md`.
10. **Licensed fonts and client logos never enter a repository that can be read
    anonymously.** A bundle *may* carry its font when the bundle's own
    distribution respects the licence (private or internal). That is a licensing
    judgement, and the tool must not make it quietly on someone's behalf.

    The same rule covers working notes. **Handoffs from other sessions are never
    tracked** — `/docs/` is ignored. They quote the documents that found the
    defect, which are client documents under someone's brand, and this
    repository is public. What a handoff is worth keeping for belongs here as an
    invariant, in the tool's own words; the note itself stays on the machine.

11. **The built-in bundle must build on a machine with nothing on it.** It
    named Inter as its body face and died inside fontspec where Inter was
    absent — on a CI runner, which is how it was found at all. A named bundle
    whose body face is missing must stop, because a substituted face makes a
    document that is not the one the bundle describes; `--brand none` has no
    identity to betray and falls back to Latin Modern with a warning.
    → `internal/brand/brand_test.go`, and `testdata/traps/absent-body-font.md`.

12. **A chart's data urls are settled before vl2svg sees them.** vl2svg
    treats a file it cannot open as a warning: empty axes, exit 0. It also
    resolves urls against its own base — the work directory, for a fenced
    block — and prefixes that base even to an absolute path. So every
    `data.url` is resolved against `Fig.BaseDir`, checked, and handed over as
    `file://`; a remote URL is refused. `Loading failed` in its output fails
    the build as well.
    → `internal/fig/vega_test.go`, and `testdata/traps/missing-data.md`.

13. **A data value prints as its characters, and a path that leads nowhere
    stops the build.** Values are held as YAML nodes so `0.10` is not
    reformatted, escaped so `$` or `*` from a spreadsheet does not become
    Markdown, and put into the front matter's YAML tree rather than its text.
    A CSV's separator is detected, its BOM stripped, Latin-1 and ragged rows
    refused. No loops or conditions: that is the template language this tool
    is not. Tables likewise: equality filter, one sort key, `decimals`
    rounded through `big.Rat` and never a float, a decimal comma read as a
    number only where `lang` writes one (else an English 1,234 is 1.234),
    collated sorting, and a column never narrower than its longest word — a
    table sized to the letter overflowed by 3pt, under the warning threshold.
    Totals are exact and refuse a blank cell. A chart's `"data": {"name": …}`
    reads the records through the same parser, and a field it plots that
    mixes numbers and text fails: Vega-Lite drew Ávila alone, exit 0, when
    an English document read a Spanish CSV.
    → `internal/data/*_test.go`, `internal/fig/vega_test.go`,
    `testdata/data.md`, and `testdata/traps/unknown-data-key.md`,
    `table-typo.md`, `chart-mixed-data.md`.

14. **No page break leaves fewer than `tex.TableKeep` rows of a table on
    either side.** pandoc writes every table as a longtable, which may break
    after any row: a three-row table put its caption, header and first row at
    the foot of a page. mdbrand rewrites the row ends of pandoc's `.tex` to
    `\\*` where a break would orphan rows, between pandoc and xelatex — the
    one place both hand-written and data tables pass. In Spanish, tables are
    *Tabla*: babel's default *Cuadro* is appended over in the preamble,
    because pandoc loads babel before it and `es-tabla` cannot be passed.
    → `internal/tex/tables_test.go`, and the page checks on `testdata/data.md`.

15. **A .docx asks only for the faces the bundle declares, and uses only what
    Google Docs keeps.** A .docx carries no fonts, so `fonts.office` is
    required of a named bundle, and Repair refuses a package naming any other
    face: pandoc's reference names Aptos and Consolas, which Word and Docs
    substitute in silence. Cover and letterhead are inline paragraphs, table
    rules sit on cells, sizes are direct formatting: Google's importer drops
    anchored frames and table styles. Columns are never narrower than their
    longest word, code measured in the mono face — `report` broke in Docs when
    it was measured as prose. Every `pPr`/`rPr` is normalised to schema order
    and checked, because Word alone refuses a file out of order and nothing
    here runs Word: pandoc 3.1 writes `<w:bCs/>` before `<w:b/>`.
    pandoc's bullets are private-use characters in Symbol and Wingdings, so
    Repair redraws every bulleted level in the body face; until it did, no
    document with a list built as a .docx, and no fixture had one.
    → `internal/docx/docx_test.go`, the docx section of `scripts/torture.sh`
    (read back through LibreOffice), `traps/pdf-picture.md`.

16. **The PDF's text layer reads as its text.** Inter's contextual
    alternates swap `( ) [ ] { } : < >` beside capitals for case forms its
    cmap puts at private-use code points, and xdvipdfmx builds ToUnicode
    from that cmap: "(SD1)" printed right and reached Turnitin as U+EE4E SD1
    U+EE4F, in every Inter document. calt is off through pandoc's
    `mainfontoptions` — pandoc loads the face a second time through
    `\babelfont`, so a `\setmainfont` of our own is undone — and on the
    display faces. The PDF is read back with pdftotext, and a private-use
    character the source does not hold stops the build.
    → `internal/build/build_test.go`, the brackets check in `testdata/torture.md`.

17. **A colour pandoc would drop is refused.** `color=`, `colour=` and
    `style=` vanish on both writers, so the words printed black and the build
    exited 0. `[words]{.accent}` is the one colour, the brand's
    `colors.accent` (its primary by default), through
    `internal/build/spans.lua` for both outputs; the filter reports everything
    else on stderr and the build stops naming `.accent`. It also reports each
    accent it sets, and only then is the colour measured against white: Amplía's
    orange is 2.4:1 as words. Under 4.5:1 a deck stops and a page warns — pale
    accents have always printed, and a document that built must still build —
    both naming the darker shade of the same hue that passes. The filter names
    `brandAccent` only when the bundle declares one, so a bundle without it
    keeps its `.tex` byte for byte.
    → `internal/build/build_test.go`, `internal/build/slides_test.go`,
    `traps/span-colour.md`, `traps/accent-pale.md`.

18. **A slide that does not fit stops the build, by its title.** beamer sets
    the excess over the footer or off the page and exits 0; the log's
    `Overfull \vbox … at line N` points at the frame's `\end{frame}`, the
    title is read from its `\begin{frame}`, and the cover slides — which are
    not frame environments — are named as such rather than as the slide
    before them. Figures on a slide are sized for the frame (140 × 48 mm,
    8.5–14 pt) and written with both width and height, because pandoc's
    beamer writer otherwise adds `keepaspectratio` and shrinks a tall one in
    silence. Display faces take `Ligatures=TeX` there, since pandoc writes a
    frame title's em dash as `---`; the caption package, which pandoc loads
    for any table, numbered every caption until the deck set its label
    format itself. A cover ground is held to WCAG contrast: 4.5:1 for its
    type, 3:1 for each fill of an SVG logo.
    → `internal/build/slides_test.go`, `internal/brand/slides_test.go`,
    `internal/tex/tex_test.go`, `testdata/slides.md`, `traps/slide-*.md`.

Tests must not depend on what the machine has installed. Two did: one asserted
against a real Gotham that only exists on one laptop, another was rescued by a
fontconfig hit. Use invented face names like `MdbrandTestFace-Regular.otf`.

## Working on it

```sh
just build          # or: go build -o mdbrand .
just check          # gofmt -w . && go vet && golangci-lint && go test
just lint           # golangci-lint alone, pinned in CI to the same version
just torture        # builds testdata/ and reads what came out — needs the toolchain
just example        # builds examples/demo.md with the built-in default bundle
just install        # into ~/.local/bin, version from git describe
just skill          # dev symlink of SKILL.md into ~/.claude/skills/mdbrand
just shots          # regenerates the README pictures from examples/ (freeze, vhs, ffmpeg)
```

`just example` matters: it uses `--brand none`, so it proves the tool works on a
machine with no bundle configured.

**`just torture` matters more, and is the check to reach for first.** Every
defect this tool has shipped was invisible to `go test` and plain in a PDF or a
XeLaTeX log, so the fixtures assert on those instead of on Go values:
`testdata/torture.md` gathers every shape that has ever gone wrong and must come
out with no warning, no overfull line and no missing glyph, and every document
in `testdata/traps/` must fail with the words that name the fix. When a real
document finds something new, the fix is not finished until its shape is in
`testdata/`. Both run in CI, and a release will not publish without them.

**When a build misbehaves, `--work ./out` keeps everything**: the generated
`preamble.tex`, `before.tex`, `after.tex`, the rewritten Markdown, every figure
and the full XeLaTeX log. Read the log before theorising.

Verifying a change to the LaTeX or the sizing means looking at the PDF, not at
the exit code: `pdftoppm -f 1 -l 1 -r 75 -png out.pdf p` and open it.
`pdffonts out.pdf` says which faces were really embedded; `pdfinfo` gives page
size and count.

### The template gotcha

`internal/tex/templates/*.tmpl` are Go templates producing LaTeX, so **a doubled
opening brace anywhere — comments included — starts a template action.** Write
`\begingroup … \endgroup` instead of a nested brace group. And a trailing `%`
before a `{{-` trim swallows the rest of the line, closing brace and all.

## Releasing

A `v*` tag is the whole procedure. `.github/workflows/release.yml` runs the
tests, cross-compiles linux and darwin × amd64/arm64 plus windows/amd64 with no
cgo, archives each with LICENSE and README, writes checksums and publishes the
release. windows/arm64 is left out on purpose: add it to the target list only
when someone asks for it.

```sh
just check && just torture && just example
git tag -a vX.Y.Z -m "…" && git push origin vX.Y.Z
gh run watch --exit-status "$(gh run list --workflow=release --limit 1 --json databaseId --jq '.[0].databaseId')"
```

`ci.yml` runs gofmt/vet/lint/test on every push to main, and deliberately uses the
same action versions as the release workflow so a bad bump surfaces there first.
Both also build the fixtures, through the composite action in
`.github/actions/document-toolchain`, which pins the d2 version because the
fixtures assert behaviour that belongs to it.

## Keep these in step

A user-visible change usually touches four places, and forgetting one is the
common failure:

- **README.md** — for people installing and using it.
- **SKILL.md** — for agents. It is embedded in the binary, so a release ships
  it; re-run `mdbrand skill install --force` after upgrading a copy.
- **`--help` text** — treated as the manual, both for humans and for agents
  driving the tool without reading a file.
- **README pictures** — `just shots` whenever the result line, an error message
  or the examples change; a screenshot of output the tool no longer prints is
  documentation that lies.
- **`mdbrand doctor`** — every dependency it names carries an install command
  *and* the project's own page. Check a URL with curl before writing it down;
  a dead link in an install guide is worse than none.

## Conventions

- Code, comments, identifiers, docs and commit messages in **English**.
- Comments explain **why**, and especially what a line is defending against.
  The code says what it does.
- Tests after implementing, and only where a failure would be silent: the sizing
  maths, precedence, path resolution, escaping. No trivial tests.
- Never commit or push without being asked. Present the change first.
- No `Co-Authored-By` trailers.
