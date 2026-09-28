package data

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// Entry is one namespace the document can reach, and what reading it found.
type Entry struct {
	Name  string // data.aws.ec2
	Path  string
	Shape string // a one-line description, or why the file cannot be read
	Err   bool
}

// Roots names where the store looks, for a report.
func (s *Store) Roots() []string {
	out := append([]string{}, s.dirs...)
	for _, p := range s.files {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

// Namespaces reads every data file the document could reach and describes
// it. Unlike a build, it reads them all: this is the place to find out that a
// file nobody has used yet will not parse.
func (s *Store) Namespaces() []Entry {
	var out []Entry
	for stem, p := range s.files {
		out = append(out, s.describe("data"+formatPath([]string{stem}), p))
	}
	for _, root := range s.dirs {
		filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
			if err != nil || strings.HasPrefix(d.Name(), ".") && p != root {
				if d != nil && d.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			if d.IsDir() || !isDataFile(d.Name()) {
				return nil
			}
			rel, _ := filepath.Rel(root, p)
			segs := strings.Split(filepath.ToSlash(filepath.Join(filepath.Dir(rel), stemOf(d.Name()))), "/")
			if segs[0] == "." {
				segs = segs[1:]
			}
			out = append(out, s.describe("data"+formatPath(segs), p))
			return nil
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func (s *Store) describe(name, path string) Entry {
	e := Entry{Name: name, Path: path}
	n, err := s.read(path)
	if err != nil {
		e.Shape, e.Err = err.Error(), true
		return e
	}
	e.Shape = shape(deref(n))
	return e
}

func shape(n *yaml.Node) string {
	switch n.Kind {
	case yaml.MappingNode:
		pairs := mapPairs(n)
		return fmt.Sprintf("mapping of %d: %s", len(pairs), keyList(pairs))
	case yaml.SequenceNode:
		if len(n.Content) > 0 {
			if first := deref(n.Content[0]); first.Kind == yaml.MappingNode {
				return fmt.Sprintf("list of %d records: %s", len(n.Content), keyList(mapPairs(first)))
			}
		}
		return fmt.Sprintf("list of %d", len(n.Content))
	}
	return "a single value"
}

// Dump renders what a path names as YAML, for a person reading it.
func (s *Store) Dump(expr string) (string, error) {
	n, err := s.Node(expr)
	if err != nil {
		return "", err
	}
	if n.Kind == yaml.ScalarNode {
		return n.Value, nil
	}
	out, err := yaml.Marshal(resolved(n))
	return strings.TrimRight(string(out), "\n"), err
}

// resolved copies a node with aliases followed and merge keys applied, in
// block style: what a lookup would see, not how the file spells it.
func resolved(n *yaml.Node) *yaml.Node {
	n = deref(n)
	switch n.Kind {
	case yaml.MappingNode:
		m := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		for _, p := range mapPairs(n) {
			k := *p[0]
			k.Style = 0
			m.Content = append(m.Content, &k, resolved(p[1]))
		}
		return m
	case yaml.SequenceNode:
		l := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
		for _, c := range n.Content {
			l.Content = append(l.Content, resolved(c))
		}
		return l
	}
	c := *n
	c.Anchor = ""
	return &c
}
