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
	logger       *slog.Logger
	usersFilter  *tmplstring.TMPL[filterTemplateValues]
	url          string
	rootDN       string
	rootPasswd   string
	usersDN      string
	groupsDN     string
	adminGroupDN string
	accessesDN   string
	clients      atomic.Int32
}

func NewFactory(config LDAPConfig, logger *slog.Logger) (*Factory, error) {
	usersFilter, err := tmplstring.NewTMPL[filterTemplateValues](config.UsersFilter)
	if err != nil {
		return nil, fmt.Errorf("error while parsing user filter template: %w", err)
	}

	return &Factory{
		logger: logger,

		url:        config.URL,
		rootDN:     config.RootDN,
		rootPasswd: config.RootPasswd,

		usersDN:      config.UsersDN,
		usersFilter:  usersFilter,
		groupsDN:     config.GroupsDN,
		adminGroupDN: config.AdminGroupDN,
		accessesDN:   config.AccessesDN,
	}, nil
}

func (f *Factory) NewClient(ctx context.Context) (*Client, error) {
	conn, err := ldap.DialURL(f.url)
	if err != nil {
		return nil, fmt.Errorf("could not enstablish a connection to the LDAP server: %w", err)
	}

	// We always bind as root user, so we can perofrm all operations,
	// including, possibly, a second bind as a lower-privilege user to test auth.
	if err := conn.Bind(f.rootDN, f.rootPasswd); err != nil {
		return nil, fmt.Errorf("error while binding as root: %w", err)
	}

	defer func() { f.clients.Add(1) }()
	return &Client{
		logger: f.logger.With("client", f.clients.Load()),

		ctx:  ctx,
		conn: conn,

		usersDN:      f.usersDN,
		usersFilter:  f.usersFilter,
		groupsDN:     f.groupsDN,
		adminGroupDN: f.adminGroupDN,
		accessesDN:   f.accessesDN,
	}, nil
}
