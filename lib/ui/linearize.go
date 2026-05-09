package ui

import (
	"crypto/sha256"
	"errors"
	"sort"
	"strings"

	"github.com/teapotovh/teapot/lib/ui/dependency"
)

var ErrCyclicDependency = errors.New("cyclic dependency detected: linearization is not possible")

func cacheKey(units map[dependency.Dependency]Unit) [32]byte {
	entries := make([]string, 0, len(units))
	for dep := range units {
		entries = append(entries, dep.String())
	}

	sort.Strings(entries)

	return sha256.Sum256([]byte(strings.Join(entries, ",")))
}

func (rer *Renderer) Linearize(
	units map[dependency.Dependency]Unit,
	graph dependency.DependencyGraph,
) ([]dependency.Dependency, error) {
	key := cacheKey(units)
	if cached, ok := rer.linearized.Load(key); ok {
		return cached.([]dependency.Dependency), nil
	}

	result, err := linearize(units, graph)
	if err != nil {
		return nil, err
	}

	rer.linearized.Store(key, result)

	return result, nil
}

func linearize(
	units map[dependency.Dependency]Unit,
	graph dependency.DependencyGraph,
) ([]dependency.Dependency, error) {
	inDegree := make(map[dependency.Dependency]int)
	dependents := make(map[dependency.Dependency][]dependency.Dependency)

	for dep := range units {
		if _, ok := inDegree[dep]; !ok {
			inDegree[dep] = 0
		}

		for _, req := range graph[dep] {
			// only consider edges within our unit set
			if _, ok := units[req]; !ok {
				continue
			}

			dependents[req] = append(dependents[req], dep)
			inDegree[dep]++
		}
	}

	queue := make([]dependency.Dependency, 0)

	for dep, degree := range inDegree {
		if degree == 0 {
			queue = append(queue, dep)
		}
	}

	result := make([]dependency.Dependency, 0, len(inDegree))
	for len(queue) > 0 {
		node := queue[0]
		queue = queue[1:]

		result = append(result, node)

		for _, dependent := range dependents[node] {
			inDegree[dependent]--
			if inDegree[dependent] == 0 {
				queue = append(queue, dependent)
			}
		}
	}

	if len(result) != len(inDegree) {
		return nil, ErrCyclicDependency
	}

	return result, nil
}
