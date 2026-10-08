#!/usr/bin/env bash
# Regenerates the pictures in README.md from the examples, so they show what the
# tool prints today and not what it printed when someone last took a screenshot.
#
#   assets/readme/pages.png      the report's cover and first page, the essay's cover
#   assets/readme/slides.png     four slides of the example deck
#   assets/readme/build.png      a build, as the terminal shows it
#   assets/readme/trap.png       a build that stops, naming the fix
#   assets/readme/wordcount.png  the essay counted by two criteria
#   assets/readme/watch.gif      --watch through an edit, a mistake and its fix
#
# Needs the document toolchain plus freeze and vhs (github.com/charmbracelet),
# ffmpeg, ImageMagick and poppler-utils. Everything runs on copies in a temporary
# directory, with --brand none: no bundle, logo or licensed font can end up in a
# picture that is published with this repository.
set -euo pipefail
unset CDPATH

root="$(cd -P "$(dirname "$0")/.." && pwd)"
out="$root/assets/readme"
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

for t in freeze vhs ffmpeg convert pdftoppm; do
	command -v "$t" >/dev/null || { echo "missing $t — see the header of $0"; exit 1; }
done
[ -x "$root/mdbrand" ] || { echo "no binary at $root/mdbrand — run just build first"; exit 1; }
export PATH="$root:$PATH"

# capped <max-memory> <command>...: an ffmpeg graph with an infinite input
# (-loop 1, loop=-1) never reaches EOF, and palettegen buffers every frame
# until it does — one grew to 52 GB and took the desktop down with it. So every
# recorder and encoder runs under a memory cap and a clock, and a mistake kills
# itself rather than the machine. No swap: with it the desktop freezes first.
capped() {
	local mem="$1"; shift
	if command -v systemd-run >/dev/null; then
		systemd-run --user --scope -q -p MemoryMax="$mem" -p MemorySwapMax=0 timeout 120 "$@"
	else
		prlimit --as="$(numfmt --from=iec "$mem")" timeout 120 "$@"
	fi
}

mkdir -p "$out"
# The examples come out of the binary, as anyone using it gets them.
for name in informe ensayo charla; do "$root/mdbrand" example "$name" "$work" >/dev/null; done
cp "$root/testdata/traps/missing-glyph.md" "$work/"
cd "$work"

# One look for every terminal picture, in a monospaced face so the result
# columns line up. JetBrains Mono when it is installed; otherwise DejaVu Sans
# Mono, which nearly every Linux machine has. Naming an absent face is not an
# option: freeze falls back to a proportional one and the columns go ragged.
# (Asked of fontconfig directly: `fc-list | grep -q` under pipefail fails when
# grep stops reading early and fc-list dies of SIGPIPE, i.e. whenever it finds.)
if [ -z "${SHOTS_FONT:-}" ] && [ -n "$(fc-list 'JetBrains Mono' family)" ]; then
	font="JetBrains Mono"
else
	font="${SHOTS_FONT:-DejaVu Sans Mono}"
fi
# freeze draws PNGs from the face's file and ignores the family name alone.
font_file="$(fc-match -f '%{file}' "$font:style=Regular")"

# shot <output> <command>...: each command is shown after a prompt, then run.
shot() {
	local file="$1"; shift
	: >shot.sh
	for c in "$@"; do
		printf 'printf "\\033[32m$\\033[0m %%s\\n" %q\n%s || true\n' "$c" "$c" >>shot.sh
	done
	# The commands run here and freeze only draws their output. Left to run
	# them itself with --execute, freeze now and then waits forever on a
	# terminal whose command has long exited.
	bash shot.sh 2>&1 | timeout 60 freeze --language ansi \
		--window --border.radius 8 --padding 20,24 --margin 0 \
		--font.family "$font" --font.file "$font_file" --font.size 14 --theme dracula \
		-o "$out/$file" >/dev/null
	echo "  $file"
}

echo "pictures in assets/readme"

mdbrand build informe.md -q >/dev/null
mdbrand build ensayo.md -q >/dev/null
pdftoppm -r 70 -png -f 1 -l 2 informe.pdf demo
pdftoppm -r 70 -png -f 1 -l 1 ensayo.pdf ensayo
convert demo-1.png demo-2.png ensayo-1.png \
	-bordercolor '#b8bcc0' -border 1 \
	-bordercolor none -border 12 +append \
	"$out/pages.png"
echo "  pages.png"

mdbrand build charla.md -q >/dev/null
# One page at a time: pdftoppm pads the number to the page count's width, so
# deck-1.png became deck-01.png the day the deck reached ten pages.
for n in 1 3 4 6; do pdftoppm -r 60 -png -f $n -l $n -singlefile charla.pdf deck-$n; done
# Each slide framed on its own, as the pages above are: a white slide on a
# white README is otherwise a rectangle of nothing.
for n in 1 3 4 6; do convert deck-$n.png -bordercolor '#b8bcc0' -border 1 -bordercolor none -border 6 frame-$n.png; done
convert \( frame-1.png frame-3.png +append \) \( frame-4.png frame-6.png +append \) -append \
	-bordercolor none -border 6 "$out/slides.png"
echo "  slides.png"

shot build.png "mdbrand build informe.md"
shot trap.png "mdbrand build missing-glyph.md"
shot wordcount.png "mdbrand build ensayo.md -q" "mdbrand build ensayo.md -q --wordcount all"

# The watch: an edit that adds six words, a misspelt placeholder that stops the
# build and keeps the last good PDF, and the fix. The edits run from a
# background job started out of frame, so the recording shows only what a
# person would see.
cat >edits.sh <<'EDITS'
sleep 7
# In the conclusion: appended at the end, it would land in the {.nocount}
# appendix and, quite correctly, change nothing.
sed -i 's/nunca cuentan exactamente igual\./& Una frase nueva de seis palabras./' ensayo.md
sleep 5
sed -i 's/{{words}} palabras"/{{palabras}} palabras"/' ensayo.md
sleep 5
sed -i 's/{{palabras}} palabras"/{{words}} palabras"/' ensayo.md
EDITS

# No BorderRadius or WindowBar: vhs would then merge a mask looped with
# loop=-1 and no shortest=1 — the same unbounded graph capped() exists for.
cat >watch.tape <<TAPE
Output "$work/frames/"
Set Shell bash
Set Framerate 20
Set FontFamily "$font"
Set FontSize 16
Set Width 1100
Set Height 380
Set Theme "Dracula"
Set TypingSpeed 40ms
Hide
Type "cd '$work' && export PATH='$root':\$PATH && PS1='\\\\$ ' && (bash edits.sh &) && clear"
Enter
Show
Type "mdbrand build ensayo.md -q -w"
Enter
Sleep 21s
Ctrl+C
Sleep 1s
TAPE
capped 3G vhs watch.tape >vhs.log 2>&1 || { cat vhs.log; exit 1; }
# vhs is asked for frames, and ffmpeg assembles the GIF: vhs 0.12 renders the
# frames and then exits 0 without ever running ffmpeg, leaving no GIF behind.
# A palette built from the frames keeps the file small and the text crisp, and
# the padding vhs would add goes in here, in the terminal's own background.
[ -e frames/frame-text-00001.png ] || { echo "vhs wrote no frames:"; cat vhs.log; exit 1; }
capped 2G ffmpeg -y -loglevel error -framerate 20 -i frames/frame-text-%05d.png \
	-framerate 20 -i frames/frame-cursor-%05d.png \
	-filter_complex "[0][1]overlay,pad=iw+48:ih+40:24:20:color=0x1d1d27,split[a][b];[a]palettegen=stats_mode=diff[p];[b][p]paletteuse=dither=none" \
	"$out/watch.gif"
echo "  watch.gif"
