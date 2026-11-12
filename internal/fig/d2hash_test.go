package fig

import (
	"errors"
	"strings"
	"testing"

	"github.com/carlosprados/mdbrand/internal/doc"
)

// d2 0.7.1 printed "PKCS" for PKCS#11 and lost a brace to the same #, exit 0
// or an error lines away; a comment and a quoted colour are d2's own.
func TestD2Hashes(t *testing.T) {
	src := strings.Join([]string{
		`# a comment line`,                // 1 comment
		`b: B. HSM con PKCS#11 {`,         // 2 glued: text
		`  hsm: CloudHSM # the cloud one`, // 3 trailing comment
		`  op: "AES#ECB"`,                 // 4 quoted
		`  fill: "#ff0000"`,               // 5 quoted colour
		`}`,                               // 6
		`a -> b: run #3`,                  // 7 before a digit: text
		`keys: |md`,                       // 8 block string opens
		`  # A heading in Markdown`,       // 9 inside the block
		`|`,                               // 10 closes
		`c: 'it#s'`,                       // 11 single-quoted
		`"""`,                             // 12 block comment
		`d: x#y`,                          // 13 inside it
		`"""`,                             // 14
		"e: |`code # here`|",              // 15 one-line block string
		`f: Backend (PKCS#11 / KMS)`,      // 16 glued
	}, "\n")
	var got []int
	for _, h := range d2Hashes(src) {
		got = append(got, h.line)
	}
	want := []int{2, 7, 16}
	if len(got) != len(want) {
		t.Fatalf("lines %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("lines %v, want %v", got, want)
		}
	}
}

// A fenced block's source is a copy in the work directory: its path and
// line lead nowhere, so the error names the block by its caption.
func TestD2ErrorNamesTheBlock(t *testing.T) {
	f := &doc.Fig{SrcPath: "/work/fig05.d2", Caption: "Claves", Fenced: true, Index: 5}
	err := errors.New("err: failed to compile .mdbrand-fig05.d2: /work/.mdbrand-fig05.d2:15:1: unexpected map termination character }")
	got := d2Error(err, f, "/work/.mdbrand-fig05.d2").Error()
	if !strings.Contains(got, `line 15 of the d2 block captioned "Claves", column 1:`) || strings.Contains(got, ":15:1:") {
		t.Errorf("error reads %q", got)
	}
	side := &doc.Fig{SrcPath: "/doc/arch.d2", Index: 0}
	got = d2Error(errors.New("/work/.mdbrand-fig00.d2:3:7: bad"), side, "/work/.mdbrand-fig00.d2").Error()
	if !strings.Contains(got, "arch.d2:3, column 7:") {
		t.Errorf("side file error reads %q", got)
	}
}
