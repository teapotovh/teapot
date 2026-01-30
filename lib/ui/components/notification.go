package components

import (
	g "maragu.dev/gomponents"
	c "maragu.dev/gomponents/components"
	h "maragu.dev/gomponents/html"

	"github.com/teapotovh/teapot/lib/ui"
)

var NotificationStyle = ui.MustParseStyle(`
  padding: var(--size-2) var(--size-1);
`)

func Notification(ctx ui.Context, children ...g.Node) g.Node {
	return h.Div(c.JoinAttrs("class", g.Group(children), ctx.Class(NotificationStyle)))
}

var ErrorNotificationStyle = ui.MustParseStyle(`
  background: var(--theme-error-0);
  border: var(--size-1) solid var(--theme-error-1);
  color: var(--theme-background-9);
`)

func ErrorNotification(ctx ui.Context, err error) g.Node {
	return Notification(ctx, ctx.Class(ErrorNotificationStyle), g.Group{g.Textf("Error: %s", err)})
}

var SuccessNotificationStyle = ui.MustParseStyle(`
  background: var(--theme-success-0);
  border: var(--size-1) solid var(--theme-success-1);
  color: var(--theme-background-9);
`)

func SuccessNotification(ctx ui.Context, msg g.Node) g.Node {
	return Notification(ctx, ctx.Class(SuccessNotificationStyle), g.Group{msg})
}
