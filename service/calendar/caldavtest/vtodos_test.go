package caldavtest

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAppleVTodos(t *testing.T) {
	t.Parallel()

	for i := range 5 {
		t.Run(fmt.Sprintf("vtodo %d", i+1), func(t *testing.T) {
			t.Parallel()

			// Ported from:
			// https://github.com/apple/ccs-caldavtester/blob/bed21e5924275552c1561febc8203a9f194cf737/scripts/tests/CalDAV/vtodos.xml#L31-L48
			srv := newTestServer(t)

			name := fmt.Sprintf("%d.ics", i+1)
			body := loadAppleFixture(t, "Resource/CalDAV/vtodos/"+name, map[string]string{
				"$uidrandom:": fmt.Sprintf("teapot-vtodo-%d@example.com", i+1),
			})

			res := putObject(t, srv, userOne, tasksPath(userOne)+name, body)
			assert.GreaterOrEqual(t, res.Status, 200)
			assert.Less(t, res.Status, 300, "body: %s", res.Body)
		})
	}
}
