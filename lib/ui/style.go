package ui

import (
	"fmt"
	"regexp"
	"strings"
	"sync/atomic"

	"github.com/sqids/sqids-go"

	"github.com/teapotovh/teapot/lib/ui/dependency"
)

var counter atomic.Uint64

type Style struct {
	id           string
	css          string
	dependencies []dependency.Dependency
}

func MustParseStyle(css string) *Style {
	style, err := ParseStyle(css)
	if err != nil {
		panic(fmt.Errorf("error while parsing style: %w", err))
	}

	return style
}

func (s *Style) String() string {
	return s.id
}

var (
	ids *sqids.Sqids
	re  = regexp.MustCompile(`var\(--([a-zA-Z0-9\-]+)`)
)

//nolint:gochecknoinits
func init() {
	s, err := sqids.New()
	if err != nil {
		panic(fmt.Errorf("could not create squids ID encoder: %w", err))
	}

	ids = s
}

func ParseStyle(css string) (*Style, error) {
	id, err := ids.Encode([]uint64{counter.Load()})
	if err != nil {
		return nil, fmt.Errorf("error while computing hash for style: %w", err)
	}

	counter.Add(1)

	var dependencies []dependency.Dependency

	matches := re.FindAllStringSubmatch(css, -1)
	for _, m := range matches {
		parts := strings.Split(m[1], "-")
		dep := dependency.Dependency{
			Type: dependency.DependencyTypeStyle,
			Name: parts[0],
		}
		dependencies = append(dependencies, dep)
	}

	style := Style{
		id:           id,
		css:          css,
		dependencies: dependencies,
	}

	return &style, nil
}
