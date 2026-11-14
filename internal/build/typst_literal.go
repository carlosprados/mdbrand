package build

import (
	"reflect"
	"strings"
)

// pandocLiteral doubles every $ in a template's data. The Go template writes
// a pandoc template, where $ opens a variable: a title holding "$100"
// stopped the build in pandoc's template parser, naming a line of a file
// the author never wrote. It walks the string and []string fields, and the
// structs embedded in v, which must be a pointer to a struct.
func pandocLiteral(v any) {
	walkLiteral(reflect.ValueOf(v).Elem())
}

func walkLiteral(v reflect.Value) {
	for i := range v.NumField() {
		f := v.Field(i)
		switch {
		case f.Kind() == reflect.String:
			f.SetString(strings.ReplaceAll(f.String(), "$", "$$"))
		case f.Kind() == reflect.Slice && f.Type().Elem().Kind() == reflect.String:
			for j := range f.Len() {
				f.Index(j).SetString(strings.ReplaceAll(f.Index(j).String(), "$", "$$"))
			}
		case f.Kind() == reflect.Struct && v.Type().Field(i).Anonymous:
			walkLiteral(f)
		}
	}
}
