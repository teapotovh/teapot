package initialize

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/teapotovh/teapot/lib/kubeutil"
	"github.com/teapotovh/teapot/lib/run"
	"github.com/teapotovh/teapot/service/ccm"
)

type Initialize struct {
	logger *slog.Logger
	ccm    *ccm.CCM
}

func NewInitialize(ccm *ccm.CCM, logger *slog.Logger) (*Initialize, error) {
	return &Initialize{
		logger: logger,
		ccm:    ccm,
	}, nil
}

// Run implements run.Runnable
func (iip *Initialize) Run(ctx context.Context, notify run.Notify) error {
	sub := iip.ccm.Broker().Subscribe()
	defer sub.Unsubscribe()

	notify.Notify()
	for {
		select {
		case <-ctx.Done():
			return nil
		case event := <-sub.Chan():
			iip.logger.Debug("received CCM event", "event", event)

			if !event.InternalIP.IsValid() || !event.ExternalIP.IsValid() {
				continue
			}
			iip.logger.Info("node initialization complete", "node", event.Node, "internalIP", event.InternalIP, "externalIP", event.ExternalIP)

			err := kubeutil.RemoveTaint(ctx, iip.ccm.KubeClient(), event.Node, "node.cloudprovider.kubernetes.io/uninitialized", "true")
			if err != nil {
				return fmt.Errorf("error while removing uninitialized taint from node %q: %w", event.Node, err)
			}
		}
	}
}
