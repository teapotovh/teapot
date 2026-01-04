package ldap

import (
	"context"
	"fmt"
	"log/slog"
	"sync/atomic"

	"github.com/go-ldap/ldap/v3"

	"github.com/teapotovh/teapot/lib/tmplstring"
)

type LDAPConfig struct {
	URL        string
	RootDN     string
	RootPasswd string

	UsersDN      string
	UsersFilter  string
	GroupsDN     string
	AdminGroupDN string
	AccessesDN   string
}

// Factory is a constructor for high-level LDAP clients that can perform
// operations to manage users, as needed by kontakte.
// One client should be constructed per request.
type Factory struct {
	logger *slog.Logger

	url        string
	rootDN     string
	rootPasswd string

	usersFilter  *tmplstring.TMPL[filterTemplateValues]
	usersDN      string
	groupsDN     string
	adminGroupDN string
	accessesDN   string

	clients atomic.Int32
	metrics metrics
}

func NewFactory(config LDAPConfig, logger *slog.Logger) (*Factory, error) {
	usersFilter, err := tmplstring.NewTMPL[filterTemplateValues](config.UsersFilter)
	if err != nil {
		return nil, fmt.Errorf("error while parsing user filter template: %w", err)
	}

	fact := Factory{
		logger: logger,

		url:        config.URL,
		rootDN:     config.RootDN,
		rootPasswd: config.RootPasswd,

		usersDN:      config.UsersDN,
		usersFilter:  usersFilter,
		groupsDN:     config.GroupsDN,
		adminGroupDN: config.AdminGroupDN,
		accessesDN:   config.AccessesDN,
	}

	fact.initMetrics()

	return &fact, nil
}

func (f *Factory) NewClient(ctx context.Context) (client *Client, err error) {
	defer func() {
		f.clients.Add(1)

		if err != nil {
			f.metrics.total.WithLabelValues(metricsStatusError).Add(1)
		} else {
			f.metrics.active.Inc()
		}
	}()

	conn, err := ldap.DialURL(f.url)
	if err != nil {
		return nil, fmt.Errorf("could not enstablish a connection to the LDAP server: %w", err)
	}

	// We always bind as root user, so we can perofrm all operations,
	// including, possibly, a second bind as a lower-privilege user to test auth.
	if err := bind(&f.metrics, conn, f.rootDN, f.rootPasswd); err != nil {
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
		adminGroupDN: f.adminGroupDN,
		accessesDN:   f.accessesDN,
	}, nil
}
