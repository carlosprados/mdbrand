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
	# Links printed as body text: they worked, and nothing on the page said so.
	grep -qF 'urlcolor={brandLink}' "$work/torture/torture.tex" \
		&& ok "links are coloured, not hidden" \
		|| bad "the .tex does not colour links — pandoc's hidelinks is back"
	grep -qF 'hyperfootnotes=false' "$work/torture/torture.tex" \
		&& ok "footnote marks keep the text colour" \
		|| bad "footnote marks are painted as links"
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
	# The colon is in the pattern on purpose: it used to come back as U+EE47,
	# Inter's case form, which was the text-layer defect showing.
	for caption in "Figure 1:" "Figure 2:" "Figure 3:" "Figure 4:"; do
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

	# Inter's case forms printed right and copied as U+EE4E/U+EE4F. The build
	# refuses that itself now; this is the witness that the text survives.
	flat="$(printf '%s' "$text" | tr -s '[:space:]' ' ')"
	printf '%s' "$flat" | grep -qF "(SD1) and (Fox Business, 2026) and [ABC]" \
		&& ok "brackets beside capitals copy as themselves" \
		|| bad "brackets beside capitals do not survive text extraction"

	# The cover, top to bottom: reference under the subtitle, then author and
	# date at the foot, then the label. The .docx is held to the same order.
	pdftotext -f 1 -l 1 "$work/torture.pdf" - 2>/dev/null | tr -s '[:space:]' ' ' \
		| grep -qF "Ref. MDB-0001 mdbrand September 2026 Confidential & internal" \
		&& ok "the cover sets reference, author, date and label in order" \
		|| bad "the cover's reference, author, date and label are out of order"

	# The running header prints short_title; the cover keeps the full title.
	last="$(pdftotext -f "$pages" -l "$pages" "$work/torture.pdf" - 2>/dev/null)"
	printf '%s' "$last" | grep -qF "Torture, short" \
		&& ok "the running header prints short_title" \
		|| bad "the running header does not print short_title"

	# The cover's confidentiality label repeats in every footer: the last page
	# is the one furthest from the cover, so it is the witness.
	last="$(pdftotext -f "$pages" -l "$pages" "$work/torture.pdf" - 2>/dev/null)"
	printf '%s' "$last" | grep -qF "Confidential & internal" \
		&& ok "the confidentiality label prints in the footer of the last page" \
		|| bad "the last page has no confidentiality label in its footer"
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

# ------------------------------------------------------------------------ data
# Values from testdata/data/, printed where the document asked for them: in the
# title, the prose and a caption, merged keys included, and every Markdown
# character of the data printed as itself. The phrases avoid < > and the
# colon, which beside capitals once copied as Inter's private-use case forms;
# the build refuses that now, so the avoidance is history, not a limitation.
echo
echo "testdata/data.md — data values and tables, as printed"
out="$("$bin" build "$root/testdata/data.md" --work "$work/data" -o "$work/data.pdf" 2>&1)"
status=$?
if [ $status -ne 0 ]; then
	bad "build failed (exit $status)"; printf '%s\n' "$out" | sed 's/^/        /'
else
	# A generated table sized to the letter pushed "vCPU" past its column
	# by 3pt — under the warning threshold, so the log is read directly.
	warnings="$(printf '%s\n' "$out" | grep '^  !' | grep -v 'the default bundle set this document in Latin Modern')"
	[ -z "$warnings" ] && ok "no warnings" || bad "warnings: $warnings"
	n="$(grep -c 'Overfull \\hbox' "$work/data/data.log")"
	[ "$n" -eq 0 ] && ok "no overfull line, tables included" || bad "$n overfull line(s) in the log"
fi
if [ $status -eq 0 ] && command -v pdftotext >/dev/null; then
	text="$(pdftotext "$work/data.pdf" - 2>/dev/null | sed 's/\xc2\xa0/ /g' | tr -s '[:space:]' ' ')"
	for phrase in \
		"ofrece 4 vCPU, 16 GiB de RAM y 100 GB" \
		"a 0.192 €/hora" \
		"hereda 50 GB de disco de la familia m5" \
		'1.500 $ *neto* para @acme' \
		"[aparte] & ~ ^" \
		"Latencia para ACME" \
		"{{data.maquinas[m5.large].cpu}}" \
		"m5.xlarge 4 16 GiB 0,192" \
		"RAM 8 GiB 16 GiB 16 GiB Disco 50 GB 100 GB 100 GB" \
		"Instancias m5 para ACME" \
		"Ávila 5 2100 Burgos 4 1875,25" \
		"24 GiB 0,288"; do
		printf '%s' "$text" | grep -qF -- "$phrase" \
			&& ok "prints \"$phrase\"" \
			|| bad "the PDF lacks \"$phrase\""
	done
	# The three-row table used to split: caption, header and one row at the
	# foot of page 1, two rows alone on page 2. Wherever it lands, it lands
	# whole — the page holding its first row holds its last and its caption.
	pages="$(pdfinfo "$work/data.pdf" 2>/dev/null | awk '/^Pages:/{print $2}')"
	whole=""
	for p in $(seq 1 "${pages:-1}"); do
		pt="$(pdftotext -f "$p" -l "$p" "$work/data.pdf" - 2>/dev/null)"
		if printf '%s' "$pt" | grep -qF Zamora; then
			printf '%s' "$pt" | grep -qF Burgos && printf '%s' "$pt" | grep -qF "Tabla 3" && whole=yes
		fi
	done
	[ -n "$whole" ] && ok "a short table is never split across pages" \
		|| bad "the Sedes table is split across a page break"

	# Charted by name, the CSV's decimal commas are numbers: three bars. Read
	# as text, Vega-Lite drew Ávila alone and exited 0.
	n="$(grep -o 'sede: [^"]*"' "$work/data/fig01.svg" 2>/dev/null | wc -l)"
	[ "$n" -eq 3 ] && ok "the chart by name draws all three bars" \
		|| bad "the chart by name drew $n bar(s), want 3"

	printf '%s' "$text" | grep -qE 'Cuadro [0-9]' \
		&& bad "a table is labelled Cuadro" \
		|| ok "tables are labelled Tabla in Spanish"

	# A note with neither author nor date opened its line with a middle dot:
	# "· Uso interno". The footer prints the label alone too, so the
	# witness is the dot, not the label.
	first="$(pdftotext -f 1 -l 1 "$work/data.pdf" - 2>/dev/null)"
	printf '%s' "$first" | grep -qF '· Uso interno' \
		&& bad "a note's label line opens with a middle dot" \
		|| ok "a note's label stands alone when there is no author or date"
elif [ $status -eq 0 ]; then
	echo "  --    pdftotext absent, PDF contents not checked"
fi

# ------------------------------------------------------------------ the letter
# The letter style had no fixture until the .docx needed one: letterhead,
# recipient, place and date, greeting, a table, signature — on one page.
echo
echo "testdata/letter.md — the letter style"
out="$("$bin" build "$root/testdata/letter.md" -o "$work/letter.pdf" 2>&1)"
status=$?
if [ $status -eq 0 ]; then ok "builds"; else
	bad "build failed (exit $status)"; printf '%s\n' "$out" | sed 's/^/        /'
fi
if [ $status -eq 0 ] && command -v pdftotext >/dev/null; then
	pages="$(pdfinfo "$work/letter.pdf" 2>/dev/null | awk '/^Pages:/{print $2}')"
	[ "${pages:-0}" -eq 1 ] && ok "one page" || bad "the letter takes ${pages:-no} pages, want 1"
	text="$(pdftotext "$work/letter.pdf" - 2>/dev/null | tr -s '[:space:]' ' ')"
	# The letterhead page has no footer: the label lived nowhere on a
	# one-page letter.
	for phrase in "ACME Industrial S.A." "Madrid, 3 de octubre de 2026" "Confidencial" "Estimados señores:" "Ana Ruiz Sánchez"; do
		printf '%s' "$text" | grep -qF -- "$phrase" && ok "prints \"$phrase\"" || bad "the letter lacks \"$phrase\""
	done
fi

# -------------------------------------------------------------------- the docx
# The same fixtures as .docx files. A .docx has no log: it is laid out by
# whatever opens it. LibreOffice, rendering it to PDF, is the witness — not
# Word, not Google Docs, but it reads the same OOXML, and the defects that
# found this format (a word broken across a narrow column, a face nobody
# declared) show in what it draws.
echo
echo "docx — the fixtures as Word documents"
for f in torture data letter; do
	out="$("$bin" build "$root/testdata/$f.md" --brand none --to docx -o "$work/$f.docx" 2>&1)"
	status=$?
	if [ $status -eq 0 ]; then ok "$f.docx builds"; else
		bad "$f.docx: build failed (exit $status)"; printf '%s\n' "$out" | sed 's/^/        /'
	fi
	# torture's 101-column block fits LaTeX's mono at 8pt and no 0.6em face:
	# the .docx says so, which is the point of saying so.
	warnings="$(printf '%s\n' "$out" | grep '^  !' | grep -v 'code block(s) stay past the measure' \
		| grep -v 'were not checked against it')"
	[ -z "$warnings" ] && ok "$f.docx: no unexpected warning" || bad "$f.docx warns: $warnings"
	# pandoc writes a picture's path into pic:cNvPr, and mdbrand makes it
	# absolute: a handed-in document carried the author's home directory.
	if command -v unzip >/dev/null && [ $status -eq 0 ]; then
		unzip -p "$work/$f.docx" word/document.xml | grep -qE 'descr="(/|[A-Za-z]:\\)' \
			&& bad "$f.docx carries a local path in a picture's descr" \
			|| ok "$f.docx: no local path inside"
	fi
done

if command -v soffice >/dev/null && command -v pdffonts >/dev/null && command -v fc-match >/dev/null; then
	(cd "$work" && timeout 300 soffice --headless --convert-to pdf --outdir "$work/lo" \
		torture.docx data.docx letter.docx >/dev/null 2>&1)
	# Every face LibreOffice embedded must be what fontconfig gives for one of
	# the three the bundle declares. pandoc's reference names Aptos and
	# Consolas; either one sneaking back shows up here as a face of its own.
	allowed=""
	for face in "Arial" "Courier New"; do
		allowed="$allowed $(fc-match -f '%{family[0]}' "$face" | tr -d ' ')"
	done
	for f in torture data letter; do
		[ -f "$work/lo/$f.pdf" ] || { bad "LibreOffice did not render $f.docx"; continue; }
		stray=""
		# pdffonts prints PostScript names — ArialMT, CourierNewPS-BoldMT,
		# LiberationSans — which begin with the family, spaces dropped.
		for font in $(pdffonts "$work/lo/$f.pdf" | awk 'NR>2{print $1}' | sed 's/^[A-Z]*+//; s/-.*//' | sort -u); do
			known=""
			for a in $allowed; do case "$font" in "$a"*) known=yes ;; esac; done
			# Math is set in Word's own equation face, wherever the document has any.
			[ -z "$known" ] && [ "$font" != "OpenSymbol" ] && stray="$stray $font"
		done
		[ -z "$stray" ] && ok "$f.docx: only the declared faces ($allowed )" \
			|| bad "$f.docx: faces nobody declared:$stray"
	done

	text="$(pdftotext "$work/lo/data.pdf" - 2>/dev/null | sed 's/\xc2\xa0/ /g' | tr -s '[:space:]' ' ')"
	# The note's title block carried no author, date or label in the .docx:
	# only the PDF printed them.
	# Twice on the first page: under the title and in the footer.
	n="$(pdftotext -f 1 -l 1 "$work/lo/data.pdf" - 2>/dev/null | grep -c 'Uso interno')"
	[ "$n" -eq 2 ] && ok "data.docx: the note's label line, as the PDF has it" \
		|| bad "data.docx: \"Uso interno\" $n time(s) on page 1, want 2 (title block and footer)"
	for phrase in "m5.xlarge" "m5.large" "Uso recomendado" "Tabla 1:" "Figura 1:" "Ávila 5 2100"; do
		printf '%s' "$text" | grep -qF -- "$phrase" && ok "data.docx prints \"$phrase\" whole" \
			|| bad "data.docx lacks \"$phrase\" — a word broken across a column?"
	done
	pages="$(pdfinfo "$work/lo/data.pdf" 2>/dev/null | awk '/^Pages:/{print $2}')"
	whole=""
	for p in $(seq 1 "${pages:-1}"); do
		pt="$(pdftotext -f "$p" -l "$p" "$work/lo/data.pdf" - 2>/dev/null)"
		if printf '%s' "$pt" | grep -qF Zamora; then
			printf '%s' "$pt" | grep -qF Burgos && printf '%s' "$pt" | grep -qF "Tabla 3" && whole=yes
		fi
	done
	[ -n "$whole" ] && ok "data.docx: a short table is never split across pages" \
		|| bad "data.docx: the Sedes table is split across a page break"
	pages="$(pdfinfo "$work/lo/letter.pdf" 2>/dev/null | awk '/^Pages:/{print $2}')"
	[ "${pages:-0}" -eq 1 ] && ok "letter.docx: one page" || bad "letter.docx takes ${pages:-no} pages, want 1"
	pdftotext "$work/lo/letter.pdf" - 2>/dev/null | grep -qF "Confidencial" \
		&& ok "letter.docx: the confidentiality label prints" \
		|| bad "letter.docx: the confidentiality label is nowhere in the letter"
	text="$(pdftotext "$work/lo/torture.pdf" - 2>/dev/null | tr -s '[:space:]' ' ')"
	# The .docx cover put reference and label together under the subtitle, in
	# bold primary; the PDF sets the label at the foot.
	printf '%s' "$text" | grep -qF "Ref. MDB-0001 mdbrand September 2026 Confidential & internal" \
		&& ok "torture.docx: the cover follows the PDF's order" \
		|| bad "torture.docx: the cover's reference, author, date and label are out of the PDF's order"
	pages="$(pdfinfo "$work/lo/torture.pdf" 2>/dev/null | awk '/^Pages:/{print $2}')"
	pdftotext -f "$pages" -l "$pages" "$work/lo/torture.pdf" - 2>/dev/null | grep -qF "Confidential & internal" \
		&& ok "torture.docx: the confidentiality label prints in the last page's footer" \
		|| bad "torture.docx: the last page has no confidentiality label in its footer"
	printf '%s' "$text" | grep -qF "Contents What this is Code blocks" \
		&& ok "torture.docx: the contents are filled before Word updates them" \
		|| bad "torture.docx: the table of contents is empty"
else
	echo "  --    soffice, pdffonts or fc-match absent: the .docx files were built, not read"
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
	"unknown-data-key.md|fail|the ones that exist are m5.large, m5.xlarge"
	"table-typo.md|fail|has no field \"famila\"; the fields are"
	"chart-mixed-data.md|fail|coste: 1 number(s) and \"1250,5\", \"1875,25\""
	"unknown-placeholder.md|fail|the ones that exist are {{words}}"
	"wordcount-typo.md|fail|the keys are base and include"
	"absent-body-font.md|fail|fontconfig cannot find it|--brand ghost --brands-dir $root/testdata/brands"
	"absent-body-font.md|fail|names no fonts.office|--to docx --brand ghost --brands-dir $root/testdata/brands"
	"pdf-picture.md|fail|a .docx cannot hold one|--to docx --brand none"
	"span-colour.md|fail|[words]{.accent}"
	"span-colour.md|fail|[words]{.accent}|--to docx --brand none"
	"display-glyph.md|fail|display face (fonts.office.display), has no glyph|--to docx --brands-dir $root/testdata/brands"
	"confidential-too-long.md|fail|too wide for the footer"
	"confidential-too-long.md|ok|the .docx footer fits about|--to docx --brand none"
	"title-too-long.md|fail|Set mdbrand.short_title"
	"unknown-option.md|fail|did you mean confidential?"
)

echo
echo "testdata/traps — each must fail, naming the fix"
for t in "${traps[@]}"; do
	IFS='|' read -r file want phrase extra <<<"$t"
	# shellcheck disable=SC2086 — the extra arguments are meant to split.
	ext=pdf
	case "$extra" in *"--to docx"*) ext=docx ;; esac
	out="$("$bin" build "$root/testdata/traps/$file" ${extra:---brand none} \
		-o "$work/${file%.md}.$ext" 2>&1)"
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

# A format that fails publishes nothing, not even the formats before it: a new
# PDF beside the last build's .docx is a pair that no longer matches.
mkdir -p "$work/atomic"
cp -r "$root/testdata/traps/pdf-picture.md" "$root/testdata/traps/pictures" "$work/atomic/"
if "$bin" build "$work/atomic/pdf-picture.md" --to pdf,docx --brand none >/dev/null 2>&1; then
	bad "pdf-picture.md --to pdf,docx: exit 0, want the build to stop"
elif [ -e "$work/atomic/pdf-picture.pdf" ]; then
	bad "pdf-picture.md --to pdf,docx failed on the .docx but published the PDF"
else
	ok "pdf-picture.md --to pdf,docx publishes nothing when the .docx fails"
fi

echo
if [ $failures -eq 0 ]; then
	echo "all clear"
else
	echo "$failures check(s) failed"
	exit 1
fi
