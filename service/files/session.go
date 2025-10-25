package files

import (
	"fmt"
	"io/fs"
	"log/slog"
	"time"

	"github.com/ammario/tlru"
	hpfs "github.com/hack-pad/hackpadfs"
	hpfsmem "github.com/hack-pad/hackpadfs/mem"
	hpfsmount "github.com/hack-pad/hackpadfs/mount"
	flag "github.com/spf13/pflag"
)

func SessionsFlagSet() (*flag.FlagSet, func() SessionsConfig) {
	fs := flag.NewFlagSet("sessions", flag.ExitOnError)

	mounts := fs.StringSliceP("files-mount", "m", nil, "declare a list of mount points")
	cacheSize := fs.Int("session-cache-size", 128, "how many sessions to cache in memory")
	cacheLifetime := fs.Duration("session-cache-lifetime", time.Hour*4, "how long to cache sessions")

	return fs, func() SessionsConfig {
		return SessionsConfig{
			Mounts:        *mounts,
			CacheSize:     *cacheSize,
			CacheLifetime: *cacheLifetime,
		}
	}
}

type SessionsCache = *tlru.Cache[string, *Session]

type Sessions struct {
	logger *slog.Logger

	mounts        []mount
	cache         SessionsCache
	cacheLifetime time.Duration
}

type SessionsConfig struct {
	Mounts        []string
	CacheSize     int
	CacheLifetime time.Duration
}

func NewSessions(config SessionsConfig, logger *slog.Logger) (*Sessions, error) {
	var mounts []mount
	for _, m := range config.Mounts {
		mc, err := parseRawMount(m)
		if err != nil {
			return nil, err
		}

		mount, err := parseMountConfig(mc)
		if err != nil {
			return nil, err
		}

		mounts = append(mounts, mount)
	}

	return &Sessions{
		logger: logger,

		mounts:        mounts,
		cache:         tlru.New[string, *Session](nil, config.CacheSize),
		cacheLifetime: config.CacheLifetime,
	}, nil
}

func (s *Sessions) Get(username string) (*Session, error) {
	session, err := s.cache.Do(username, s.newSessionFn(username), s.cacheLifetime)
	if err != nil {
		s.logger.Error("error while constructing session", "username", username, "err", err)
	}

	return session, err
}

func (s *Sessions) newSessionFn(username string) func() (*Session, error) {
	return func() (*Session, error) {
		s.logger.Debug("constructing seesion", "username", username)

		memFS, err := hpfsmem.NewFS()
		if err != nil {
			return nil, fmt.Errorf("error while creating root memory filesystem: %w", err)
		}
		readonlyFS := NewReadOnlyFS(memFS)
		rootFS, err := hpfsmount.NewFS(readonlyFS)
		if err != nil {
			return nil, fmt.Errorf("error while creating root mount filesystem: %w", err)
		}

		// mount all mountpoints
		for _, mount := range s.mounts {
			src, err := mount.src(username)
			if err != nil {
				return nil, fmt.Errorf("error while getting filesystem for mountpoint '%s': %w", mount.dst, err)
			}

			if err = memFS.MkdirAll(mount.dst, fs.ModeDir); err != nil {
				return nil, fmt.Errorf("error while creating mounting directory at '%s': %w", mount.dst, err)
			}

			if err = rootFS.AddMount(mount.dst, src); err != nil {
				return nil, fmt.Errorf("error while mounting '%s' filesystem at '%s': %w", mount.vfs, mount.dst, err)
			}
		}

		return &Session{
			fs: rootFS,
		}, nil
	}
}

type Session struct {
	fs hpfs.FS
}

func (s *Session) FS() hpfs.FS {
	return s.fs
}
