package doc

import (
	"fmt"
	"slices"
	"strings"

	"github.com/carlosprados/mdbrand/internal/suggest"
	"gopkg.in/yaml.v3"
)

// Thesis is what a doctoral thesis's three fixed pages print and the files
// its front parts are read from. The UPM's template puts "the data in the
// < > slots and no other information" on those pages, so each slot is a key
// and nothing else can reach them.
type Thesis struct {
	Programme string // printed as "Doctorado en …"
	Degree    string // the candidate's previous degree
	Directors []Director
	Funding   string // the credits page's foot, after "parcialmente financiada por"

	// Front parts, paths relative to the document: written without a
	// heading, which mdbrand sets in the template's words.
	Dedication       string
	Acknowledgements string
	Abstract         string // English, whatever the thesis's language
	Resumen          string // Spanish, likewise
	Acronyms         string
}

// Director is one director of the thesis: the portada prints the name, the
// credits page the name, position and institution.
type Director struct {
	Name, Position, Institution string
	Co                          bool // role: codirector
}

var thesisKeys = []string{"programme", "degree", "directors", "funding",
	"dedication", "acknowledgements", "abstract", "resumen", "acronyms"}

// UnmarshalYAML decodes by hand because YAML drops an unknown field in
// silence: `director:` would have built a thesis with no director.
func (t *Thesis) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind != yaml.MappingNode {
		return fmt.Errorf("mdbrand.thesis: expected a mapping of %s", strings.Join(thesisKeys, ", "))
	}
	text := map[string]*string{
		"programme": &t.Programme, "degree": &t.Degree, "funding": &t.Funding,
		"dedication": &t.Dedication, "acknowledgements": &t.Acknowledgements,
		"abstract": &t.Abstract, "resumen": &t.Resumen, "acronyms": &t.Acronyms,
	}
	for i := 0; i+1 < len(n.Content); i += 2 {
		k, v := n.Content[i].Value, n.Content[i+1]
		if dst, ok := text[k]; ok {
			if v.Kind != yaml.ScalarNode {
				return fmt.Errorf("mdbrand.thesis.%s: expected text", k)
			}
			*dst = strings.TrimSpace(v.Value)
			continue
		}
		if k != "directors" {
			return unknownKey("mdbrand.thesis", k, thesisKeys)
		}
		if v.Kind != yaml.SequenceNode {
			return fmt.Errorf("mdbrand.thesis.directors: expected a list of directors, each a mapping of name, position and institution")
		}
		for _, d := range v.Content {
			var dir Director
			if err := dir.decode(d); err != nil {
				return err
			}
			t.Directors = append(t.Directors, dir)
		}
	}
	return nil
}

var directorKeys = []string{"name", "position", "institution", "role"}

func (d *Director) decode(n *yaml.Node) error {
	if n.Kind != yaml.MappingNode {
		return fmt.Errorf("mdbrand.thesis.directors: %q is not a director — a director is a mapping of name, position and institution; the credits page prints all three",
			strings.TrimSpace(n.Value))
	}
	for i := 0; i+1 < len(n.Content); i += 2 {
		k, v := n.Content[i].Value, n.Content[i+1]
		if v.Kind != yaml.ScalarNode {
			return fmt.Errorf("mdbrand.thesis.directors[].%s: expected text", k)
		}
		val := strings.TrimSpace(v.Value)
		switch k {
		case "name":
			d.Name = val
		case "position":
			d.Position = val
		case "institution":
			d.Institution = val
		case "role":
			if val != "director" && val != "codirector" {
				return fmt.Errorf("mdbrand.thesis.directors[].role: %q — the roles are director and codirector", val)
			}
			d.Co = val == "codirector"
		default:
			return unknownKey("mdbrand.thesis.directors[]", k, directorKeys)
		}
	}
	return nil
}

func unknownKey(where, k string, known []string) error {
	hint := ""
	if near := suggest.Closest(k, known); near != "" {
		hint = fmt.Sprintf(" — did you mean %s?", near)
	}
	return fmt.Errorf("%s.%s is not a key mdbrand reads%s\nThe keys are %s", where, k, hint, strings.Join(known, ", "))
}

// Printed is every string of the thesis that its three pages print.
func (t Thesis) Printed() []string {
	out := []string{t.Programme, t.Degree, t.Funding}
	for _, d := range t.Directors {
		out = append(out, d.Name, d.Position, d.Institution)
	}
	return out
}

// StripKeys takes the named top-level keys out of a front matter. A front
// matter holding none of them comes back as it was, byte for byte.
func StripKeys(front string, keys ...string) (string, error) {
	var root yaml.Node
	if err := yaml.Unmarshal([]byte(front), &root); err != nil || len(root.Content) == 0 {
		return front, err
	}
	m := root.Content[0]
	if m.Kind != yaml.MappingNode {
		return front, nil
	}
	changed := false
	for i := 0; i+1 < len(m.Content); {
		if slices.Contains(keys, m.Content[i].Value) {
			m.Content = append(m.Content[:i], m.Content[i+2:]...)
			changed = true
			continue
		}
		i += 2
	}
	if !changed {
		return front, nil
	}
	out, err := yaml.Marshal(&root)
	return strings.TrimRight(string(out), "\n"), err
}
