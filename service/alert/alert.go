package alert

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/go-github/v81/github"
	"github.com/teapotovh/teapot/lib/run"
)

var (
	ErrMissingToken = errors.New("GitHub access token not provided")
)

const (
	InFlightAlerts = 8
)

type AlertData struct {
	ID          string
	Title       string
	Description string
	Time        time.Time
	Labels      []string

	// Details can be arbitrary Markdown that will be formatted in a <details> element.
	Details string
}

// Alert is the service instance for teapot's alerting system.
type Alert struct {
	logger *slog.Logger

	client  *github.Client
	alerts  chan AlertData
	metrics metrics

	owner     string
	repo      string
	assignees []string

	maxRetries uint64
}

// AlertConfig is the configuration for the Alert service.
type AlertConfig struct {
	Token     string
	Owner     string
	Repo      string
	Assignees []string

	MaxRetries uint64
}

func NewAlert(config AlertConfig, logger *slog.Logger) (*Alert, error) {
	if len(config.Token) <= 0 {
		return nil, ErrMissingToken
	}
	client := github.NewClient(nil).WithAuthToken(config.Token)

	alert := Alert{
		logger: logger,

		client: client,
		alerts: make(chan AlertData, InFlightAlerts),

		owner:     config.Owner,
		repo:      config.Repo,
		assignees: config.Assignees,

		maxRetries: config.MaxRetries,
	}

	alert.initMetrics()

	return &alert, nil
}

// Run implements run.Runnable.
func (a *Alert) Run(ctx context.Context, notify run.Notify) error {
	defer close(a.alerts)

	notify.Notify()

	select {
	case <-ctx.Done():
		return nil
	case alert := <-a.alerts:
		logger := a.logger.With("alert", alert.ID)
		logger.Info("sending alert", "title", alert.Title, "time", alert.Time, "labels", alert.Labels)

		id, err := a.createGitHubIssue(ctx, alert)
		if err != nil {
			return fmt.Errorf("error while creating issue for alert: %w", err)
		}

		logger.Info("opened GitHub issue for alert", "issue", id)
	}

	return nil
}

func (a *Alert) Fire(alert AlertData) {
	a.alerts <- alert
}
