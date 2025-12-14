package tmplstring

import (
	"text/template/parse"
)

type unit struct{}

// exprsInTemplate walks a parsed template and lists all the field expressions
// involved.
func exprsInTemplate(n parse.Node) map[string]unit {
	used := map[string]unit{}

	var walk func(parse.Node)

	walk = func(node parse.Node) {
		switch x := node.(type) {
		case *parse.ActionNode:
			for _, c := range x.Pipe.Cmds {
				for _, a := range c.Args {
					if f, ok := a.(*parse.FieldNode); ok {
						if len(f.Ident) > 0 {
							used[f.Ident[0]] = unit{}
						}
					}
				}
			}
		case *parse.ListNode:
			for _, nn := range x.Nodes {
				walk(nn)
			}
		case *parse.IfNode:
			walk(x.List)

			if x.ElseList != nil {
				walk(x.ElseList)
			}
		case *parse.RangeNode:
			walk(x.List)

			if x.ElseList != nil {
				walk(x.ElseList)
			}
		case *parse.WithNode:
			walk(x.List)

			if x.ElseList != nil {
				walk(x.ElseList)
			}
		}
	}
	walk(n)

	return used
}
