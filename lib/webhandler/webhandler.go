package webhandler

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	hxhttp "maragu.dev/gomponents-htmx/http"
	c "maragu.dev/gomponents/components"

	"github.com/teapotovh/teapot/lib/httphandler"
	"github.com/teapotovh/teapot/lib/ui"
	uihttp "github.com/teapotovh/teapot/lib/ui/http"
)

var (
	ErrSkeleton  = errors.New("error while rendering the page skeleton")
	ErrRendering = errors.New("rendering error")
)

type WebHandlerFunc func(w http.ResponseWriter, r *http.Request) (ui.Component, error)
type Skeleton func(r *http.Request, component ui.Component) (ui.Component, error)

type WebHandlerConfig struct {
	HTTPHandler httphandler.HTTPHandlerConfig
	Renderer    ui.RendererConfig
}

type WebHandler struct {
	httpHandler *httphandler.HTTPHandler
	renderer    *ui.Renderer
	skeleton    Skeleton

	AssetPath    string
	AssetHandler http.Handler
}

func NewWebHandler(
	config WebHandlerConfig,
	skeleton Skeleton,
	handlers ErrorHandlers,
	logger *slog.Logger,
) (*WebHandler, error) {
	renderer, err := ui.NewRenderer(config.Renderer, logger.With("component", "renderer"))
	if err != nil {
		return nil, fmt.Errorf("error while constructing renderer: %w", err)
	}

	httpHandler := httphandler.NewHTTPHandler(config.HTTPHandler, httphandler.ErrorHandlers{
		InternalHandler:   wrapErrorHandler(renderer, skeleton, handlers.InternalHandler),
		RedirectHandler:   wrapErrorHandler(renderer, skeleton, handlers.RedirectHandler),
		NotFoundHandler:   wrapErrorHandler(renderer, skeleton, handlers.NotFoundHandler),
		BadRequestHandler: wrapErrorHandler(renderer, skeleton, handlers.BadRequestHandler),
		GenericHandler:    wrapErrorHandler(renderer, skeleton, handlers.GenericHandler),
	}, logger.With("component", "webhandler"))

	dependencyHandler := uihttp.ServeDependencies(renderer, logger.With("component", "assets"))

	wh := WebHandler{
		httpHandler: httpHandler,
		skeleton:    skeleton,
		renderer:    renderer,

		AssetPath:    config.Renderer.AssetPath + "*",
		AssetHandler: httpHandler.Adapt(dependencyHandler),
	}

	return &wh, nil
}

const (
	ErrPageTitle       = "error"
	ErrPageDescription = "an error occurred while handling the request"
)

func wrapErrorHandler[T error](
	renderer *ui.Renderer,
	skeleton Skeleton,
	eh ErrorHandler[T],
) httphandler.ErrorHandler[T] {
	return func(w http.ResponseWriter, r *http.Request, e T, contact string) error {
		component, err := eh(w, r, e, contact)
		if err != nil {
			return err
		}

		if component != nil {
			if hxhttp.IsRequest(r.Header) && !hxhttp.IsBoosted(r.Header) {
				// Don't render the skeleton in an HTMX request
				al, err := uihttp.AlreadyLoadedFromRequest(r)
				if err != nil {
					return fmt.Errorf("error while loading already loaded for error handler: %w", err)
				}

				err = renderer.Render(w, al, component)
				if err != nil {
					return fmt.Errorf("error while rendering error handler: %w", err)
				}
			} else {
				component, err = skeleton(r, component)
				if err != nil {
					return fmt.Errorf("error while rendering skeleton for error handler: %w", err)
				}

				err := renderer.RenderPage(w, c.HTML5Props{
					Title:       ErrPageTitle,
					Language:    Lang,
					Description: ErrPageDescription,
				}, component)
				if err != nil {
					return fmt.Errorf("error while rendering error handler: %w", err)
				}
			}
		}

		return nil
	}
}

func (wh *WebHandler) Adapt(fn WebHandlerFunc) http.Handler {
	f := func(w http.ResponseWriter, r *http.Request) error {
		component, err := fn(w, r)
		if err != nil {
			return err
		}

		if page, ok := component.(page); ok {
			// If we're rendering a full page, place the component inside the skeleton
			component, err = wh.skeleton(r, component)
			if err != nil {
				return httphandler.NewInternalError(err, ErrSkeleton)
			}

			err := wh.renderer.RenderPage(w, c.HTML5Props{
				Title:       page.Title(),
				Language:    page.Language(),
				Description: page.Description(),
			}, component)
			if err != nil {
				return httphandler.NewInternalError(err, ErrRendering)
			}
		} else {
			al, err := uihttp.AlreadyLoadedFromRequest(r)
			if err != nil {
				return httphandler.NewInternalError(err, nil)
			}

			err = wh.renderer.Render(w, al, component)
			if err != nil {
				return httphandler.NewInternalError(err, ErrRendering)
			}
		}

		return nil
	}

	return wh.httpHandler.Adapt(f)
}
