package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"

	"github.com/teapotovh/teapot/lib/ui/openprops"
)

const version = "1.7.16"
const base = "https://app.unpkg.com/open-props@" + version

func main() {
	for _, component := range openprops.Components {
		url := fmt.Sprintf("%s/%s.min.css", base, component)
		log.Printf("Downloading component %q from %q", component, url)
		res, err := http.Get(url)
		if err != nil {
			panic(fmt.Errorf("error while fetching component %q: %w", component, err))
		}

		contents, err := io.ReadAll(res.Body)
		if err != nil {
			panic(fmt.Errorf("error while reading response body for component %q: %w", component, err))
		}

		name := fmt.Sprintf("css/%s.css", component)
		if err := os.WriteFile(name, contents, 0660); err != nil {
			panic(fmt.Errorf("error while writing downloaded file for component %q: %w", component, err))
		}
	}
}
