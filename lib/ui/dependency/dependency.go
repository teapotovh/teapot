package dependency

import (
	"fmt"
)

type DependencyType uint8

const (
	DependencyTypeStyle DependencyType = iota
	DependencyTypeScript
)

func (dt DependencyType) String() string {
	switch dt {
	case DependencyTypeStyle:
		return "style"
	case DependencyTypeScript:
		return "script"
	default:
		return "invalid"
	}
}

type Dependency struct {
	Type DependencyType
	Name string
}

func (d Dependency) String() string {
	return fmt.Sprintf("%s:%s", d.Type, d.Name)
}
