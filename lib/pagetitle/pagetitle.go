package pagetitle

import "fmt"

func Title(page, app string) string {
	return fmt.Sprintf("%s — %s", page, app)
}
