package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	flag "github.com/spf13/pflag"

	"github.com/teapotovh/teapot/lib/log"
	"github.com/teapotovh/teapot/lib/observability"
	"github.com/teapotovh/teapot/lib/run"
	"github.com/teapotovh/teapot/service/mail"
	"github.com/teapotovh/teapot/service/mail/imap"
	"github.com/teapotovh/teapot/service/mail/lmtp"
)

const (
	CodeLog           = -1
	CodeObservability = -2
	CodeMail          = -3
	CodeIMAP          = -4
	CodeLMTP          = -5
	CodeRun           = -6
)

func main() {
	fs, getLogConfig := log.LogFlagSet()
	flag.CommandLine.AddFlagSet(fs)
	fs, getObservabilityConfig := observability.ObservabilityFlagSet("mail")
	flag.CommandLine.AddFlagSet(fs)
	fs, getMailConfig := mail.MailFlagSet()
	flag.CommandLine.AddFlagSet(fs)
	fs, getIMAPConfig := imap.ServerFlagSet()
	flag.CommandLine.AddFlagSet(fs)
	fs, getLMTPConfig := lmtp.ServerFlagSet()
	flag.CommandLine.AddFlagSet(fs)
	flag.Parse()

	logger, err := log.NewLogger(getLogConfig())
	if err != nil {
		slog.Error("error while configuring the logger", "err", err) //nolint:sloglint
		os.Exit(CodeLog)
	}

	logger = observability.WithHandler(logger)

	run := run.NewRun(run.RunConfig{Timeout: 5 * time.Second}, logger.With("sub", "run"))

	observability, err := observability.NewObservability(getObservabilityConfig(), logger.With("sub", "observability"))
	if err != nil {
		logger.Error("error while initiating the observability subsystem", "err", err)
		os.Exit(CodeObservability)
	}

	mailService, err := mail.NewMail(getMailConfig(), logger.With("sub", "mail"))
	if err != nil {
		logger.Error("error while initiating the mail subsystem", "err", err)
		os.Exit(CodeMail)
	}

	imapConfig, err := getIMAPConfig()
	if err != nil {
		logger.Error("error while reading the IMAP configuration", "err", err)
		os.Exit(CodeIMAP)
	}

	imapServer, err := imap.NewServer(imapConfig, mailService, logger.With("sub", "imap"))
	if err != nil {
		logger.Error("error while initiating the IMAP server", "err", err)
		os.Exit(CodeIMAP)
	}

	lmtpServer, err := lmtp.NewServer(getLMTPConfig(), mailService, logger.With("sub", "lmtp"))
	if err != nil {
		logger.Error("error while initiating the LMTP server", "err", err)
		os.Exit(CodeLMTP)
	}

	observability.RegisterMetrics(mailService)
	observability.RegisterReadyz(mailService)
	observability.RegisterMetrics(imapServer)
	observability.RegisterReadyz(imapServer)
	observability.RegisterLivez(imapServer)
	observability.RegisterMetrics(lmtpServer)
	observability.RegisterReadyz(lmtpServer)
	observability.RegisterLivez(lmtpServer)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	run.Add("mail/ldap", mailService.LDAPFactory(), nil)
	run.Add("mail/notifications", mailService.Notifications(), nil)
	run.Add("mail/store", mailService.Store(), nil)
	run.Add("mail/imap", imapServer, nil)
	run.Add("mail/lmtp", lmtpServer, nil)
	run.Add("observability", observability, nil)

	if err := run.Run(ctx); err != nil {
		logger.Error("error while running mail components", "err", err)
		os.Exit(CodeRun)
	}
}
