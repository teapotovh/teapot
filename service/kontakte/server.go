package kontakte

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/kataras/muxie"
	g "maragu.dev/gomponents"
	gcohttp "maragu.dev/gomponents/http"

	"github.com/teapotovh/teapot/lib/ldap"
)

const (
	PathStyle  = "/style.css"
	PathIndex  = "/"
	PathLogin  = "/login"
	PathLogout = "/logout"

	PathUsers          = "/users"
	PathUserTemplate   = PathUsers + "/:username"
	PathPasswdTemplate = PathUsers + "/:username/passwd"

	PathGroups = "/groups"
)

func PathUser(username string) string {
	return fmt.Sprintf("%s/%s", PathUsers, username)
}

func PathPasswd(username string) string {
	return fmt.Sprintf("%s/%s/passwd", PathUsers, username)
}

var ErrLDAP = errors.New("error while performing LDAP operation")

// Server is the kontakte HTTP server that renders web pages for users to
// modify their LDAP information.
type Server struct {
	logger  *slog.Logger
	factory *ldap.Factory
	mux     *muxie.Mux
	http    *http.Server
	// groupsDN     string
	// adminGroup   string
	// accessesDN   string
	jwtSecret []byte
}

type ServerConfig struct {
	Addr      string
	JWTSecret string

	FactoryOptions ldap.LDAPConfig
}

var (
	ErrRedirect = errors.New("redirect")
)

func Adapt(h gcohttp.Handler) http.Handler {
	fn := func(w http.ResponseWriter, r *http.Request) (g.Node, error) {
		log := getLogger(r.Context())
		node, err := h(w, r)
		if errors.Is(err, ErrRedirect) {
			err = nil
		}
		if err != nil && !errors.Is(err, ErrNotFound) {
			log.ErrorContext(r.Context(), "error handling request", "err", err)
		}
		return node, err
	}

	return gcohttp.Adapt(fn)
}

func NewServer(options ServerConfig, logger *slog.Logger) (*Server, error) {
	factory, err := ldap.NewFactory(options.FactoryOptions, logger.With("component", "ldap"))
	if err != nil {
		return nil, fmt.Errorf("error while building LDAP factory: %w", err)
	}

	mux := muxie.NewMux()
	httpsrv := http.Server{
		Addr:              options.Addr,
		ReadHeaderTimeout: time.Minute,
		Handler:           mux,
	}

	srv := &Server{
		logger:  logger,
		factory: factory,

		jwtSecret: []byte(options.JWTSecret),

		mux:  mux,
		http: &httpsrv,
	}

	mux.PathCorrection = true
	mux.Use(
		srv.AuthMiddleware,
		LogMiddleware,
	)
	mux.HandleFunc(PathIndex, srv.HandleIndex)
	mux.HandleFunc(PathStyle, srv.HandleStyle)
	mux.Handle("/*path", Adapt(srv.HandleNotFound))

	mux.Handle(PathLogin, muxie.Methods().
		Handle(http.MethodGet, Adapt(srv.HandleLoginGet)).
		Handle(http.MethodPost, Adapt(srv.HandleLoginPost)),
	)
	mux.HandleFunc(PathLogout, srv.HandleLogout)

	mux.Handle(PathUsers, Adapt(srv.HandleUsersGet))
	mux.Handle(PathUserTemplate, muxie.Methods().
		Handle(http.MethodGet, Adapt(srv.HandleUserGet)),
	)
	mux.Handle(PathPasswdTemplate, muxie.Methods().
		Handle(http.MethodGet, Adapt(srv.HandlePasswdGet)).
		Handle(http.MethodPost, Adapt(srv.HandlePasswdPost)),
	)

	return srv, nil
}

func (srv *Server) Listen() error {
	srv.logger.Info("listening", "addr", srv.http.Addr)
	return srv.http.ListenAndServe()
}
