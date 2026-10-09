package fig

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/carlosprados/mdbrand/internal/exit"
	"github.com/carlosprados/mdbrand/internal/run"
)

// RsvgMaskMin is the first librsvg that draws <mask>. 2.40 was the last
// release in C, and the rsvg-convert most Windows installs carry: it paints
// every mask as a black box, and d2 masks every labelled connection.
var RsvgMaskMin = [2]int{2, 41}

// RsvgFix is the remedy a build and doctor both name.
const RsvgFix = "install a current librsvg; on Windows, MSYS2's: pacman -S mingw-w64-x86_64-librsvg  ·  https://packages.msys2.org/base/mingw-w64-librsvg"

var rsvgVersionRe = regexp.MustCompile(`version (\d+)\.(\d+)`)

// ParseRsvgVersion reads `rsvg-convert --version`.
func ParseRsvgVersion(out string) (major, minor int, ok bool) {
	m := rsvgVersionRe.FindStringSubmatch(out)
	if m == nil {
		return 0, 0, false
	}
	major, _ = strconv.Atoi(m[1])
	minor, _ = strconv.Atoi(m[2])
	return major, minor, true
}

// RsvgDrawsMasks reports false only for a version known to be too old: an
// unreadable version is not a measurement, and is let through.
func RsvgDrawsMasks(major, minor int) bool {
	return major > RsvgMaskMin[0] || (major == RsvgMaskMin[0] && minor >= RsvgMaskMin[1])
}

// RsvgVersion runs rsvg-convert once per process.
var RsvgVersion = sync.OnceValues(func() (string, error) {
	out, err := run.Cmd("", "rsvg-convert", "--version")
	return strings.TrimSpace(out), err
})

// RefuseMasks fails when svg holds a <mask> and the installed rsvg-convert
// would print it black. name is what the error calls the file.
func RefuseMasks(svg, name string) error {
	if !strings.Contains(svg, "<mask") {
		return nil
	}
	out, err := RsvgVersion()
	if err != nil {
		return nil
	}
	major, minor, ok := ParseRsvgVersion(out)
	if !ok || RsvgDrawsMasks(major, minor) {
		return nil
	}
	return exit.AsEnvironment(fmt.Errorf(`%s uses an SVG <mask>, which rsvg-convert %d.%d prints as a black box
  (in d2, every labelled connection has one). Versions before %d.%d cannot draw it:
  %s`, name, major, minor, RsvgMaskMin[0], RsvgMaskMin[1], RsvgFix))
}

// RefuseMasksIn is RefuseMasks for a file on disk.
func RefuseMasksIn(path, name string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return RefuseMasks(string(raw), name)
}
