package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	flag "github.com/spf13/pflag"

	"github.com/teapotovh/teapot/lib/ldapserver"
	"github.com/teapotovh/teapot/lib/log"
	"github.com/teapotovh/teapot/service/bottin"
)

const (
	CodeLog         = -1
	CodePasswd      = -2
	CodeConstruct   = -3
	CodeInit        = -4
	CodeListenLDAP  = -5
	CodeListenLDAPS = -6
	CodeNoListen    = -7
)

func main() {
	addr := flag.String("addr", "0.0.0.0:1389", "the address to listen at")

	fs, getBottinConfig := bottin.BottinFlagSet()
	flag.CommandLine.AddFlagSet(fs)
	fs, getLogConfig := log.LogFlagSet()
	flag.CommandLine.AddFlagSet(fs)
	flag.Parse()

	logger, err := log.NewLogger(getLogConfig())
	if err != nil {
		// This is the only place where we use the default slog logger,
		// as our internal one has not been setup yet.
		slog.Error("error while configuring the logger", "err", err) //nolint:sloglint
		os.Exit(CodeLog)
	}
	logger = logger.With("sub", "bottin")

	bottinConfig := getBottinConfig()
	ctx := context.Background()
	if bottinConfig.Passwd == "" {
		adminPassBytes := make([]byte, 8)
		_, err := rand.Read(adminPassBytes)
		if err != nil {
			logger.Error("error while generating random root password", "err", err)
			os.Exit(CodePasswd)
		}
		bottinConfig.Passwd = base64.RawURLEncoding.EncodeToString(adminPassBytes)
		logger.Info("using randomly generated root password", "passwd", bottinConfig.Passwd)
	}

	bottin, err := bottin.NewBottin(bottinConfig, logger.With("sub", "bottin"))
	if err != nil {
		logger.Error("error while constructing server", "err", err)
		os.Exit(CodeConstruct)
	}
	if err = bottin.Init(ctx); err != nil {
		logger.Error("error while initializing ldap server", "err", err)
		os.Exit(CodeInit)
	}

	// Create routes
	routes := ldapserver.NewRouteMux(logger.With("sub", "router"))

	routes.Bind(bottin.HandleBind)
	routes.Search(bottin.HandleSearch)
	routes.Add(bottin.HandleAdd)
	routes.Compare(bottin.HandleCompare)
	routes.Delete(bottin.HandleDelete)
	routes.Modify(bottin.HandleModify)
	routes.Extended(bottin.HandlePasswordModify).
		RequestName(ldapserver.NoticeOfPasswordModify).Label("PasswordModify")

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	ldapServer := ldapserver.NewServer(logger.With("component", "ldapserver", "kind", "ldap"))
	ldapServer.Handle(routes)
	go func() {
		logger.Info("listening ldap://", "addr", *addr)
		if err := ldapServer.ListenAndServe(ctx, *addr); err != nil {
			logger.Error("error while listening on ldap://", "addr", *addr, "err", err)
			os.Exit(CodeListenLDAP)
		}
	}()

	<-ctx.Done()

	if ldapServer != nil {
		ldapServer.Wait()
	}
}
