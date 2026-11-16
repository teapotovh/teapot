package lmtp

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	mailservice "github.com/teapotovh/teapot/service/mail"
	"github.com/teapotovh/teapot/service/mail/mailtest"
	"github.com/teapotovh/teapot/service/mail/store"
)

func TestLMTPServerRunAndShutdown(t *testing.T) {
	t.Parallel()

	mem := store.NewMem()
	logger := slog.New(slog.DiscardHandler)

	mailService, err := mailservice.NewMail(mailservice.Config{
		Store:         store.StoreConfig{Type: "mem"},
		StoreInstance: mem,
		Authenticator: testAuth{},
		Resolver:      StaticResolver{aliceAddress: aliceOwner},
	}, logger)
	require.NoError(t, err)

	srv, err := NewServer(Config{
		Network:       "tcp",
		Listener:      mailtest.NewPipeListener(),
		Domain:        testDomain,
		ShutdownDelay: time.Second,
	}, mailService, logger)
	require.NoError(t, err)

	require.Error(t, srv.ReadinessChecks()["lmtp/started"].Check(context.Background()))
	require.Error(t, srv.LivenessChecks()["lmtp/running"].Check(context.Background()))

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	ready := make(chan struct{})

	go func() {
		done <- srv.Run(ctx, lmtpNotifyFunc(func() { close(ready) }))
	}()

	select {
	case <-ready:
	case <-time.After(time.Second):
		t.Fatal("server did not become ready")
	}

	require.NoError(t, srv.ReadinessChecks()["lmtp/started"].Check(context.Background()))
	require.NoError(t, srv.LivenessChecks()["lmtp/running"].Check(context.Background()))

	cancel()

	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("server did not shut down")
	}

	require.Error(t, srv.LivenessChecks()["lmtp/running"].Check(context.Background()))
}

type lmtpNotifyFunc func()

func (f lmtpNotifyFunc) Notify() { f() }
