package ui

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"path/filepath"
	"strings"
	"sync"

	g "maragu.dev/gomponents"
	hx "maragu.dev/gomponents-htmx"
	c "maragu.dev/gomponents/components"
	h "maragu.dev/gomponents/html"

	"github.com/teapotovh/teapot/lib/ui/dependency"
	"github.com/teapotovh/teapot/lib/ui/htmx"
	"github.com/teapotovh/teapot/lib/ui/openprops"
	"github.com/teapotovh/teapot/lib/ui/teapot"
)

var (
	ErrInvalidDependencyPath   = errors.New("invalid dependency path")
	ErrMissingDependency       = errors.New("missing dependency")
	ErrDependencyRegistered    = errors.New("dependency already registered")
	ErrDependencyNotRegistered = errors.New("dependency not registered")
)

type RendererConfig struct {
	AssetPath string
}

type Renderer struct {
	logger           *slog.Logger
	dependencies     map[dependency.Dependency][]byte
	dependencyPaths  map[dependency.Dependency]string
	pathDependencies map[string]dependency.Dependency
	assetPath        string
	debug            bool
	linearized       sync.Map
}

func NewRenderer(config RendererConfig, logger *slog.Logger) (*Renderer, error) {
	renderer := Renderer{
		logger: logger,

		assetPath: config.AssetPath,

		dependencies:     map[dependency.Dependency][]byte{},
		dependencyPaths:  map[dependency.Dependency]string{},
		pathDependencies: map[string]dependency.Dependency{},
	}

	err := renderer.RegisterDependencies(
		teapot.Dependencies,
		htmx.Dependencies,
		openprops.Dependencies,
	)
	if err != nil {
		return nil, fmt.Errorf("error while registering default dependencies: %w", err)
	}

	return &renderer, nil
}

func (rer *Renderer) Debug(dbg bool) {
	rer.debug = dbg
}

type DependenciesFunc func() (map[dependency.Dependency][]byte, error)

func (rer *Renderer) RegisterDependencies(fns ...DependenciesFunc) error {
	var errs []error

	for i, fn := range fns {
		deps, err := fn()
		if err != nil {
			errs = append(errs, fmt.Errorf("error while loading dependencies at %d: %w", i, err))
		}

		for dep, bytes := range deps {
			if _, ok := rer.dependencies[dep]; ok {
				return fmt.Errorf("error while registering dependency %q: %w", dep, ErrDependencyRegistered)
			}

			hash := sha256.Sum256(bytes)
			path := filepath.Join(rer.assetPath, fmt.Sprintf("%s-%x", dep, hash))

			rer.dependencies[dep] = bytes
			rer.dependencyPaths[dep] = path
			rer.pathDependencies[path] = dep
		}

		maps.Insert(rer.dependencies, maps.All(deps))
	}

	return errors.Join(errs...)
}

func (rer *Renderer) Dependency(path string) (dependency.Dependency, []byte, error) {
	if !strings.HasPrefix(path, rer.assetPath) {
		return dependency.Dependency{}, nil, ErrInvalidDependencyPath
	}

	if dep, ok := rer.pathDependencies[path]; ok {
		return dep, rer.dependencies[dep], nil
	}

	return dependency.Dependency{}, nil, ErrMissingDependency
}

var (
	ScriptHTMX                = dependency.Dependency{Type: dependency.DependencyTypeScript, Name: "htmx"}             // htmx/htmx
	ScriptHTMXResponseTargets = dependency.Dependency{Type: dependency.DependencyTypeScript, Name: "response-targets"} // htmx/response-targets
	ScriptHTMXHeadSupport     = dependency.Dependency{Type: dependency.DependencyTypeScript, Name: "head-support"}     // htmx/head-support
	ScriptTeapotRender        = dependency.Dependency{Type: dependency.DependencyTypeScript, Name: "render"}           // teapot/render

	StyleNormalize = dependency.Dependency{Type: dependency.DependencyTypeStyle, Name: "normalize"} // open-props/normalize
	StyleColors    = dependency.Dependency{Type: dependency.DependencyTypeStyle, Name: "colors"}    // open-props/colors
	StyleFont      = dependency.Dependency{Type: dependency.DependencyTypeStyle, Name: "font"}      // open-props/font
)

var defaultDependencies = map[dependency.Dependency]Unit{
	ScriptHTMX:                {},
	ScriptHTMXResponseTargets: {},
	ScriptHTMXHeadSupport:     {},
	ScriptTeapotRender:        {},

	StyleColors:    {},
	StyleNormalize: {},
	StyleFont:      {},
}

var DependencyGraph = dependency.DependencyGraph{
	ScriptHTMXResponseTargets: []dependency.Dependency{ScriptHTMX},
	ScriptHTMXHeadSupport:     []dependency.Dependency{ScriptHTMX},
	ScriptTeapotRender:        []dependency.Dependency{ScriptHTMX},

	StyleColors: []dependency.Dependency{StyleNormalize},
	StyleFont:   []dependency.Dependency{StyleNormalize},
}

type AlreadyLoaded struct {
	Styles       map[string]Unit
	Dependencies map[dependency.Dependency]Unit
}

func (al AlreadyLoaded) IsEmpty() bool {
	return len(al.Styles) <= 0 && len(al.Dependencies) <= 0
}

func EmptyAlreadyLoaded() AlreadyLoaded {
	return AlreadyLoaded{Styles: nil, Dependencies: map[dependency.Dependency]Unit{}}
}

func registerScript[T fmt.Stringer](target string, elements []T) (string, error) {
	var slice []string
	for _, ele := range elements {
		slice = append(slice, ele.String())
	}

	bytes, err := json.Marshal(slice)
	if err != nil {
		return "", fmt.Errorf("error while generating register code: %w", err)
	}

	return fmt.Sprintf("(%s).forEach(e => window.teapot.%s.add(e))", string(bytes), target), nil
}

// Render renders a component to the response. It adds styles as necessary.
func (rer *Renderer) Render(ctx context.Context, w io.Writer, loaded AlreadyLoaded, component Component) error {
	styles, scripts, node, err := rer.renderWithDependencies(ctx, loaded, component)
	if err != nil {
		return err
	}

	all := g.Group(append(
		styles,
		append(scripts, node)...,
	))

	if err := all.Render(w); err != nil {
		return fmt.Errorf("error while rendering component: %w", err)
	}

	return nil
}

// RenderPage renders a full page to the response. It adds styles as necessary.
func (rer *Renderer) RenderPage(ctx context.Context, w io.Writer, loaded AlreadyLoaded, opts c.HTML5Props, body Component) error {
	styles, scripts, node, err := rer.renderWithDependencies(ctx, loaded, body)
	if err != nil {
		return err
	}

	// Preprend hx-ext before all body elements
	opts.Body = append([]g.Node{hx.Ext("head-support")}, opts.Body...)
	opts.Head = append(opts.Head, styles...)
	opts.Head = append(opts.Head, scripts...)
	opts.Body = append(opts.Body, node)

	if err := c.HTML5(opts).Render(w); err != nil {
		return fmt.Errorf("error while rendering page: %w", err)
	}

	return nil
}

func (rer *Renderer) dependencyPath(dep dependency.Dependency) (string, error) {
	if path, ok := rer.dependencyPaths[dep]; ok {
		return path, nil
	}

	return "", fmt.Errorf("could not find path for dependency %q: %w", dep, ErrDependencyNotRegistered)
}

func (rer *Renderer) contextRender(component Component) (renderContext, g.Node) {
	ctx := renderContext{
		renderer:     rer,
		styles:       map[*Style]Unit{},
		dependencies: maps.Clone(defaultDependencies),
	}
	node := component.Render(&ctx)

	return ctx, node
}

func (rer *Renderer) renderWithDependencies(
	ctx context.Context,
	loaded AlreadyLoaded,
	component Component,
) ([]g.Node, []g.Node, g.Node, error) {
	rerCtx, node := rer.contextRender(component)
	rer.logger.DebugContext(ctx, "rendering with", "styles", rerCtx.styles, "dependencies", rerCtx.dependencies)

	var (
		links   []g.Node
		scripts []g.Node

		dependencies []dependency.Dependency
		styles       []*Style
	)

	// The order in which script dependencies are inserted is important.
	// For example, htmx-ext-response-targets and render need to be registered after htmx.
	linearized, err := rer.Linearize(rerCtx.dependencies, DependencyGraph)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("error while linearizing dependencies: %w", err)
	}

	for _, dep := range linearized {
		if _, ok := loaded.Dependencies[dep]; ok {
			continue
		}

		url, err := rer.dependencyPath(dep)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("error while rendering dependency tags (script, style): %w", err)
		}

		switch dep.Type {
		case dependency.DependencyTypeStyle:
			links = append(links, h.Link(hx.Preserve("true"), h.Rel("stylesheet"), h.Href(url)))
		case dependency.DependencyTypeScript:
			scripts = append(scripts, h.Script(hx.Preserve("true"), h.Src(url)))
		case dependency.DependencyTypeInvalid:
		default:
			return nil, nil, nil, dependency.ErrInvalidDependency
		}

		dependencies = append(dependencies, dep)
	}

	// Generate style element for all components in this page
	var stylesheet strings.Builder

	for style := range rerCtx.styles {
		if _, ok := loaded.Styles[style.id]; ok {
			continue
		}

		var rule string
		if rer.debug {
			rule = fmt.Sprintf(".%s {\n  %s\n}", style.id, strings.ReplaceAll(style.css, "\n", "\n  "))
		} else {
			rule = fmt.Sprintf(".%s{%s}", style.id, strings.ReplaceAll(style.css, "\n", ""))
		}

		_, err := stylesheet.WriteString(rule)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("error while generating stylesheet for %q: %w", style.id, err)
		}

		styles = append(styles, style)
	}

	if stylesheet.Len() > 0 {
		style := stylesheet.String()
		hash := sha256.Sum256([]byte(style))
		links = append(links, h.StyleEl(h.ID(fmt.Sprintf("style-%x", hash)), hx.Preserve("true"), g.Raw(style)))
	}

	// Add script tags to update the list of styles and dependencies registered
	src, err := registerScript("styles", styles)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("error while generating styles register code: %w", err)
	}

	if len(styles) > 0 {
		scripts = append(scripts, h.Script(h.Data("inject", ""), g.Raw(src)))
	}

	drc, err := registerScript("dependencies", dependencies)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("error while generating dependencies register code: %w", err)
	}

	if len(dependencies) > 0 {
		scripts = append(scripts, h.Script(h.Data("inject", ""), g.Raw(drc)))
	}

	return links, scripts, node, nil
}
