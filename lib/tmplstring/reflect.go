package tmplstring

import (
	"errors"
	"fmt"
	"reflect"
)

var (
	ErrNotStruct = errors.New("provided type is not a struct. only structs are supported")
)

// fieldsInStruct retruns a list of all fields (recursively) exported in the struct.
func fieldsInStruct[T any]() ([]string, error) {
	if !isStruct[T]() {
		return nil, ErrNotStruct
	}

	var t T
	tt := reflect.TypeOf(t)
	return fieldsInStructHelper(tt), nil
}

// isStruct returns true if the generic type is a struct.
func isStruct[T any]() bool {
	var t T
	tt := reflect.TypeOf(t)
	return tt.Kind() == reflect.Struct
}

// fieldsInStructHelper uses reflection to recursively list all public fields
// in the given struct type.
func fieldsInStructHelper(t reflect.Type) []string {
	if t.Kind() != reflect.Struct {
		return nil
	}

	var fields []string
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)

		if field.IsExported() {
			fields = append(fields, field.Name)

			subfields := fieldsInStructHelper(field.Type)
			for _, subfield := range subfields {
				fields = append(fields, fmt.Sprintf("%s.%s", field.Name, subfield))
			}
		}
	}
	return fields
}
