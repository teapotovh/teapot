package ldap

import (
	"context"
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/teapotovh/teapot/lib/observability"
	"github.com/teapotovh/teapot/lib/run"
	"github.com/teapotovh/teapot/lib/tmplstring"
	"golang.org/x/sync/errgroup"
)

type LDAPConfig struct {
	URL            string
	Timeout        time.Duration
	MaxConnections uint32
	Workers        uint32

	RootDN     string
	RootPasswd string

	UsersDN      string
	UsersFilter  string
	GroupsDN     string
	GroupsFilter string
	AdminGroupDN string
	AccessesDN   string
}

// Factory is a constructor for high-level LDAP clients that can perform
// operations to manage users, as needed by kontakte.
// One client should be constructed per request.
type Factory struct {
	logger *slog.Logger

	pool       *pool
	workers    uint32
	rootDN     string
	rootPasswd string

	usersDN      string
	usersFilter  *tmplstring.TMPL[userFilterTemplateValues]
	groupsDN     string
	groupsFilter *tmplstring.TMPL[groupFilterTemplateValues]
	adminGroupDN string
	accessesDN   string

	clients atomic.Int32
	metrics metrics
}

func NewFactory(config LDAPConfig, logger *slog.Logger) (*Factory, error) {
	usersFilter, err := tmplstring.NewTMPL[userFilterTemplateValues](config.UsersFilter)
	if err != nil {
		return nil, fmt.Errorf("error while parsing user filter template: %w", err)
	}

	groupsFilter, err := tmplstring.NewTMPL[groupFilterTemplateValues](config.GroupsFilter)
	if err != nil {
		return nil, fmt.Errorf("error while parsing group filter template: %w", err)
	}

	pool := newPool(config.URL, config.RootDN, config.Timeout, config.MaxConnections, logger)

	fact := Factory{
		logger: logger,

		pool:       pool,
		workers:    config.Workers,
		rootDN:     config.RootDN,
		rootPasswd: config.RootPasswd,

		usersDN:      config.UsersDN,
		usersFilter:  usersFilter,
		groupsDN:     config.GroupsDN,
		groupsFilter: groupsFilter,
		adminGroupDN: config.AdminGroupDN,
		accessesDN:   config.AccessesDN,
	}

	fact.initMetrics()

	return &fact, nil
}

func (f *Factory) NewClient(ctx context.Context) (client *Client, err error) {
	ctx, span := observability.TracerFromContext(ctx).Start(ctx, "Factory.NewClient")
	defer func() { observability.SpanEnd(span, err) }()

	defer func() {
		f.clients.Add(1)

		if err != nil {
			f.metrics.total.WithLabelValues(metricsStatusError).Add(1)
		} else {
			f.metrics.active.Inc()
		}
	}()

	conn, err := f.pool.get(ctx)
	if err != nil {
		return nil, fmt.Errorf("could not enstablish a connection to the LDAP server: %w", err)
	}

	// We always bind as root user, so we can perform all operations,
	// including, possibly, a second bind as a lower-privilege user to test auth.
	if err := bind(ctx, &f.metrics, conn, f.rootDN, f.rootPasswd); err != nil {
		return nil, fmt.Errorf("error while binding as root: %w", err)
	}

	return &Client{
		logger: f.logger.With("client", f.clients.Load()),

		ctx:     ctx,
		conn:    conn,
		metrics: &f.metrics,

		usersDN:      f.usersDN,
		usersFilter:  f.usersFilter,
		groupsDN:     f.groupsDN,
		groupsFilter: f.groupsFilter,
		adminGroupDN: f.adminGroupDN,
		accessesDN:   f.accessesDN,
	}, nil
}

// Run implements run.Runnable.
func (f *Factory) Run(ctx context.Context, notify run.Notify) (err error) {
	eg, ctx := errgroup.WithContext(ctx)
	for i := range f.workers {
		eg.Go(func() error {
			return f.pool.fill(ctx, i, f.metrics.dial)
		})
	}

	notify.Notify()
	if err := eg.Wait(); err != nil {
		return err
	}

	return nil
}
