// Package run wraps the external tools mdbrand conducts: pandoc, xelatex, d2,
// vega and rsvg-convert. Every failure carries the tool's own output, because
// "exit status 1" from xelatex is useless and its log is not.
package run

import (
	"fmt"
	"os/exec"
	"strings"
)

// Missing reports the tools from names that are not on PATH.
func Missing(names ...string) []string {
	var out []string
	for _, n := range names {
		if _, err := exec.LookPath(n); err != nil {
			out = append(out, n)
		}
	}
	return out
}

// Have reports whether bin is on PATH.
func Have(bin string) bool {
	_, err := exec.LookPath(bin)
	return err == nil
}

// Cmd runs bin in dir and returns its combined output. On failure the error
// carries the last lines of that output, which is where tools put the reason.
func Cmd(dir, bin string, args ...string) (string, error) {
	c := exec.Command(bin, args...)
	c.Dir = dir
	out, err := c.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("%s %s: %w\n%s", bin, strings.Join(args, " "), err, tail(string(out), 25))
	}
	return string(out), nil
}

// Quiet runs bin and discards output unless it fails.
func Quiet(dir, bin string, args ...string) error {
	_, err := Cmd(dir, bin, args...)
	return err
}

func tail(s string, n int) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}
