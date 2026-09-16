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

[ -x "$bin" ] || { echo "no binary at $bin — run make build first"; exit 1; }

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
# produce them.
if printf '%s\n' "$out" | grep -q '^  !'; then
	bad "warnings, and this document must produce none:"
	printf '%s\n' "$out" | grep '^  !' | sed 's/^/        /'
else
	ok "no warnings"
fi

if [ -f "$log" ]; then
	n="$(grep -c 'Overfull \\hbox' "$log")"
	[ "$n" -eq 0 ] && ok "no overfull line" || bad "$n overfull line(s) in the log"
	n="$(grep -c 'Missing character' "$log")"
	[ "$n" -eq 0 ] && ok "no missing glyph" || bad "$n missing glyph(s) in the log"
else
	bad "no XeLaTeX log at $log"
fi

# Three figures, by two render routes and three source shapes: a fence using
# vars, a side file that imports another, and a Vega-Lite spec.
# The summary line, not the progress one: each figure is announced twice.
n="$(printf '%s\n' "$out" | grep -c '^  fig .*mm')"
[ "$n" -eq 3 ] && ok "three figures rendered" || bad "$n figure(s) rendered, want 3"

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
	for caption in "Figure 1" "Figure 2" "Figure 3"; do
		printf '%s' "$text" | grep -qF "$caption" || missing="$missing $caption"
	done
	[ -z "$missing" ] && ok "all three figures placed in the PDF" \
		|| bad "the PDF has no$missing — rendered, but never placed"

	printf '%s' "$text" | grep -qF "Prados Hijón" \
		&& ok "the bibliography printed" \
		|| bad "the bibliography is not in the PDF"

	printf '%s' "$text" | grep -qE '\([A-Za-z0-9]+\?\)' \
		&& bad "an unresolved citation key printed as (key?)" \
		|| ok "no unresolved citation"
else
	echo "  --    pdftotext absent, PDF contents not checked"
fi

# ------------------------------------------------------------------- the traps
# file · expected exit (ok|fail) · a phrase the message must carry
traps=(
	"wide-code.md|ok|columns, where"
	"illegible-figure.md|fail|below the 8.0pt floor"
	"illegible-figure.md|fail|layout-engine: elk"
	"missing-glyph.md|fail|no glyph for"
	"missing-citation.md|fail|no entry for"
	"header-includes.md|fail|header-includes"
)

echo
echo "testdata/traps — each must fail, naming the fix"
for t in "${traps[@]}"; do
	IFS='|' read -r file want phrase <<<"$t"
	out="$("$bin" build "$root/testdata/traps/$file" --brand none \
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
