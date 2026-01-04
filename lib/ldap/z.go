package ldap

import (
	"context"
	"fmt"

	"github.com/go-ldap/ldap/v3"

	"github.com/teapotovh/teapot/lib/observability"
)

func (f *Factory) canDialServer(ctx context.Context) (err error) {
	conn, err := ldap.DialURL(f.url)
	if err != nil {
		return fmt.Errorf("could not enstablish a connection to the LDAP server: %w", err)
	}

	defer func() {
		if e := conn.Close(); e != nil {
			err = fmt.Errorf("error while closing the ping LDAP connection: %w", err)
		}
	}()

	return nil
}

// ReadinessChecks implements observability.ReadinessChecks.
func (f *Factory) ReadinessChecks() map[string]observability.Check {
	return map[string]observability.Check{
		"ldap/connect": observability.CheckFunc(f.canDialServer),
	}
}

func (f *Factory) canBindAsRoot(ctx context.Context) (err error) {
	conn, err := ldap.DialURL(f.url)
	if err != nil {
		return fmt.Errorf("could not enstablish a connection to the LDAP server: %w", err)
	}

	defer func() {
		if e := conn.Close(); e != nil {
			err = fmt.Errorf("error while closing the ping LDAP connection: %w", err)
		}
	}()

	if err := conn.Bind(f.rootDN, f.rootPasswd); err != nil {
		return fmt.Errorf("error while binding as root: %w", err)
	}

	return nil
}

// LivenessChecks implements observability.LivenessChecks.
func (f *Factory) LivenessChecks() map[string]observability.Check {
	return map[string]observability.Check{
		"ldap/root-bind": observability.CheckFunc(f.canBindAsRoot),
	}
}
