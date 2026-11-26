package staticsite

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"path"
	"strings"

	"github.com/teapotovh/teapot/lib/fs"
	"github.com/teapotovh/teapot/lib/remotefiles"
)

const (
	// UsernamePrefix starts the path of a user static site.
	UsernamePrefix = "/~"
	// SiteRoot is the directory that holds a user static site.
	SiteRoot = "/site"
	// IndexFile is the file served for a directory.
	IndexFile = "index.html"
	// NotFoundFile is the custom 404 page of a user static site.
	NotFoundFile = "404.html"

	headerContentSecurityPolicy = "Content-Security-Policy"
	headerCrossOriginResource   = "Cross-Origin-Resource-Policy"
	headerReferrerPolicy        = "Referrer-Policy"
	headerXContentTypeOptions   = "X-Content-Type-Options"
	headerXFrameOptions         = "X-Frame-Options"

	sandboxPolicy = "sandbox allow-scripts allow-forms allow-popups allow-modals allow-downloads"
	htmlCache     = "no-cache"
	assetCache    = "public, max-age=3600"
	errorCache    = "no-store"
	defaultType   = "application/octet-stream"
)

// FileSystem is the read-only file system view of one user.
type FileSystem interface {
	// Stat returns the inode at path.
	Stat(ctx context.Context, path string) (remotefiles.Inode, error)
	// Open opens the file at path.
	Open(ctx context.Context, path string) (remotefiles.File, error)
}

// Files returns the file system view for one user.
type Files interface {
	// For returns the file system view for uid.
	For(uid int64) FileSystem
}

// Users looks up cached users.
type Users interface {
	// UID returns the uid of a username and reports whether the user is cached.
	UID(username string) (int64, bool)
}

func (s *StaticSite) handle(w http.ResponseWriter, r *http.Request) {
	setSecurityHeaders(w)

	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		w.Header().Set("Cache-Control", errorCache)
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)

		return
	}

	username, rest, ok := parseSitePath(r.URL.Path)
	if !ok {
		s.notFound(w, r, nil)

		return
	}

	uid, ok := s.users.UID(username)
	if !ok {
		s.notFound(w, r, nil)

		return
	}

	target, err := sitePath(rest)
	if err != nil {
		s.notFound(w, r, nil)

		return
	}

	if err := s.serveTarget(w, r, s.files.For(uid), target); err != nil {
		s.logger.ErrorContext(
			r.Context(),
			"error while serving static site",
			"username", username,
			"path", target,
			"err", err,
		)
		s.notFound(w, r, nil)
	}
}

func (s *StaticSite) serveTarget(
	w http.ResponseWriter,
	r *http.Request,
	files FileSystem,
	target string,
) error {
	inode, err := files.Stat(r.Context(), target)
	if err != nil {
		if errors.Is(err, remotefiles.ErrNotFound) {
			s.notFound(w, r, files)

			return nil
		}

		return fmt.Errorf("could not stat %q: %w", target, err)
	}

	if inode.Kind == remotefiles.KindDirectory {
		if !strings.HasSuffix(r.URL.Path, "/") {
			// The path is relative and always stays on the same host.
			http.Redirect(w, r, r.URL.Path+"/", http.StatusMovedPermanently) //nolint:gosec

			return nil
		}

		target = path.Join(target, IndexFile)

		inode, err = files.Stat(r.Context(), target)
		if err != nil {
			if errors.Is(err, remotefiles.ErrNotFound) {
				s.notFound(w, r, files)

				return nil
			}

			return fmt.Errorf("could not stat %q: %w", target, err)
		}
	}

	if inode.Kind != remotefiles.KindFile {
		s.notFound(w, r, files)

		return nil
	}

	return s.serveFile(w, r, files, target, inode)
}

func (s *StaticSite) serveFile(
	w http.ResponseWriter,
	r *http.Request,
	files FileSystem,
	target string,
	inode remotefiles.Inode,
) error {
	file, err := files.Open(r.Context(), target)
	if err != nil {
		return fmt.Errorf("could not open %q: %w", target, err)
	}

	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			s.logger.ErrorContext(r.Context(), "could not close static site file", "path", target, "err", closeErr)
		}
	}()

	setFileHeaders(w, inode)
	http.ServeContent(w, r, inode.Name, inode.ModTime, file)

	return nil
}

func (s *StaticSite) notFound(w http.ResponseWriter, r *http.Request, files FileSystem) {
	if files != nil {
		target := path.Join(SiteRoot, NotFoundFile)

		file, inode, err := openFile(r, files, target)
		if err == nil {
			defer func() {
				if closeErr := file.Close(); closeErr != nil {
					s.logger.WarnContext(r.Context(), "could not close 404 page", "err", closeErr)
				}
			}()

			setFileHeaders(w, inode)
			w.Header().Set("Cache-Control", errorCache)
			w.WriteHeader(http.StatusNotFound)

			if r.Method != http.MethodHead {
				if _, err := io.Copy(w, file); err != nil {
					s.logger.WarnContext(r.Context(), "could not write 404 page", "err", err)
				}
			}

			return
		}

		if !errors.Is(err, remotefiles.ErrNotFound) {
			s.logger.WarnContext(r.Context(), "could not open 404 page", "err", err)
		}
	}

	w.Header().Set("Cache-Control", errorCache)
	http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
}

func openFile(
	r *http.Request,
	files FileSystem,
	target string,
) (remotefiles.File, remotefiles.Inode, error) {
	inode, err := files.Stat(r.Context(), target)
	if err != nil {
		return nil, remotefiles.Inode{}, fmt.Errorf("could not stat %q: %w", target, err)
	}

	if inode.Kind != remotefiles.KindFile {
		return nil, remotefiles.Inode{}, fmt.Errorf("path %q is not a file: %w", target, remotefiles.ErrNotFound)
	}

	file, err := files.Open(r.Context(), target)
	if err != nil {
		return nil, remotefiles.Inode{}, fmt.Errorf("could not open %q: %w", target, err)
	}

	return file, inode, nil
}

func parseSitePath(value string) (username string, rest string, ok bool) {
	if !strings.HasPrefix(value, UsernamePrefix) {
		return "", "", false
	}

	value = strings.TrimPrefix(value, UsernamePrefix)
	if value == "" {
		return "", "", false
	}

	username, rest, found := strings.Cut(value, string(fs.PathSeparator))
	if username == "" {
		return "", "", false
	}

	if !found {
		return username, "", true
	}

	return username, string(fs.PathSeparator) + rest, true
}

func sitePath(rest string) (string, error) {
	clean, err := fs.CleanPath(string(fs.PathSeparator) + strings.TrimPrefix(rest, string(fs.PathSeparator)))
	if err != nil {
		return "", fmt.Errorf("could not clean site path: %w", err)
	}

	return path.Join(SiteRoot, clean), nil
}

func setSecurityHeaders(w http.ResponseWriter) {
	w.Header().Set(headerContentSecurityPolicy, sandboxPolicy)
	w.Header().Set(headerXContentTypeOptions, "nosniff")
	w.Header().Set(headerReferrerPolicy, "no-referrer")
	w.Header().Set(headerXFrameOptions, "DENY")
	w.Header().Set(headerCrossOriginResource, "same-origin")
}

func setFileHeaders(w http.ResponseWriter, inode remotefiles.Inode) {
	w.Header().Set("Content-Type", contentType(inode.Name))
	w.Header().Set("Cache-Control", cacheControl(inode.Name))

	if inode.Hash != "" {
		etag := strings.Trim(string(inode.Hash), `"`)
		w.Header().Set("ETag", `"`+etag+`"`)
	}
}

func contentType(name string) string {
	if value := mime.TypeByExtension(path.Ext(name)); value != "" {
		return value
	}

	return defaultType
}

func cacheControl(name string) string {
	extension := strings.ToLower(path.Ext(name))
	if extension == ".html" || extension == ".htm" {
		return htmlCache
	}

	return assetCache
}
