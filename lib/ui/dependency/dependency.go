package dependency

import (
	"errors"
	"fmt"
	"strings"
)

type DependencyType uint8

const (
	DependencyTypeInvalid DependencyType = iota // should not be used
	DependencyTypeStyle
	DependencyTypeScript
)

var (
	ErrInvalidDependency     = errors.New("invalid dependency")
	ErrInvalidDependencyType = errors.New("invalid dependency type")
)

func ParseDependencyType(raw string) (DependencyType, error) {
	switch raw {
	case "style":
		return DependencyTypeStyle, nil
	case "script":
		return DependencyTypeScript, nil
	default:
		return DependencyTypeInvalid, fmt.Errorf(
			"could not parse dependency type %q: %w",
			raw,
			ErrInvalidDependencyType,
		)
	}
}

func (dt DependencyType) String() string {
	switch dt {
	case DependencyTypeStyle:
		return "style"
	case DependencyTypeScript:
		return "script"
	case DependencyTypeInvalid:
		return "invalid"
	}

	return "invalid"
}

type Dependency struct {
	Type DependencyType
	Name string
}

func ParseDependency(raw string) (Dependency, error) {
	parts := strings.Split(raw, ":")
	if len(parts) != 2 {
		return Dependency{}, ErrInvalidDependency
	}

	dt, err := ParseDependencyType(parts[0])
	if err != nil {
		return Dependency{}, err
	}

	name := parts[1]

	return Dependency{
		Type: dt,
		Name: name,
	}, nil
}

func (d Dependency) String() string {
	return fmt.Sprintf("%s:%s", d.Type, d.Name)
}
