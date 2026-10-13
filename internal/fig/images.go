package fig

import (
	"encoding/base64"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var imageHrefRe = regexp.MustCompile(`(<image\b[^>]*?\s(?:xlink:)?href\s*=\s*)("[^"]*"|'[^']*')`)

var imageMIME = map[string]string{
	".png":  "image/png",
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".gif":  "image/gif",
	".webp": "image/webp",
	".svg":  "image/svg+xml",
}

// inlineImages puts every picture an SVG links into the SVG itself, as a data
// URI. rsvg-convert reads the copy in the work directory, where a relative
// href leads nowhere, and draws nothing in its place, exit 0: a photo inside a
// linked SVG, or a Vega image mark, vanished from the PDF. Resolved against
// base — the source's own directory — so the PDF and the .docx, which both
// convert that copy, print the picture; one that is not there, or is remote,
// stops the build by name.
func inlineImages(svg, base, name string) (string, error) {
	var bad []string
	out := imageHrefRe.ReplaceAllStringFunc(svg, func(m string) string {
		sub := imageHrefRe.FindStringSubmatch(m)
		href := sub[2][1 : len(sub[2])-1]
		if href == "" || strings.HasPrefix(href, "data:") || strings.HasPrefix(href, "#") {
			return m
		}
		uri, err := imageDataURI(href, base)
		if err != nil {
			bad = append(bad, err.Error())
			return m
		}
		return sub[1] + `"` + uri + `"`
	})
	if len(bad) > 0 {
		return "", fmt.Errorf(`%s links picture(s) that would not print — rsvg-convert leaves an empty space
and says nothing:
  %s
A relative href resolves against the directory of %s`,
			name, strings.Join(bad, "\n  "), name)
	}
	return out, nil
}

func imageDataURI(href, base string) (string, error) {
	path := href
	if u, err := url.Parse(href); err == nil && u.Scheme != "" && len(u.Scheme) > 1 {
		if u.Scheme != "file" {
			return "", fmt.Errorf("%s: a remote picture; download it beside the figure", href)
		}
		path = u.Path
	} else if unescaped, err := url.PathUnescape(href); err == nil {
		path = unescaped
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(base, path)
	}
	mime := imageMIME[strings.ToLower(filepath.Ext(path))]
	if mime == "" {
		return "", fmt.Errorf("%s: not a PNG, JPEG, GIF, WebP or SVG picture", href)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("%s: not found at %s", href, path)
	}
	return "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(raw), nil
}
