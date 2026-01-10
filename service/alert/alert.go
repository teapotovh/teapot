package alert

import (
	"context"
	"errors"
	"log/slog"

	"github.com/google/go-github/v81/github"
	"github.com/teapotovh/teapot/lib/run"
)

var (
	ErrMissingToken = errors.New("GitHub access token not provided")
)

const (
	InFlightAlerts = 8
)

// Alert is the service instance for teapot's alerting system.
type Alert struct {
	logger *slog.Logger

	client  *github.Client
	alerts  chan alert
	metrics metrics

	owner string
	repo  string
}

// AlertConfig is the configuration for the Alert service.
type AlertConfig struct {
	Token string
	Owner string
	Repo  string
}

func NewAlert(config AlertConfig, logger *slog.Logger) (*Alert, error) {
	if len(config.Token) <= 0 {
		return nil, ErrMissingToken
	}
	client := github.NewClient(nil).WithAuthToken(config.Token)

	alert := Alert{
		logger: logger,

		client: client,
		alerts: make(chan alert, InFlightAlerts),

		owner: config.Owner,
		repo:  config.Repo,
	}

	alert.initMetrics()

	return &alert, nil
}

type alert struct {
}

// Run implements run.Runnable.
func (a *Alert) Run(ctx context.Context, notify run.Notify) error {
	defer close(a.alerts)

	notify.Notify()

	select {
	case <-ctx.Done():
		return nil
	case alert := <-a.alerts:
		a.logger.Info("sending alert", "alert", alert)
	}

	return nil
}
