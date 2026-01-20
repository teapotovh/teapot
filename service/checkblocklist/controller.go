package checkblocklist

import (
	v1 "k8s.io/api/core/v1"
)

func (cbl *CheckBlockList) handle(name string, node *v1.Node, exists bool) error {
	cbl.logger.Info("got update", "name", name, "node", node, "exists", exists)
	return nil
}
