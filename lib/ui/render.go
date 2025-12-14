package ui

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"path"
	"strings"

	flag "github.com/spf13/pflag"
	g "maragu.dev/gomponents"
	hx "maragu.dev/gomponents-htmx"
	h "maragu.dev/gomponents/html"

	"github.com/teapotovh/teapot/lib/ui/dependency"
	"github.com/teapotovh/teapot/lib/ui/htmx"
	"github.com/teapotovh/teapot/lib/ui/openprops"
	"github.com/teapotovh/teapot/lib/ui/teapot"
)

var (
	ErrInvalidDependencyPath = errors.New("invalid dependency path")
	ErrMissingDependency     = errors.New("missing dependency")
)

type RendererConfig struct {
	AssetPath string
}

func RendererFlagSet() (*flag.FlagSet, func() RendererConfig) {
	fs := flag.NewFlagSet("ui/renderer", flag.ExitOnError)

	assetPath := fs.String("ui-renderer-asset-path", "/static/", "the URI path where assets will be served")

	return fs, func() RendererConfig {
		return RendererConfig{
			AssetPath: *assetPath,
		}
	}
}

type Renderer struct {
	page             Page
	logger           *slog.Logger
	dependencies     map[dependency.Dependency][]byte
	dependencyPaths  map[dependency.Dependency]string
	pathDependencies map[string]dependency.Dependency
	assetPath        string
}

func NewRenderer(config RendererConfig, page Page, logger *slog.Logger) (*Renderer, error) {
	renderer := Renderer{
		logger: logger,

		assetPath: config.AssetPath,
		page:      page,

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
				return fmt.Errorf("dependency %q registered multiple times", dep)
			}

			hash := sha256.Sum256(bytes)
			path := path.Join(rer.assetPath, fmt.Sprintf("%s-%x", dep, hash))

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

type Component interface {
	Render(ctx Context) g.Node
}

var defaultDependencies = map[dependency.Dependency]unit{
	{Type: dependency.DependencyTypeScript, Name: "htmx"}:             {}, // htmx/htmx
	{Type: dependency.DependencyTypeScript, Name: "response-targets"}: {}, // htmx/response-targets
	{Type: dependency.DependencyTypeScript, Name: "render"}:           {}, // teapot/render

	{Type: dependency.DependencyTypeStyle, Name: "colors"}:    {}, // open-props/colors
	{Type: dependency.DependencyTypeStyle, Name: "normalize"}: {}, // open-props/normalize
	{Type: dependency.DependencyTypeStyle, Name: "font"}:      {}, // open-props/font
}

func (rer *Renderer) contextRender(component Component) (context, g.Node) {
	ctx := context{
		renderer:     rer,
		styles:       map[*Style]unit{},
		dependencies: defaultDependencies,
	}
	node := component.Render(&ctx)

	return ctx, node
}

func (rer *Renderer) dependencyPath(dep dependency.Dependency) (string, error) {
	if path, ok := rer.dependencyPaths[dep]; ok {
		return path, nil
	}

	return "", fmt.Errorf("dependency %q not registered", dep)
}

type AlreadyLoaded struct {
	Styles       map[string]unit
	Dependencies map[dependency.Dependency]unit
}

func emptyAlreadyLoaded() AlreadyLoaded {
	return AlreadyLoaded{Styles: nil, Dependencies: map[dependency.Dependency]unit{}}
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

func (rer *Renderer) renderWithDependencies(
	loaded AlreadyLoaded,
	component Component,
) ([]g.Node, []g.Node, g.Node, error) {
	ctx, node := rer.contextRender(component)
	rer.logger.Debug("rendering with", "styles", ctx.styles, "dependencies", ctx.dependencies)

	var (
		links   []g.Node
		scripts []g.Node

		dependencies []dependency.Dependency
		styles       []*Style
	)

	// TODO: the order in which script dependencies are inserted is important.
	// For example, htmx-ext-response-targets and render need to be registered
	// after htmx. We need proper dependency tree linearization to handle this.
	for dep := range ctx.dependencies {
		if _, ok := loaded.Dependencies[dep]; ok {
			continue
		}

		url, err := rer.dependencyPath(dep)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("error while rendering dependency tags (script, style): %w", err)
		}

		switch dep.Type {
		case dependency.DependencyTypeStyle:
			links = append(links, h.Link(h.Rel("stylesheet"), h.Href(url)))
		case dependency.DependencyTypeScript:
			scripts = append(scripts, h.Script(h.Src(url)))
		default:
			return nil, nil, nil, fmt.Errorf("unexpected dependency type: %s", dep.Type)
		}

		dependencies = append(dependencies, dep)
	}

	// Generate style element for all components in this page
	var stylesheet strings.Builder

	for style := range ctx.styles {
		if _, ok := loaded.Styles[style.id]; ok {
			continue
		}

		rule := fmt.Sprintf(".%s {\n  %s\n}", style.id, strings.ReplaceAll(style.css, "\n", "\n  "))

		_, err := stylesheet.WriteString(rule)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("error while generating stylesheet for %q: %w", style.id, err)
		}

		styles = append(styles, style)
	}

	links = append(links, h.StyleEl(g.Raw(stylesheet.String())))

	// Add script tags to update the list of styles and dependencies registered
	src, err := registerScript("styles", styles)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("error while generating styles register code: %w", err)
	}

	scripts = append(scripts, h.Script(hx.SwapOOB("beforeend:head"), g.Raw(src)))

	drc, err := registerScript("dependencies", dependencies)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("error while generating dependencies register code: %w", err)
	}

	scripts = append(scripts, h.Script(hx.SwapOOB("beforeend:head"), g.Raw(drc)))

	return links, scripts, node, nil
}

// Render renders a component to the response. It adds styles as necessary.
func (rer *Renderer) Render(w io.Writer, loaded AlreadyLoaded, component Component) error {
	styles, scripts, node, err := rer.renderWithDependencies(loaded, component)
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
func (rer *Renderer) RenderPage(w io.Writer, title string, component Component) error {
	loaded := emptyAlreadyLoaded()

	styles, scripts, node, err := rer.renderWithDependencies(loaded, component)
	if err != nil {
		return err
	}

	props := PageOptions{
		Title:   title,
		Styles:  styles,
		Scripts: scripts,
		Body:    []g.Node{node},
	}
	page := rer.page.Render(props)

	if err := page.Render(w); err != nil {
		return fmt.Errorf("error while rendering page: %w", err)
	}

	return nil
}
