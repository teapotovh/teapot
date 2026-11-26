package staticsite

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/teapotovh/teapot/lib/httplog"
	"github.com/teapotovh/teapot/lib/ldap"
	"github.com/teapotovh/teapot/lib/remotefiles"
	"github.com/teapotovh/teapot/lib/run"
	"github.com/teapotovh/teapot/lib/s3cache"
	filespb "github.com/teapotovh/teapot/proto/files"
)

// FilesConfig configures the files service connection.
type FilesConfig struct {
	// Address is the gRPC address of the files service.
	Address string
}

// S3Config configures the S3 blob store.
type S3Config struct {
	// URL is the S3 endpoint with optional credentials.
	URL string
	// Region is the S3 region.
	Region string
	// Cache is the S3 cache configuration.
	Cache s3cache.S3CacheConfig
}

// Config configures the static site service.
type Config struct {
	HTTPLog     httplog.HTTPLogConfig
	LDAP        ldap.LDAPConfig
	Users       UserCacheConfig
	Files       FilesConfig
	RemoteFiles remotefiles.Config
	S3          S3Config
}

// StaticSite serves static sites from the files store.
type StaticSite struct {
	logger *slog.Logger

	httpLog     *httplog.HTTPLog
	ldapFactory *ldap.Factory
	users       Users
	userCache   *UserCache
	files       Files
	remoteFiles *remotefiles.RemoteFiles
	connection  *grpc.ClientConn
}

type remoteFiles struct {
	files *remotefiles.RemoteFiles
}

// For implements Files.
func (r remoteFiles) For(uid int64) FileSystem {
	return r.files.For(uid)
}

// NewStaticSite creates a static site service.
func NewStaticSite(config Config, logger *slog.Logger) (*StaticSite, error) {
	httpLog, err := httplog.NewHTTPLog(config.HTTPLog, logger.With("component", "httplog"))
	if err != nil {
		return nil, fmt.Errorf("error while constructing httplog: %w", err)
	}

	ldapFactory, err := ldap.NewFactory(config.LDAP, logger.With("component", "ldap"))
	if err != nil {
		return nil, fmt.Errorf("error while building LDAP factory: %w", err)
	}

	users, err := NewUserCache(config.Users, ldapFactory, logger.With("component", "users"))
	if err != nil {
		return nil, fmt.Errorf("error while constructing user cache: %w", err)
	}

	client, err := s3cache.NewClient(config.S3.URL, config.S3.Region, nil)
	if err != nil {
		return nil, fmt.Errorf("error while creating the S3 client: %w", err)
	}

	blobs, err := s3cache.NewS3Cache(config.S3.Cache, client, logger.With("component", "s3cache"))
	if err != nil {
		return nil, fmt.Errorf("error while building s3 cache: %w", err)
	}

	connection, err := grpc.NewClient(
		config.Files.Address,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return nil, fmt.Errorf("error while connecting to the files service: %w", err)
	}

	files, err := remotefiles.NewRemoteFiles(
		config.RemoteFiles,
		filespb.NewFilesClient(connection),
		blobs,
		logger.With("component", "remotefiles"),
	)
	if err != nil {
		return nil, errors.Join(
			fmt.Errorf("error while constructing remote files: %w", err),
			connection.Close(),
		)
	}

	return &StaticSite{
		logger: logger,

		httpLog:     httpLog,
		ldapFactory: ldapFactory,
		users:       users,
		userCache:   users,
		files:       remoteFiles{files: files},
		remoteFiles: files,
		connection:  connection,
	}, nil
}

// Handler implements httpsrv.HTTPService.
func (s *StaticSite) Handler(_ string) http.Handler {
	var handler http.Handler = http.HandlerFunc(s.handle)

	handler = s.httpLog.LogMiddleware(handler)
	handler = s.httpLog.ExtractMiddleware(handler)

	return handler
}

// Run implements run.Runnable.
func (s *StaticSite) Run(step run.Step) (err error) {
	defer func() {
		err = errors.Join(err, s.connection.Close())
	}()

	step.Add("files", s.remoteFiles)
	step.Add("users", s.userCache)
	step.Wait()
	step.Notify()

	<-step.Context().Done()

	return nil
}
