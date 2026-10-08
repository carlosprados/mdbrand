package brand

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"github.com/carlosprados/mdbrand/internal/suggest"
	"gopkg.in/yaml.v3"
)

// ToolVersion is the running mdbrand, "v0.18.0" or "v0.18.0-3-gabc-dirty",
// set by cmd at start. It is a package variable rather than an argument to
// Load because every caller of Load would otherwise have to carry it, and
// only the requires check reads it. Empty or unparseable — a build from
// source with no version stamped — is never compared.
var ToolVersion string

// releases is where a reader whose mdbrand is too old gets a newer one.
const releases = "https://github.com/carlosprados/mdbrand/releases"

// checkRequires refuses a bundle written for a newer mdbrand. A bundle is
// shared, and the reader's binary is not the author's: an older one dropped
// slides: and colors.accent in silence, and built the document without them.
// It runs before the unknown-key check, so a key from the future is reported
// as what it is — a tool to upgrade — and not as a typo.
func checkRequires(root *yaml.Node, name string) error {
	req := mappingValue(root, "requires")
	if req == "" {
		return nil
	}
	want, ok := parseVersion(req)
	if !ok {
		return fmt.Errorf(`brand %q: requires: %q is not a version; write the oldest mdbrand
the bundle works with, e.g. requires: "0.18"`, name, req)
	}
	have, ok := parseVersion(ToolVersion)
	if !ok || !less(have, want) {
		return nil
	}
	return fmt.Errorf(`brand %q requires mdbrand %s or later, and this is %s.
The bundle uses settings an older mdbrand would skip in silence, so the
document would not be the one the bundle describes. Upgrade:
  %s`, name, req, ToolVersion, releases)
}

// mappingValue is the scalar under key at the top of a YAML document.
func mappingValue(root *yaml.Node, key string) string {
	m := root
	if m.Kind == yaml.DocumentNode && len(m.Content) > 0 {
		m = m.Content[0]
	}
	if m.Kind != yaml.MappingNode {
		return ""
	}
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return strings.TrimSpace(m.Content[i+1].Value)
		}
	}
	return ""
}

// parseVersion reads "0.18", "0.18.0", "v0.18.0" or a git describe such as
// "v0.18.0-3-gabc-dirty" into major, minor, patch.
func parseVersion(s string) ([3]int, bool) {
	var v [3]int
	s = strings.TrimPrefix(strings.TrimSpace(s), "v")
	if i := strings.IndexAny(s, "-+ "); i >= 0 {
		s = s[:i]
	}
	parts := strings.Split(s, ".")
	if len(parts) < 2 || len(parts) > 3 {
		return v, false
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return v, false
		}
		v[i] = n
	}
	return v, true
}

func less(a, b [3]int) bool {
	for i := range a {
		if a[i] != b[i] {
			return a[i] < b[i]
		}
	}
	return false
}

// checkKeys refuses a key in brand.yaml that no setting reads. YAML drops an
// unknown field in silence, so `colors: {acent: …}` built every document with
// the primary as its accent and exited 0. The known keys are the yaml tags of
// the structs, so a new setting is known here the moment it is declared.
func checkKeys(root *yaml.Node, name string) error {
	if root.Kind == yaml.DocumentNode && len(root.Content) > 0 {
		root = root.Content[0]
	}
	if path, known := unknownKey(root, reflect.TypeOf(Brand{}), ""); path != "" {
		leaf := path[strings.LastIndexByte(path, '.')+1:]
		hint := ""
		if near := suggest.Closest(leaf, known); near != "" {
			hint = fmt.Sprintf(" — did you mean %s?", near)
		}
		return fmt.Errorf("brand %q: %s is not a setting mdbrand reads%s\n  the settings there are %s",
			name, path, hint, strings.Join(known, ", "))
	}
	return nil
}

// unknownKey walks a mapping against a struct type and returns the dotted
// path of the first key the struct has no field for, with the keys it has.
func unknownKey(n *yaml.Node, t reflect.Type, prefix string) (string, []string) {
	if n.Kind != yaml.MappingNode || t.Kind() != reflect.Struct {
		return "", nil
	}
	fields := map[string]reflect.Type{}
	var known []string
	for i := 0; i < t.NumField(); i++ {
		tag, _, _ := strings.Cut(t.Field(i).Tag.Get("yaml"), ",")
		if tag == "" || tag == "-" {
			continue
		}
		fields[tag] = t.Field(i).Type
		known = append(known, tag)
	}
	for i := 0; i+1 < len(n.Content); i += 2 {
		k := n.Content[i].Value
		ft, ok := fields[k]
		if !ok {
			return prefix + k, known
		}
		if path, kn := unknownKey(n.Content[i+1], ft, prefix+k+"."); path != "" {
			return path, kn
		}
	}
	return "", nil
}
