// Package data is the document's data directory: YAML, JSON and CSV files whose
// values a document prints with {{data.file.key}}, the way Hugo reads data/.
//
// The file name is the namespace — data/maquinas.yaml is data.maquinas, and
// data/aws/ec2.csv is data.aws.ec2 — and a path goes on into the file: a key
// of a mapping, an index or an `id` of a list. Brackets take any key, which a
// catalogue needs because its ids carry dots: data.maquinas[m5.large].ram.
//
// Nothing is guessed. A key that is not there stops the build listing the ones
// that are, where a template engine would print nothing, and a name two files
// could answer to is refused rather than settled by which one was read first.
package data

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// Store resolves data paths against the document's data roots, reading each
// file the first time a path reaches it and never otherwise: a data directory
// may hold far more than the document uses.
type Store struct {
	docDir string
	dirs   []string          // directories whose files are namespaces
	files  map[string]string // single files declared in mdbrand.data, by stem
	cache  map[string]*yaml.Node
	refs   map[string]bool
}

// Open prepares the data roots for the document at docPath. With no declared
// roots it is data/ beside the document, and it is not an error for that to be
// absent until a path needs it. Declared roots replace the default, resolve
// against the document, expand ~ and $VARS, and must exist: naming a directory
// that is not there is a mistake worth stopping on.
func Open(docPath string, declared []string) (*Store, error) {
	docDir, err := filepath.Abs(filepath.Dir(docPath))
	if err != nil {
		return nil, err
	}
	s := &Store{docDir: docDir, files: map[string]string{}, cache: map[string]*yaml.Node{}, refs: map[string]bool{}}
	if len(declared) == 0 {
		s.dirs = []string{filepath.Join(docDir, "data")}
		return s, nil
	}
	for _, d := range declared {
		p := expandPath(d)
		if !filepath.IsAbs(p) {
			p = filepath.Join(docDir, p)
		}
		st, err := os.Stat(p)
		if err != nil {
			return nil, fmt.Errorf("mdbrand.data: %s: %w\n  Paths resolve against the document.", d, err)
		}
		if st.IsDir() {
			s.dirs = append(s.dirs, p)
			continue
		}
		if !isDataFile(p) {
			return nil, fmt.Errorf("mdbrand.data: %s is not a data file (%s)", d, strings.Join(exts, ", "))
		}
		stem := stemOf(filepath.Base(p))
		if prev, ok := s.files[stem]; ok {
			return nil, fmt.Errorf("mdbrand.data: %s and %s would both be data.%s", prev, p, stem)
		}
		s.files[stem] = p
	}
	return s, nil
}

// Refs is every file the store read or looked for, absolute. Watch mode needs
// the ones looked for and not found too: creating one is exactly the change
// that should rebuild.
func (s *Store) Refs() []string {
	out := make([]string, 0, len(s.refs))
	for p := range s.refs {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

// Value is a scalar a path led to: its text as written, and whether the data
// marked it as Markdown with the !md tag.
type Value struct {
	Text     string
	Markdown bool
}

// Lookup resolves a path such as data.maquinas[m5.large].ram to a scalar.
func (s *Store) Lookup(expr string) (Value, error) {
	n, err := s.Node(expr)
	if err != nil {
		return Value{}, err
	}
	switch n.Kind {
	case yaml.MappingNode:
		return Value{}, fmt.Errorf("%s is a mapping, not a value; pick a key: %s", expr, keyList(mapPairs(n)))
	case yaml.SequenceNode:
		return Value{}, fmt.Errorf("%s is a list of %d, not a value; pick one by index or id", expr, len(n.Content))
	}
	if n.ShortTag() == "!!null" {
		return Value{}, fmt.Errorf("%s has no value (it is null in the data)", expr)
	}
	return Value{Text: n.Value, Markdown: n.Tag == "!md"}, nil
}

// Node resolves a path to whatever it names, for callers that want a whole
// table rather than one value.
func (s *Store) Node(expr string) (*yaml.Node, error) {
	segs, err := ParsePath(expr)
	if err != nil {
		return nil, err
	}
	file, rest, err := s.resolveFile(segs)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", expr, err)
	}
	n, err := s.read(file)
	if err != nil {
		return nil, err
	}
	shown := "data" + formatPath(segs[:len(segs)-len(rest)])
	for _, seg := range rest {
		n = deref(n)
		next, err := child(n, seg, shown)
		if err != nil {
			return nil, err
		}
		n, shown = next, shown+formatPath([]string{seg})
	}
	return deref(n), nil
}

func (s *Store) read(file string) (*yaml.Node, error) {
	if n, ok := s.cache[file]; ok {
		return n, nil
	}
	n, err := load(file)
	if err != nil {
		return nil, err
	}
	s.cache[file] = n
	return n, nil
}

// resolveFile finds the file the leading segments name, in every root, and
// returns the segments left over for inside it.
func (s *Store) resolveFile(segs []string) (string, []string, error) {
	type hit struct {
		file string
		rest []string
	}
	var hits []hit
	var dead []string // where each root ran out, for the message
	if p, ok := s.files[segs[0]]; ok {
		s.refs[p] = true
		hits = append(hits, hit{p, segs[1:]})
	}
	for _, root := range s.dirs {
		dir := root
		for i, seg := range segs {
			var found []string
			for _, e := range exts {
				p := filepath.Join(dir, seg+e)
				if _, err := os.Stat(p); err == nil {
					found = append(found, p)
				}
			}
			sub := filepath.Join(dir, seg)
			st, err := os.Stat(sub)
			isDir := err == nil && st.IsDir()
			if isDir {
				found = append(found, sub+string(filepath.Separator))
			}
			if len(found) > 1 {
				return "", nil, fmt.Errorf("%s could be any of %s; rename all but one", seg, strings.Join(found, ", "))
			}
			if !isDir && len(found) == 1 {
				s.refs[found[0]] = true
				hits = append(hits, hit{found[0], segs[i+1:]})
				break
			}
			if isDir {
				if i == len(segs)-1 {
					return "", nil, fmt.Errorf("%s is a directory of data files, not a value; its files are %s", sub, available(sub))
				}
				dir = sub
				continue
			}
			for _, e := range exts {
				s.refs[filepath.Join(dir, seg+e)] = true
			}
			dead = append(dead, fmt.Sprintf("no %s.{yaml,yml,json,csv,tsv} in %s — %s", seg, dir, available(dir)))
			break
		}
	}
	switch len(hits) {
	case 1:
		return hits[0].file, hits[0].rest, nil
	case 0:
		return "", nil, fmt.Errorf("%s", strings.Join(dead, "\n  "))
	}
	names := make([]string, len(hits))
	for i, h := range hits {
		names[i] = h.file
	}
	return "", nil, fmt.Errorf("two data roots answer to it: %s", strings.Join(names, ", "))
}

// available lists the namespaces a directory offers, for a message.
func available(dir string) string {
	ents, err := os.ReadDir(dir)
	if err != nil {
		return "the directory does not exist; create it, or point mdbrand.data at one"
	}
	var names []string
	seen := map[string]bool{}
	for _, e := range ents {
		name := ""
		switch {
		case strings.HasPrefix(e.Name(), "."):
		case e.IsDir():
			name = e.Name() + "/"
		case isDataFile(e.Name()):
			name = stemOf(e.Name())
		}
		if name != "" && !seen[name] {
			seen[name] = true
			names = append(names, name)
		}
	}
	if len(names) == 0 {
		return "it holds no data files"
	}
	return "there is " + strings.Join(names, ", ")
}

func stemOf(name string) string {
	for _, e := range exts {
		if strings.HasSuffix(name, e) {
			return strings.TrimSuffix(name, e)
		}
	}
	return name
}

// child takes one step into a node: a key of a mapping, or an index or id of a
// list. shown is the path so far, as the author would write it.
func child(n *yaml.Node, seg, shown string) (*yaml.Node, error) {
	switch n.Kind {
	case yaml.MappingNode:
		pairs := mapPairs(n)
		for _, p := range pairs {
			if p[0].Value == seg {
				return p[1], nil
			}
		}
		return nil, fmt.Errorf("%s has no key %q; the ones that exist are %s", shown, seg, keyList(pairs))
	case yaml.SequenceNode:
		if i, err := strconv.Atoi(seg); err == nil {
			if i >= 0 && i < len(n.Content) {
				return n.Content[i], nil
			}
			return nil, fmt.Errorf("%s has %d entries, so index %d is past the end (they count from 0)", shown, len(n.Content), i)
		}
		// A list of records is found by its id, which is how a CSV catalogue
		// with an id column is read.
		var ids []string
		for _, e := range n.Content {
			e = deref(e)
			if e.Kind != yaml.MappingNode {
				continue
			}
			for _, p := range mapPairs(e) {
				if p[0].Value == "id" {
					if deref(p[1]).Value == seg {
						return e, nil
					}
					ids = append(ids, deref(p[1]).Value)
				}
			}
		}
		if len(ids) == 0 {
			return nil, fmt.Errorf("%s is a list without an id field; pick an entry by index, 0 to %d", shown, len(n.Content)-1)
		}
		return nil, fmt.Errorf("%s has no entry with id %q; the ids are %s", shown, seg, list(ids))
	}
	return nil, fmt.Errorf("%s is a single value, so it has no %q", shown, seg)
}

// mapPairs is a mapping's entries with YAML merge keys applied: a catalogue
// written with `<<: *base` must find the inherited keys, and an entry's own
// keys win over the merged ones.
func mapPairs(n *yaml.Node) [][2]*yaml.Node {
	var own, merged [][2]*yaml.Node
	for i := 0; i+1 < len(n.Content); i += 2 {
		k, v := n.Content[i], n.Content[i+1]
		if k.ShortTag() != "!!merge" {
			own = append(own, [2]*yaml.Node{k, v})
			continue
		}
		v = deref(v)
		switch v.Kind {
		case yaml.MappingNode:
			merged = append(merged, mapPairs(v)...)
		case yaml.SequenceNode:
			for _, m := range v.Content {
				if m = deref(m); m.Kind == yaml.MappingNode {
					merged = append(merged, mapPairs(m)...)
				}
			}
		}
	}
	seen := map[string]bool{}
	for _, p := range own {
		seen[p[0].Value] = true
	}
	out := own
	for _, p := range merged {
		if !seen[p[0].Value] {
			seen[p[0].Value] = true
			out = append(out, p)
		}
	}
	return out
}

func deref(n *yaml.Node) *yaml.Node {
	for n != nil && (n.Kind == yaml.AliasNode || n.Kind == yaml.DocumentNode) {
		if n.Kind == yaml.AliasNode {
			n = n.Alias
		} else if len(n.Content) > 0 {
			n = n.Content[0]
		} else {
			break
		}
	}
	return n
}

// listed drops the keys that start with _, which hold what the data is built
// from rather than data: `_base: &base {…}` exists to be merged into the real
// entries, and listing it among the machine types would offer it as one. Such
// a key still answers when a path names it.
func listed(pairs [][2]*yaml.Node) [][2]*yaml.Node {
	var out [][2]*yaml.Node
	for _, p := range pairs {
		if !strings.HasPrefix(p[0].Value, "_") {
			out = append(out, p)
		}
	}
	return out
}

func keyList(pairs [][2]*yaml.Node) string {
	pairs = listed(pairs)
	keys := make([]string, len(pairs))
	for i, p := range pairs {
		keys[i] = p[0].Value
	}
	return list(keys)
}

// list joins names for a message, cut short: a catalogue of three hundred
// machines is not an error message.
func list(names []string) string {
	const max = 20
	if len(names) > max {
		return strings.Join(names[:max], ", ") + fmt.Sprintf(" and %d more", len(names)-max)
	}
	return strings.Join(names, ", ")
}

var identRe = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

// ParsePath splits data.a[b.c].d into its segments after "data".
func ParsePath(expr string) ([]string, error) {
	rest, ok := strings.CutPrefix(expr, "data")
	if !ok || rest == "" || (rest[0] != '.' && rest[0] != '[') {
		return nil, fmt.Errorf("%s: a data path starts with data. or data[", expr)
	}
	var segs []string
	for rest != "" {
		switch rest[0] {
		case '.':
			rest = rest[1:]
			end := strings.IndexAny(rest, ".[")
			if end < 0 {
				end = len(rest)
			}
			seg := rest[:end]
			if !identRe.MatchString(seg) {
				return nil, fmt.Errorf("%s: %q needs brackets: [%s]", expr, seg, seg)
			}
			segs, rest = append(segs, seg), rest[end:]
		case '[':
			end := strings.IndexByte(rest, ']')
			if end < 0 {
				return nil, fmt.Errorf("%s: a [ is never closed", expr)
			}
			seg := rest[1:end]
			if seg == "" {
				return nil, fmt.Errorf("%s: empty brackets", expr)
			}
			segs, rest = append(segs, seg), rest[end+1:]
		default:
			return nil, fmt.Errorf("%s: expected . or [ before %q", expr, rest)
		}
	}
	return segs, nil
}

func formatPath(segs []string) string {
	var b strings.Builder
	for _, s := range segs {
		if identRe.MatchString(s) {
			b.WriteString("." + s)
		} else {
			b.WriteString("[" + s + "]")
		}
	}
	return b.String()
}

// expandPath is the invariant-8 rule for a path someone else will read: ~ and
// $VARS expand, so no machine's home directory has to be written down.
func expandPath(p string) string {
	p = os.ExpandEnv(strings.TrimSpace(p))
	if p == "~" || strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			p = filepath.Join(home, strings.TrimPrefix(p, "~"))
		}
	}
	return p
}
