package brand

import (
	"reflect"
	"strings"
	"testing"

	"github.com/carlosprados/mdbrand/internal/contract"
)

// Every key a brand.yaml can carry. A bundle is shared and outlives the
// binary that wrote it: a key renamed here is a bundle refused elsewhere.
func TestContractBundleKeys(t *testing.T) {
	contract.Check(t, "brand-keys", yamlPaths(reflect.TypeOf(Brand{}), ""))
}

func yamlPaths(t reflect.Type, prefix string) []string {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return nil
	}
	var out []string
	for i := 0; i < t.NumField(); i++ {
		tag, _, _ := strings.Cut(t.Field(i).Tag.Get("yaml"), ",")
		if tag == "" || tag == "-" {
			continue
		}
		out = append(out, prefix+tag)
		out = append(out, yamlPaths(t.Field(i).Type, prefix+tag+".")...)
	}
	return out
}
