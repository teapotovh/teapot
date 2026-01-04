package observability

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
)

type httpServiceZ struct {
	logger *slog.Logger

	name   string
	checks map[string]Check
}

// Handler implements httpsrv.Handler.
func (z *httpServiceZ) Handler(prefix string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Individual check
		name := r.PathValue("name")
		if name != "" {
			check, ok := z.checks[name]
			if !ok {
				http.NotFound(w, r)
				return
			}

			if err := check.Check(r.Context()); err != nil {
				z.logger.Error("check failed", "name", name, "err", err)
				http.Error(w, fmt.Sprintf("[-]%s failed: %v", name, err), http.StatusInternalServerError)

				return
			} else {
				z.fprintf(w, "[+]%s ok\n", name)
			}

			return
		}

		// All checks
		exclude := r.URL.Query()["exclude"]
		verbose := r.URL.Query().Has("verbose")

		excludeSet := make(map[string]bool)
		for _, e := range exclude {
			excludeSet[e] = true
		}

		var (
			failed []string
			output strings.Builder
		)

		for name, check := range z.checks {
			if excludeSet[name] {
				if verbose {
					z.fprintf(&output, "[excluded] %s\n", name)
				}

				continue
			}

			if err := check.Check(r.Context()); err != nil {
				z.logger.Error("check failed", "name", name, "err", err)

				failed = append(failed, name)
				if verbose {
					z.fprintf(&output, "[-]%s failed: %v\n", name, err)
				}
			} else if verbose {
				z.fprintf(&output, "[+]%s ok\n", name)
			}
		}

		if len(failed) > 0 {
			w.WriteHeader(http.StatusInternalServerError)
		} else if verbose {
			z.fprintf(w, "%s checks passed\n", z.name)
		}

		if verbose {
			z.fprintf(w, "%s", output.String())
		}
	})
}

func (z *httpServiceZ) fprintf(w io.Writer, format string, a ...any) {
	_, err := fmt.Fprintf(w, format, a...)
	if err != nil {
		z.logger.Error("error while writing output for check", "name", z.name, "err", err)
	}
}
