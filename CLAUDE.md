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

Non-goals: a general pandoc wrapper, HTML or slide output, a template language
for users. It renders documents that look like the ones in `examples/`.

## Layout

```
main.go                  //go:embed SKILL.md (a directive cannot reach outside
                         //  its own package, and SKILL.md belongs at the root)
cmd/                     cobra commands; the help text IS the manual
internal/brand/          brand.yaml: parsing, defaults, validation, font resolution
internal/doc/            front matter, and extracting figures from the body
internal/fig/            diagram source -> SVG -> PDF, and the print sizing maths
internal/tex/            templates/*.tmpl + escaping + page arithmetic
internal/build/          the pipeline; owns the xelatex run and its log
internal/run/            external commands, with their output on failure
```

The one-way dependency is `cmd -> build -> {brand, doc, fig, tex} -> run`.

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
9. **Never write through a symlink.** `skill install` uses `os.Lstat`: in a
   checkout the installed skill is a symlink to this repository's `SKILL.md`.
10. **Licensed fonts and client logos never enter a repository that can be read
    anonymously.** A bundle *may* carry its font when the bundle's own
    distribution respects the licence (private or internal). That is a licensing
    judgement, and the tool must not make it quietly on someone's behalf.

Tests must not depend on what the machine has installed. Two did: one asserted
against a real Gotham that only exists on one laptop, another was rescued by a
fontconfig hit. Use invented face names like `MdbrandTestFace-Regular.otf`.

## Working on it

```sh
make build          # or: go build -o mdbrand .
make check          # gofmt -w . && go vet ./... && go test ./...
make example        # builds examples/demo.md with the built-in default bundle
make install        # into ~/.local/bin, version from git describe
make skill          # dev symlink of SKILL.md into ~/.claude/skills/mdbrand
```

`make example` matters: it uses `--brand none`, so it proves the tool works on a
machine with no bundle configured. Run it before releasing.

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
tests, cross-compiles linux/darwin/windows × amd64/arm64 with no cgo, archives
each with LICENSE and README, writes checksums and publishes the release.

```sh
make check && make example
git tag -a vX.Y.Z -m "…" && git push origin vX.Y.Z
gh run watch --exit-status "$(gh run list --workflow=release --limit 1 --json databaseId --jq '.[0].databaseId')"
```

`ci.yml` runs gofmt/vet/test on every push to main, and deliberately uses the
same action versions as the release workflow so a bad bump surfaces there first.

## Keep these in step

A user-visible change usually touches four places, and forgetting one is the
common failure:

- **README.md** — for people installing and using it.
- **SKILL.md** — for agents. It is embedded in the binary, so a release ships
  it; re-run `mdbrand skill install --force` after upgrading a copy.
- **`--help` text** — treated as the manual, both for humans and for agents
  driving the tool without reading a file.
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
