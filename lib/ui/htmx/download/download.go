package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/teapotovh/teapot/lib/ui/htmx"
)

const (
	version = "2.0.4"
	base    = "https://unpkg.com/%s@" + version + "/dist"
)

func main() {
	for _, component := range htmx.Components {
		var name string
		if component == "htmx.org" {
			name = "htmx"
		} else {
			name = strings.TrimPrefix(component, "htmx-ext-")
		}
		url := fmt.Sprintf(base+"/%s.min.js", component, name)
		log.Printf("Downloading component %q from %q", component, url)
		res, err := http.Get(url) //nolint:gosec
		if err != nil {
			panic(fmt.Errorf("error while fetching component %q: %w", component, err))
		}
		defer func() {
			if err := res.Body.Close(); err != nil {
				panic(fmt.Errorf("error while closing request body: %w", err))
			}
		}()

		contents, err := io.ReadAll(res.Body)
		if err != nil {
			panic(fmt.Errorf("error while reading response body for component %q: %w", component, err))
		}

		name = fmt.Sprintf("js/%s.js", name)
		if err := os.WriteFile(name, contents, 0o600); err != nil {
			panic(fmt.Errorf("error while writing downloaded file for component %q: %w", component, err))
		}
	}
}
