package doc

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/carlosprados/mdbrand/internal/mdtext"
)

// ReadPart reads a file that is part of a larger document — a thesis's
// chapter, appendix or front part — and rebases its relative links so they
// mean, from the master's directory, what they meant from the part's own:
// everything downstream resolves against the master. A part carries no front
// matter; the document's is the master's.
func ReadPart(path, masterDir string) (string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	text := strings.ReplaceAll(string(raw), "\r\n", "\n")
	if fmRe.MatchString(text) {
		name := path
		if rel, err := filepath.Rel(masterDir, path); err == nil {
			name = rel
		}
		return "", fmt.Errorf("%s: a part carries no front matter; the document's is in the master file", name)
	}
	return Rebase(text, filepath.Dir(path), masterDir), nil
}

// linkTargetRe is the target of an inline link or picture: ](target) or
// ](<target>), with an optional title after it.
var linkTargetRe = regexp.MustCompile(`(\]\()(<[^>\n]+>|[^)\s<]+)`)

// Rebase rewrites the relative targets of inline links, pictures, figure
// sources and reference definitions in text, written from dir, so they read
// the same from base. Code is left alone: a link there is an example of one.
// Targets stay relative, so nothing absolute of this machine's reaches the
// output.
func Rebase(text, dir, base string) string {
	if filepath.Clean(dir) == filepath.Clean(base) {
		return text
	}
	move := func(written string) string {
		t := strings.Trim(written, "<>")
		if t == "" || strings.HasPrefix(t, "#") || strings.Contains(t, "://") || strings.HasPrefix(t, "mailto:") || filepath.IsAbs(t) {
			return written
		}
		rel, err := filepath.Rel(base, filepath.Join(dir, t))
		if err != nil {
			return written
		}
		rel = filepath.ToSlash(rel)
		if strings.ContainsAny(rel, " ") || strings.HasPrefix(written, "<") {
			return "<" + rel + ">"
		}
		return rel
	}
	text = replaceOutsideCode(linkTargetRe, text, func(m []string) string { return m[1] + move(m[2]) })
	return replaceOutsideCode(refDefRe, text, func(m []string) string { return m[1] + move(m[3]) + m[4] })
}

// ExpandParts turns what a document wrote for a list of parts — a glob, a
// directory, or a list of either — into the files, resolved against dir.
// A glob or a directory is sorted by name, as 01-intro.md, 02-estado.md are
// meant to be read; a list keeps its order.
func ExpandParts(dir string, specs []string) ([]string, error) {
	var out []string
	for _, s := range specs {
		p := s
		if !filepath.IsAbs(p) {
			p = filepath.Join(dir, s)
		}
		if st, err := os.Stat(p); err == nil && st.IsDir() {
			p = filepath.Join(p, "*.md")
		}
		if !strings.ContainsAny(p, "*?[") {
			if _, err := os.Stat(p); err != nil {
				return nil, fmt.Errorf("%s: no such file beside the document", s)
			}
			out = append(out, p)
			continue
		}
		matches, err := filepath.Glob(p)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", s, err)
		}
		if len(matches) == 0 {
			return nil, fmt.Errorf("%s matches no file beside the document", s)
		}
		sort.Strings(matches)
		out = append(out, matches...)
	}
	return out, nil
}

// CheckChapter holds a chapter's file to one chapter: a level-1 heading,
// first, and no other. Two are two chapters in one file, and none leaves the
// chapter without a title.
func CheckChapter(name, text string) error {
	spans := mdtext.Protected(text)
	var heads []string
	first := ""
	off := 0
	for _, ln := range strings.SplitAfter(text, "\n") {
		if mdtext.Inside(spans, off) {
			off += len(ln)
			continue
		}
		t := strings.TrimSpace(ln)
		if first == "" && t != "" && !strings.HasPrefix(t, "<!--") {
			first = t
		}
		if strings.HasPrefix(ln, "# ") {
			heads = append(heads, t)
		}
		off += len(ln)
	}
	switch {
	case len(heads) == 0:
		return fmt.Errorf("%s holds no chapter heading: a chapter's file opens with its # title", name)
	case len(heads) > 1:
		return fmt.Errorf("%s holds %d chapters (%s): one chapter per file — split it, and name both", name, len(heads), strings.Join(heads, ", "))
	case first != heads[0]:
		return fmt.Errorf("%s: %q comes before the chapter's heading %q; a chapter's file opens with its # title", name, first, heads[0])
	}
	return nil
}
