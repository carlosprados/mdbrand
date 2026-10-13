package mdtext

import (
	"strings"
	"testing"
)

// protected marks what Protected leaves alone: each protected byte becomes
// '#', so a case reads as the text it is about.
func protected(s string) string {
	b := []byte(s)
	for _, sp := range Protected(s) {
		for i := sp[0]; i < sp[1]; i++ {
			if b[i] != '\n' {
				b[i] = '#'
			}
		}
	}
	return string(b)
}

func TestProtected(t *testing.T) {
	for _, tc := range []struct {
		name, in, want string
	}{
		{"fenced with backticks", "a\n```\nx\n```\nb", "a\n###\n#\n###\nb"},
		{"fenced with tildes", "a\n~~~\nx\n~~~\nb", "a\n###\n#\n###\nb"},
		{"a shorter run does not close", "````\n```\nx\n````\nb", "####\n###\n#\n####\nb"},
		{"the other character does not close", "```\n~~~\nx", "###\n###\n#"},
		{"unclosed runs to the end", "a\n```\nx {{words}}", "a\n###\n###########"},
		{"indented four spaces is not a fence", "a\n\n    ```\n    x\n\nb", "a\n\n#######\n#####\n\nb"},
		{"inline code", "a `x` b", "a ### b"},
		{"double backticks hold a single one", "a ``x ` y`` b", "a ######### b"},
		{"an unmatched backtick is prose", "a ` b", "a ` b"},
		{"an escaped backtick is prose", "a \\`x` b", "a \\`x` b"},
		{"inline math", "a $x+1$ b", "a ##### b"},
		{"display math", "a $$x$$ b", "a ##### b"},
		{"a space after the opening $ is money", "costs $ 5 and $6", "costs $ 5 and $6"},
		{"a digit after the closing $ is money", "$5 and $6 each", "$5 and $6 each"},
		{"a paragraph break ends math", "$a\n\nb$", "$a\n\nb$"},
		{"an escaped $ is prose", "a \\$x$ b", "a \\$x$ b"},
		{"indented code after a blank line", "a\n\n    {{words}}\n\nb", "a\n\n#############\n\nb"},
		{"indented code across a blank line", "a\n\n    x\n\n    y\nb", "a\n\n#####\n\n#####\nb"},
		{"a tab indents", "a\n\n\tx\n", "a\n\n##\n"},
		{"no blank line: a lazy continuation", "a\n    {{words}}", "a\n    {{words}}"},
		{"a list item's paragraph is prose", "- a\n\n    {{words}}", "- a\n\n    {{words}}"},
		{"a numbered item's paragraph is prose", "1. a\n\n    {{words}}", "1. a\n\n    {{words}}"},
		{"a footnote's paragraph is prose", "[^1]: a\n\n    {{words}}", "[^1]: a\n\n    {{words}}"},
		{"code again after the list ends", "- a\n\nb\n\n    x", "- a\n\nb\n\n#####"},
		{"inline code does not cross a fence", "a `b\n```\n`c\n```", "a `b\n###\n##\n###"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := protected(tc.in); got != tc.want {
				t.Errorf("\n in:   %q\n got:  %q\n want: %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestInside(t *testing.T) {
	spans := [][2]int{{2, 4}}
	for i, want := range []bool{false, false, true, true, false} {
		if got := Inside(spans, i); got != want {
			t.Errorf("Inside(%d) = %v, want %v", i, got, want)
		}
	}
}

func TestFence(t *testing.T) {
	for in, want := range map[string]string{
		"```":       "```",
		"````go":    "````",
		"~~~ {.py}": "~~~",
		"``":        "",
		"x```":      "",
	} {
		if got := Fence(in); got != want {
			t.Errorf("Fence(%q) = %q, want %q", in, got, want)
		}
	}
	if strings.Contains(protected("``"), "#") {
		t.Error("two backticks protected as a fence")
	}
}
