package data

import (
	"bytes"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"gopkg.in/yaml.v3"
)

// exts are the file types a data namespace can come from, in the order they
// are named in messages.
var exts = []string{".yaml", ".yml", ".json", ".csv", ".tsv"}

func isDataFile(name string) bool {
	for _, e := range exts {
		if strings.HasSuffix(name, e) {
			return true
		}
	}
	return false
}

// load reads one data file into a YAML node tree. Everything is held as nodes
// rather than decoded into Go values, because a scalar's Value is the text as
// written: `precio: 0.10` decoded to a float64 prints as 0.1, and in an offer
// that is a visible error.
func load(path string) (*yaml.Node, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	switch filepath.Ext(path) {
	case ".csv", ".tsv":
		return loadCSV(path, raw)
	}
	// JSON is YAML, and yaml.v3 refuses a duplicate key where encoding/json
	// would keep the last one without a word.
	var doc yaml.Node
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if len(doc.Content) == 0 {
		return nil, fmt.Errorf("%s: the file is empty", path)
	}
	return doc.Content[0], nil
}

// loadCSV turns a table with a header row into a list of records, every value
// a string. The checks are the ways a spreadsheet export goes wrong without
// the file looking any different in an editor.
func loadCSV(path string, raw []byte) (*yaml.Node, error) {
	// Excel's UTF-8 export starts with a BOM, and the first column is then
	// called "U+FEFF id": present on screen, absent to every lookup.
	raw = bytes.TrimPrefix(raw, []byte("\xef\xbb\xbf"))
	if !utf8.Valid(raw) {
		line := 1 + bytes.Count(raw[:firstInvalid(raw)], []byte("\n"))
		return nil, fmt.Errorf(`%s: line %d is not UTF-8 — most likely a spreadsheet export in Latin-1.
  Convert it: iconv -f latin1 -t utf-8 %s > fixed.csv`, path, line, filepath.Base(path))
	}
	delim := '\t'
	if filepath.Ext(path) == ".csv" {
		d, err := sniffDelimiter(raw)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		delim = d
	}
	r := csv.NewReader(bytes.NewReader(raw))
	r.Comma = delim
	header, err := r.Read()
	if err == io.EOF {
		return nil, fmt.Errorf("%s: the file is empty", path)
	}
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	seen := map[string]bool{}
	for i, h := range header {
		h = strings.TrimSpace(h)
		header[i] = h
		switch {
		case h == "":
			return nil, fmt.Errorf("%s: column %d of the header row has no name", path, i+1)
		case seen[h]:
			return nil, fmt.Errorf("%s: the header row names %q twice", path, h)
		}
		seen[h] = true
	}
	list := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		var pe *csv.ParseError
		if errors.As(err, &pe) && errors.Is(pe.Err, csv.ErrFieldCount) {
			return nil, fmt.Errorf("%s: line %d has a different number of fields from the header row (%d)",
				path, pe.StartLine, len(header))
		}
		if err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		row := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		for i, v := range rec {
			row.Content = append(row.Content, str(header[i]), str(strings.TrimSpace(v)))
		}
		list.Content = append(list.Content, row)
	}
	return list, nil
}

func str(s string) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: s}
}

// sniffDelimiter picks the separator from the header row. A Spanish Excel
// writes `;`, because the comma is its decimal separator; read with the wrong
// one, every row is a single column and nothing complains.
func sniffDelimiter(raw []byte) (rune, error) {
	header := raw
	if i := bytes.IndexByte(raw, '\n'); i >= 0 {
		header = raw[:i]
	}
	counts := map[rune]int{}
	quoted := false
	for _, c := range string(header) {
		switch {
		case c == '"':
			quoted = !quoted
		case !quoted && (c == ',' || c == ';' || c == '\t'):
			counts[c]++
		}
	}
	best, n, tie := ',', 0, false
	for _, c := range []rune{',', ';', '\t'} {
		switch {
		case counts[c] > n:
			best, n, tie = c, counts[c], false
		case counts[c] == n && n > 0:
			tie = true
		}
	}
	if tie {
		return 0, fmt.Errorf("the header row has as many of one separator as another, so which one splits it is a guess; quote the header names that contain the other")
	}
	return best, nil
}

func firstInvalid(b []byte) int {
	for i := 0; i < len(b); {
		r, size := utf8.DecodeRune(b[i:])
		if r == utf8.RuneError && size == 1 {
			return i
		}
		i += size
	}
	return len(b)
}
