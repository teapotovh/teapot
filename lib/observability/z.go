package observability

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
)

const sep = "/"

type httpServiceZ struct {
	logger *slog.Logger

	name   string
	checks map[string]Check
}

// Handler implements httpsrv.Handler.
func (r *httpServiceZ) Handler(prefix string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		// Individual check
		if name := strings.TrimPrefix(req.URL.Path, prefix); name != "" && name != sep {
			check, ok := r.checks[strings.TrimSuffix(name, sep)]
			if !ok {
				http.NotFound(w, req)
				return
			}

			if err := check.Check(req.Context()); err != nil {
				r.logger.Error("check failed", "name", name, "err", err)
				http.Error(w, fmt.Sprintf("[-]%s failed: %v", name, err), http.StatusInternalServerError)

				return
			} else {
				r.fprintf(w, "[+]%s ok\n", name)
			}

			return
		}

		// All checks
		exclude := req.URL.Query()["exclude"]
		verbose := req.URL.Query().Has("verbose")

		excludeSet := make(map[string]bool)
		for _, e := range exclude {
			excludeSet[e] = true
		}

		var (
			failed []string
			output strings.Builder
		)

		for name, check := range r.checks {
			if excludeSet[name] {
				if verbose {
					r.fprintf(&output, "[excluded] %s\n", name)
				}

				continue
			}

			if err := check.Check(req.Context()); err != nil {
				r.logger.Error("check failed", "name", name, "err", err)

				failed = append(failed, name)
				if verbose {
					r.fprintf(&output, "[-]%s failed: %v\n", name, err)
				}
			} else if verbose {
				r.fprintf(&output, "[+]%s ok\n", name)
			}
		}

		if len(failed) > 0 {
			w.WriteHeader(http.StatusInternalServerError)
			r.fprintf(w, "[-]%s failed\n", strings.Join(failed, ","))
		} else if verbose {
			r.fprintf(w, "%s checks passed\n", r.name)
		}

		if verbose {
			r.fprintf(w, "%s", output.String())
		}
	})
}

func (r *httpServiceZ) fprintf(w io.Writer, format string, a ...any) {
	_, err := fmt.Fprintf(w, format, a...)
	if err != nil {
		r.logger.Error("error while writing output for check", "name", r.name, "err", err)
	}
}
