package calendar

import (
	flag "github.com/spf13/pflag"

	"github.com/teapotovh/teapot/lib/httplog"
	"github.com/teapotovh/teapot/lib/ldap"
	"github.com/teapotovh/teapot/service/calendar/store"
)

func CalendarFlagSet() (*flag.FlagSet, func() CalendarConfig) {
	fs := flag.NewFlagSet("calendar", flag.ExitOnError)

	httpLogFS, getHTTPLogConfig := httplog.HTTPLogFlagSet()
	fs.AddFlagSet(httpLogFS)

	ldapFS, getLDAPConfig := ldap.LDAPFlagSet()
	fs.AddFlagSet(ldapFS)

	storeFS, getStoreConfig := store.StoreFlagSet()
	fs.AddFlagSet(storeFS)

	return fs, func() CalendarConfig {
		return CalendarConfig{
			HTTPLog: getHTTPLogConfig(),
			LDAP:    getLDAPConfig(),
			Store:   getStoreConfig(),
		}
	}
}
