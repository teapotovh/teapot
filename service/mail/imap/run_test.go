package imap_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	mailservice "github.com/teapotovh/teapot/service/mail"
	mailimap "github.com/teapotovh/teapot/service/mail/imap"
	"github.com/teapotovh/teapot/service/mail/mailtest"
	"github.com/teapotovh/teapot/service/mail/store"
)

func TestIMAPServerRunAndShutdown(t *testing.T) {
	t.Parallel()

	mem := store.NewMem()
	logger := testLogger()

	mailService, err := mailservice.NewMail(mailservice.Config{
		Store:         store.StoreConfig{Type: "mem"},
		StoreInstance: mem,
		Authenticator: testAuth{},
		Resolver:      testResolver{},
	}, logger)
	require.NoError(t, err)

	srv, err := mailimap.NewServer(mailimap.ServerConfig{
		Network:       "pipe",
		InsecureAuth:  true,
		Listener:      mailtest.NewPipeListener(),
		ShutdownDelay: time.Second,
	}, mailService, logger)
	require.NoError(t, err)

	require.Error(t, srv.ReadinessChecks()["imap/started"].Check(context.Background()))
	require.Error(t, srv.LivenessChecks()["imap/running"].Check(context.Background()))

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	ready := make(chan struct{})

	go func() {
		done <- srv.Run(ctx, notifyFunc(func() { close(ready) }))
	}()

	select {
	case <-ready:
	case <-time.After(time.Second):
		t.Fatal("server did not become ready")
	}

	require.NoError(t, srv.ReadinessChecks()["imap/started"].Check(context.Background()))
	require.NoError(t, srv.LivenessChecks()["imap/running"].Check(context.Background()))

	cancel()

	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("server did not shut down")
	}

	require.Error(t, srv.LivenessChecks()["imap/running"].Check(context.Background()))
}

type notifyFunc func()

func (f notifyFunc) Notify() { f() }
