package ldap

import (
	"context"
	"fmt"
	"log/slog"
	"sync/atomic"
	"text/template"

	"github.com/go-ldap/ldap/v3"
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
	usersFilter  *template.Template
	url          string
	rootDN       string
	rootPasswd   string
	usersDN      string
	groupsDN     string
	adminGroupDN string
	accessesDN   string
	clients      atomic.Int32
}

func NewFactory(options LDAPConfig, logger *slog.Logger) (*Factory, error) {
	usersFilter, err := template.New("usersFilter").Parse(options.UsersFilter)
	if err != nil {
		return nil, fmt.Errorf("error while parsing user filter template: %w", err)
	}

	return &Factory{
		logger: logger,

		url:        options.URL,
		rootDN:     options.RootDN,
		rootPasswd: options.RootPasswd,

		usersDN:      options.UsersDN,
		usersFilter:  usersFilter,
		groupsDN:     options.GroupsDN,
		adminGroupDN: options.AdminGroupDN,
		accessesDN:   options.AccessesDN,
	}, nil
}

func (f *Factory) NewClient(ctx context.Context) (*Client, error) {
	conn, err := ldap.DialURL(f.url)
	if err != nil {
		return nil, fmt.Errorf("could not enstablish a connection to the LDAP server: %w", err)
	}

	// We always bind as root user, so we can perofrm all operations,
	// including, possibly, a second bind as a lower-priviledge user to test auth.
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
