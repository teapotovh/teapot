package observability

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"
)

type httpServiceZ struct {
	logger *slog.Logger

	name   string
	checks map[string]Check
}

var Timeout = time.Second * 5

type exec struct {
	name  string
	check Check

	err      error
	excluded bool
}

// Handler implements httpsrv.Handler.
//
//nolint:gocyclo
func (z *httpServiceZ) Handler(prefix string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), Timeout)
		defer cancel()

		// Individual check
		selector := r.PathValue("name")
		if selector != "" {
			check, ok := z.checks[selector]
			if !ok {
				http.NotFound(w, r)
				return
			}

			if err := check.Check(ctx); err != nil {
				z.logger.Error("check failed", "name", selector, "err", err)
				http.Error(w, fmt.Sprintf("[-]%s failed: %v", selector, err), http.StatusInternalServerError)

				return
			} else {
				z.fprintf(w, "[+]%s ok\n", selector)
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

		execs := make([]exec, 0, len(z.checks))
		for name, check := range z.checks {
			execs = append(execs, exec{
				name:     name,
				check:    check,
				excluded: excludeSet[name],
			})
		}

		var wg sync.WaitGroup

		for i, exec := range execs {
			if exec.excluded {
				continue
			}

			wg.Add(1)
			wg.Go(func() {
				defer wg.Done()

				if err := execs[i].check.Check(ctx); err != nil {
					z.logger.Error("check failed", "name", execs[i].name, "err", err)
					execs[i].err = err
				}
			})
		}

		wg.Wait()

		failed := false

		for _, exec := range execs {
			if exec.err != nil {
				failed = true
				break
			}
		}

		if verbose {
			var output strings.Builder

			for _, exec := range execs {
				if exec.excluded {
					z.fprintf(&output, "[excluded] %s\n", exec.name)
				} else if exec.err != nil {
					z.fprintf(&output, "[-]%s failed: %v\n", exec.name, exec.err)
				} else {
					z.fprintf(&output, "[+]%s ok\n", exec.name)
				}
			}

			z.fprintf(w, "%s", output.String())
		}

		if failed {
			w.WriteHeader(http.StatusInternalServerError)
		} else if verbose {
			z.fprintf(w, "%s checks passed\n", z.name)
		}
	})
}

func (z *httpServiceZ) fprintf(w io.Writer, format string, a ...any) {
	_, err := fmt.Fprintf(w, format, a...)
	if err != nil {
		z.logger.Error("error while writing output for check", "name", z.name, "err", err)
	}
}
