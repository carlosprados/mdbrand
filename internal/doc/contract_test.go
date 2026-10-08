package doc

import (
	"testing"

	"github.com/carlosprados/mdbrand/internal/contract"
)

// Every option a document's mdbrand: block can carry. WordCount reads its
// mapping by hand, so its keys are listed here rather than derived.
func TestContractFrontMatterOptions(t *testing.T) {
	var keys []string
	for _, k := range optionKeys() {
		keys = append(keys, "mdbrand."+k)
	}
	keys = append(keys, "mdbrand.wordcount.base", "mdbrand.wordcount.include")
	contract.Check(t, "front-matter", keys)
}
