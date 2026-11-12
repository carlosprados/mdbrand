package fig

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"github.com/carlosprados/mdbrand/internal/doc"
)

// d2Hash is a # d2 reads as a comment where the author wrote text.
type d2Hash struct {
	line int
	text string // the line as written
}

// d2Hashes finds each # outside quotes and block strings that d2 would take
// for the start of a comment where text was meant. d2 drops the rest of the
// line: "PKCS#11" printed "PKCS", exit 0, and "B. HSM con PKCS#11 {" lost
// its brace, so d2 reported the closing one, lines below. A # glued to a word
// is text; so is one before a digit (#11, #3), which no comment starts with.
// A # after a space and before a word is a comment, and is left alone.
func d2Hashes(src string) []d2Hash {
	var found []d2Hash
	closer := ""     // the delimiter that ends the block string the line is in
	comment := false // inside a """ block comment
	for n, line := range strings.Split(src, "\n") {
		if comment {
			comment = !strings.Contains(line, `"""`)
			continue
		}
		if closer != "" {
			i := strings.Index(line, closer)
			if i < 0 {
				continue
			}
			line, closer = line[i+len(closer):], ""
		}
		if t := strings.TrimSpace(line); strings.HasPrefix(t, `"""`) {
			comment = !strings.Contains(t[3:], `"""`)
			continue
		}
		var hit bool
		hit, closer = d2LineHash(line)
		if hit {
			found = append(found, d2Hash{line: n + 1, text: strings.TrimSpace(line)})
		}
	}
	return found
}

// d2LineHash scans one line: whether a # on it is text d2 would drop, and the
// delimiter of a block string it opens and does not close.
func d2LineHash(line string) (hit bool, closer string) {
	rs := []rune(line)
	var quote rune
	for i := 0; i < len(rs); i++ {
		r := rs[i]
		var prev rune
		if i > 0 {
			prev = rs[i-1]
		}
		switch {
		case quote != 0:
			if r == '\\' {
				i++
			} else if r == quote {
				quote = 0
			}
		case (r == '"' || r == '\'') && (i == 0 || strings.ContainsRune(" \t:{;>(", prev)):
			quote = r
		case r == '|':
			j := i
			for j < len(rs) && (rs[j] == '|' || rs[j] == '`') {
				j++
			}
			open := string(rs[i:j])
			end := reverse(open)
			rest := string(rs[j:])
			k := strings.Index(rest, end)
			if k < 0 {
				return false, end
			}
			i = j + len([]rune(rest[:k+len(end)])) - 1
		case r == '#' && prev != '\\':
			if strings.TrimSpace(string(rs[:i])) == "" {
				return false, "" // a comment line
			}
			glued := !unicode.IsSpace(prev)
			digit := i+1 < len(rs) && unicode.IsDigit(rs[i+1])
			return glued || digit, ""
		}
	}
	return false, ""
}

func reverse(s string) string {
	rs := []rune(s)
	for i, j := 0, len(rs)-1; i < j; i, j = i+1, j-1 {
		rs[i], rs[j] = rs[j], rs[i]
	}
	return string(rs)
}

// d2Where names a line of a figure's d2 source where its author will find
// it: a fenced block's source is a copy in the work directory, whose name
// and line numbers lead nowhere in the document.
func d2Where(f *doc.Fig, line int) string {
	if !f.Fenced {
		return fmt.Sprintf("%s:%d", filepath.Base(f.SrcPath), line)
	}
	if f.Caption != "" {
		return fmt.Sprintf("line %d of the d2 block captioned %q", line, f.Caption)
	}
	return fmt.Sprintf("line %d of figure block %d of the document (no caption)", line, f.Index+1)
}

func refuseD2Hashes(f *doc.Fig, src string) error {
	hs := d2Hashes(src)
	if len(hs) == 0 {
		return nil
	}
	var lines []string
	for _, h := range hs {
		lines = append(lines, fmt.Sprintf("%s: %s", d2Where(f, h.line), h.text))
	}
	return fmt.Errorf(`d2 reads # as the start of a comment, so the rest of each line is dropped —
the label prints cut short, and a { after it goes too:
  %s
Quote the label: b: "B. HSM con PKCS#11" {`, strings.Join(lines, "\n  "))
}

// d2PosRe is a position in a d2 error: path:line:column.
var d2PosRe = regexp.MustCompile(`(\S+\.d2):(\d+):(\d+):`)

// d2Error rewrites the positions in d2's error that name the file d2 was
// given, the author's or mdbrand's sized copy, as d2Where says them. The copy
// keeps the author's line numbers: mdbrand appends to it.
func d2Error(err error, f *doc.Fig, given string) error {
	if err == nil {
		return nil
	}
	msg := d2PosRe.ReplaceAllStringFunc(err.Error(), func(m string) string {
		p := d2PosRe.FindStringSubmatch(m)
		if p[1] != given && p[1] != f.SrcPath && !strings.HasSuffix(p[1], "/"+filepath.Base(given)) {
			return m
		}
		line, _ := strconv.Atoi(p[2])
		return fmt.Sprintf("%s, column %s:", d2Where(f, line), p[3])
	})
	if msg == err.Error() {
		return err
	}
	return fmt.Errorf("%s", msg)
}
