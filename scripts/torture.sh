#!/usr/bin/env bash
# Builds the fixture documents in testdata/ and reads what came out of them.
#
# The unit tests cover the arithmetic; this covers the artifact. Every defect
# this tool has shipped — a code block past the margin, a glob that refused to
# compile, a path resolved twice — was invisible to `go test` and obvious in a
# PDF or in a XeLaTeX log. So: one document that must come out clean, and a
# directory of documents that must each fail with the words that name the fix.
#
# Needs the full toolchain (`mdbrand doctor`). Run it before releasing; CI runs
# it on every push.
set -uo pipefail
# A CDPATH in the environment makes `cd` echo where it landed, which would end
# up inside the path below.
unset CDPATH

root="$(cd -P "$(dirname "$0")/.." && pwd)"
bin="$root/mdbrand"
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

failures=0
ok()   { printf '  \033[32mok\033[0m    %s\n' "$1"; }
bad()  { printf '  \033[31mFAIL\033[0m  %s\n' "$1"; failures=$((failures + 1)); }

[ -x "$bin" ] || { echo "no binary at $bin — run just build first"; exit 1; }

# ---------------------------------------------------------------- the clean one
echo "testdata/torture.md — must come out clean"
out="$("$bin" build "$root/testdata/torture.md" --brand none \
	--work "$work/torture" -o "$work/torture.pdf" 2>&1)"
status=$?
log="$work/torture/torture.log"

if [ $status -eq 0 ]; then ok "builds"; else
	bad "build failed (exit $status)"; printf '%s\n' "$out" | sed 's/^/        /'
fi

# A warning is a defect here: this document is made of the shapes that used to
# produce them. The one exception is the body-font fallback, and it is an
# exception on purpose — CI deliberately does not install Inter, so the default
# bundle's "works on a machine with nothing on it" path is exercised on every
# push instead of being asserted once and assumed forever.
warnings="$(printf '%s\n' "$out" | grep '^  !' | grep -v 'the default bundle set this document in Latin Modern')"
if [ -n "$warnings" ]; then
	bad "warnings, and this document must produce none:"
	printf '%s\n' "$warnings" | sed 's/^/        /'
else
	ok "no warnings beyond the body-font fallback"
fi

if [ -f "$log" ]; then
	n="$(grep -c 'Overfull \\hbox' "$log")"
	[ "$n" -eq 0 ] && ok "no overfull line" || bad "$n overfull line(s) in the log"
	n="$(grep -c 'Missing character' "$log")"
	[ "$n" -eq 0 ] && ok "no missing glyph" || bad "$n missing glyph(s) in the log"
else
	bad "no XeLaTeX log at $log"
fi

# Four figures, by three routes and four source shapes: a fence using vars, a
# side file that imports another, a Vega-Lite spec, and an SVG already rendered.
# The summary line, not the progress one: each figure is announced twice.
n="$(printf '%s\n' "$out" | grep -c '^  fig .*mm')"
[ "$n" -eq 4 ] && ok "four figures rendered" || bad "$n figure(s) rendered, want 4"

if command -v pdfinfo >/dev/null; then
	pages="$(pdfinfo "$work/torture.pdf" 2>/dev/null | awk '/^Pages:/{print $2}')"
	[ "${pages:-0}" -ge 2 ] && ok "$pages pages" || bad "PDF has ${pages:-no} pages, want 2 or more"
else
	echo "  --    pdfinfo absent, page count not checked"
fi

# Rendered is not placed. The counts above come from what mdbrand said; these
# come from the PDF, which is the only witness that matters: a figure can render
# perfectly and never reach the page, and a citation can resolve to "(key?)"
# while everything exits 0.
if command -v pdftotext >/dev/null; then
	text="$(pdftotext "$work/torture.pdf" - 2>/dev/null)"
	missing=""
	# No colon in the pattern: the caption separator comes back through the
	# font's ToUnicode map as something else entirely.
	for caption in "Figure 1" "Figure 2" "Figure 3" "Figure 4"; do
		printf '%s' "$text" | grep -qF "$caption" || missing="$missing $caption"
	done
	[ -z "$missing" ] && ok "all four figures placed in the PDF" \
		|| bad "the PDF has no$missing — rendered, but never placed"

	# The chart's labels come only from its CSV: an empty chart has axes and
	# no p99.
	printf '%s' "$text" | grep -qF "p99" \
		&& ok "the chart drew its data file" \
		|| bad "the chart has no p99 — its data never loaded"

	printf '%s' "$text" | grep -qF "Prados Hijón" \
		&& ok "the bibliography printed" \
		|| bad "the bibliography is not in the PDF"

	printf '%s' "$text" | grep -qE '\([A-Za-z0-9]+\?\)' \
		&& bad "an unresolved citation key printed as (key?)" \
		|| ok "no unresolved citation"
else
	echo "  --    pdftotext absent, PDF contents not checked"
fi

# ------------------------------------------------------------------ word count
# The number is worked out by hand in the document. What is asserted is the PDF:
# the count printed where {{words}} was, in the subtitle and in the body, and
# the placeholder left as written inside a code span.
echo
echo "testdata/wordcount.md — the count, as printed"
out="$("$bin" build "$root/testdata/wordcount.md" -o "$work/wordcount.pdf" 2>&1)"
status=$?
if [ $status -ne 0 ]; then
	bad "build failed (exit $status)"; printf '%s\n' "$out" | sed 's/^/        /'
elif command -v pdftotext >/dev/null; then
	text="$(pdftotext "$work/wordcount.pdf" - 2>/dev/null)"
	n="$(printf '%s\n' "$text" | grep -c '35 palabras')"
	[ "$n" -eq 2 ] && ok "35 words, printed in the subtitle and the body" \
		|| bad "want \"35 palabras\" twice in the PDF, found $n; the build said: $(printf '%s' "$out" | head -1)"
	printf '%s' "$text" | grep -qF '{{words}}' \
		&& ok "a placeholder in a code span left as written" \
		|| bad "the placeholder in a code span was filled"
else
	echo "  --    pdftotext absent, PDF contents not checked"
fi

# ------------------------------------------------------------------- the traps
# file · expected exit (ok|fail) · a phrase the message must carry · extra args
traps=(
	"wide-code.md|ok|columns, where"
	"illegible-figure.md|fail|below the 8.0pt floor"
	"illegible-figure.md|fail|layout-engine: elk"
	"missing-glyph.md|fail|no glyph for"
	"missing-citation.md|fail|no entry for"
	"header-includes.md|fail|header-includes"
	"missing-picture.md|fail|pictures not found"
	"missing-data.md|fail|data that is not there"
	"unknown-placeholder.md|fail|the ones that exist are {{words}}"
	"wordcount-typo.md|fail|the keys are base and include"
	"absent-body-font.md|fail|fontconfig cannot find it|--brand ghost --brands-dir $root/testdata/brands"
)

echo
echo "testdata/traps — each must fail, naming the fix"
for t in "${traps[@]}"; do
	IFS='|' read -r file want phrase extra <<<"$t"
	# shellcheck disable=SC2086 — the extra arguments are meant to split.
	out="$("$bin" build "$root/testdata/traps/$file" ${extra:---brand none} \
		-o "$work/${file%.md}.pdf" 2>&1)"
	status=$?
	case "$want" in
		ok)   [ $status -eq 0 ] || bad "$file: exit $status, want a build that warns and succeeds" ;;
		fail) [ $status -ne 0 ] || bad "$file: exit 0, want the build to stop" ;;
	esac
	if printf '%s\n' "$out" | grep -qF "$phrase"; then
		ok "$file says \"$phrase\""
	else
		bad "$file never says \"$phrase\". It said:"
		printf '%s\n' "$out" | sed 's/^/        /'
	fi
done

echo
if [ $failures -eq 0 ]; then
	echo "all clear"
else
	echo "$failures check(s) failed"
	exit 1
fi
