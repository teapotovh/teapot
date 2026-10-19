package checkblocklist

import (
	"context"

	"github.com/teapotovh/teapot/lib/observability"
)

func (cbl *CheckBlockList) canQueryDNS(ctx context.Context) error {
	return nil
}

// ReadinessChecks implements observability.ReadinessChecks.
func (cbl *CheckBlockList) ReadinessChecks() map[string]observability.Check {
	// TODO: re-export kubeclient, kubecontroller metrics
	return map[string]observability.Check{
		"checkblocklist/dns": observability.CheckFunc(cbl.canQueryDNS),
	}
}

// LivenessChecks implements observability.LivenessChecks.
func (cbl *CheckBlockList) LivenessChecks() map[string]observability.Check {
	// TODO: export checks from the kubeclient,kubecontroller
	return map[string]observability.Check{}
}
