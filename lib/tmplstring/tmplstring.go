package tmplstring

import (
	"errors"
	"fmt"
	"strings"
	"text/template"
)

var (
	ErrMissingField = errors.New("expression references missing field")
)

// TMPL holds a reference to the underlying templating engine struct.
// It can be redered with the appropriate parameter `T`.
type TMPL[T any] struct {
	tmpl *template.Template
}

// NewTMPL creates and validates a template that can be later rendered with the given parameters.
func NewTMPL[T any](str string) (*TMPL[T], error) {
	tmpl, err := template.New("tmplstring").Parse(str)
	if err != nil {
		return nil, fmt.Errorf("error while parsing string template: %w", err)
	}

	fields, err := fieldsInStruct[T]()
	if err != nil {
		return nil, fmt.Errorf("error while gathering fields from parameter struct: %w", err)
	}

	expressions := exprsInTemplate(tmpl.Root)

	for _, field := range fields {
		delete(expressions, field)
	}

	if len(expressions) > 0 {
		var es []error
		for expr := range expressions {
			es = append(es, fmt.Errorf("invalid expression %q: %w", expr, ErrMissingField))
		}

		return nil, fmt.Errorf("error while validating string template: %w", errors.Join(es...))
	}

	return &TMPL[T]{tmpl}, nil
}

// Render renders a template with the provided parameter struct.
func (t *TMPL[T]) Render(params T) (string, error) {
	builder := new(strings.Builder)
	if err := t.tmpl.Execute(builder, params); err != nil {
		return "", fmt.Errorf("could not execute templated string: %w", err)
	}

	return builder.String(), nil
}
