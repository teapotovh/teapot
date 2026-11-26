package staticsite

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/teapotovh/teapot/lib/remotefiles"
)

func TestParseSitePath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		path     string
		username string
		rest     string
		ok       bool
	}{
		{name: "user root", path: "/~alice", username: "alice", ok: true},
		{name: "user root slash", path: "/~alice/", username: "alice", rest: "/", ok: true},
		{name: "nested", path: "/~alice/a/b", username: "alice", rest: "/a/b", ok: true},
		{name: "root", path: "/"},
		{name: "prefix only", path: "/~"},
		{name: "empty user", path: "/~/"},
		{name: "no prefix", path: "/alice"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			username, rest, ok := parseSitePath(test.path)
			if username != test.username || rest != test.rest || ok != test.ok {
				t.Fatalf(
					"parseSitePath(%q) = (%q, %q, %v), want (%q, %q, %v)",
					test.path,
					username,
					rest,
					ok,
					test.username,
					test.rest,
					test.ok,
				)
			}
		})
	}
}

func TestSitePath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		rest string
		want string
		ok   bool
	}{
		{name: "root", rest: "", want: "/site", ok: true},
		{name: "slash", rest: "/", want: "/site", ok: true},
		{name: "file", rest: "/index.html", want: "/site/index.html", ok: true},
		{name: "nested", rest: "/a/b", want: "/site/a/b", ok: true},
		{name: "parent", rest: "/../secret"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got, err := sitePath(test.rest)
			if (err == nil) != test.ok {
				t.Fatalf("sitePath(%q) error = %v, want ok %v", test.rest, err, test.ok)
			}

			if got != test.want {
				t.Fatalf("sitePath(%q) = %q, want %q", test.rest, got, test.want)
			}
		})
	}
}

func TestContentType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		file string
		want string
	}{
		{name: "html", file: "index.html", want: "text/html; charset=utf-8"},
		{name: "png", file: "logo.png", want: "image/png"},
		{name: "unknown", file: "data.unknown", want: defaultType},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			if got := contentType(test.file); got != test.want {
				t.Fatalf("contentType(%q) = %q, want %q", test.file, got, test.want)
			}
		})
	}
}

func TestCacheControl(t *testing.T) {
	t.Parallel()

	if got := cacheControl("index.html"); got != htmlCache {
		t.Fatalf("cacheControl(html) = %q, want %q", got, htmlCache)
	}

	if got := cacheControl("style.css"); got != assetCache {
		t.Fatalf("cacheControl(css) = %q, want %q", got, assetCache)
	}
}

func TestHandleServesIndex(t *testing.T) {
	t.Parallel()

	site := newTestSite(map[string]testFile{
		"/site":            {kind: remotefiles.KindDirectory},
		"/site/index.html": {kind: remotefiles.KindFile, data: "hello", hash: "abc"},
	})

	recorder := serve(t, site, http.MethodGet, "/~alice/", nil)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}

	if recorder.Body.String() != "hello" {
		t.Fatalf("body = %q, want %q", recorder.Body.String(), "hello")
	}

	if got := recorder.Header().Get("Content-Type"); got != "text/html; charset=utf-8" {
		t.Fatalf("Content-Type = %q, want html", got)
	}

	if got := recorder.Header().Get("Cache-Control"); got != htmlCache {
		t.Fatalf("Cache-Control = %q, want %q", got, htmlCache)
	}

	if got := recorder.Header().Get("ETag"); got != `"abc"` {
		t.Fatalf("ETag = %q, want %q", got, `"abc"`)
	}

	if got := recorder.Header().Get(headerContentSecurityPolicy); !strings.Contains(got, "sandbox") {
		t.Fatalf("Content-Security-Policy = %q, want sandbox", got)
	}

	if got := recorder.Header().Get(headerXContentTypeOptions); got != "nosniff" {
		t.Fatalf("X-Content-Type-Options = %q, want nosniff", got)
	}
}

func TestHandleServesAsset(t *testing.T) {
	t.Parallel()

	site := newTestSite(map[string]testFile{
		"/site/style.css": {kind: remotefiles.KindFile, data: "body{}"},
	})

	recorder := serve(t, site, http.MethodGet, "/~alice/style.css", nil)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}

	if got := recorder.Header().Get("Content-Type"); got != "text/css; charset=utf-8" {
		t.Fatalf("Content-Type = %q, want css", got)
	}

	if got := recorder.Header().Get("Cache-Control"); got != assetCache {
		t.Fatalf("Cache-Control = %q, want %q", got, assetCache)
	}
}

func TestHandleRedirectsDirectory(t *testing.T) {
	t.Parallel()

	site := newTestSite(map[string]testFile{
		"/site": {kind: remotefiles.KindDirectory},
	})

	recorder := serve(t, site, http.MethodGet, "/~alice", nil)

	if recorder.Code != http.StatusMovedPermanently {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusMovedPermanently)
	}

	if got := recorder.Header().Get("Location"); got != "/~alice/" {
		t.Fatalf("Location = %q, want %q", got, "/~alice/")
	}
}

func TestHandleConditionalRequest(t *testing.T) {
	t.Parallel()

	site := newTestSite(map[string]testFile{
		"/site/style.css": {kind: remotefiles.KindFile, data: "body{}", hash: "abc"},
	})

	recorder := serve(t, site, http.MethodGet, "/~alice/style.css", http.Header{
		"If-None-Match": []string{`"abc"`},
	})

	if recorder.Code != http.StatusNotModified {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNotModified)
	}
}

func TestHandleHeadHasNoBody(t *testing.T) {
	t.Parallel()

	site := newTestSite(map[string]testFile{
		"/site/style.css": {kind: remotefiles.KindFile, data: "body{}"},
	})

	recorder := serve(t, site, http.MethodHead, "/~alice/style.css", nil)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}

	if recorder.Body.Len() != 0 {
		t.Fatalf("body length = %d, want 0", recorder.Body.Len())
	}
}

func TestHandleUnknownUser(t *testing.T) {
	t.Parallel()

	site := newTestSite(map[string]testFile{
		"/site/index.html": {kind: remotefiles.KindFile, data: "hello"},
	})
	site.users = fakeUsers{}

	recorder := serve(t, site, http.MethodGet, "/~alice/", nil)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNotFound)
	}
}

func TestHandleMissingSite(t *testing.T) {
	t.Parallel()

	site := newTestSite(map[string]testFile{})

	recorder := serve(t, site, http.MethodGet, "/~alice/", nil)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNotFound)
	}
}

func TestHandleServesCustomNotFound(t *testing.T) {
	t.Parallel()

	site := newTestSite(map[string]testFile{
		"/site":          {kind: remotefiles.KindDirectory},
		"/site/404.html": {kind: remotefiles.KindFile, data: "missing"},
	})

	recorder := serve(t, site, http.MethodGet, "/~alice/missing", nil)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNotFound)
	}

	if recorder.Body.String() != "missing" {
		t.Fatalf("body = %q, want %q", recorder.Body.String(), "missing")
	}

	if got := recorder.Header().Get("Cache-Control"); got != errorCache {
		t.Fatalf("Cache-Control = %q, want %q", got, errorCache)
	}
}

func TestHandleRejectsMethod(t *testing.T) {
	t.Parallel()

	site := newTestSite(map[string]testFile{})

	recorder := serve(t, site, http.MethodPost, "/~alice/", nil)

	if recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusMethodNotAllowed)
	}

	if got := recorder.Header().Get("Allow"); got != "GET, HEAD" {
		t.Fatalf("Allow = %q, want %q", got, "GET, HEAD")
	}
}

func TestHandleRejectsTraversal(t *testing.T) {
	t.Parallel()

	site := newTestSite(map[string]testFile{
		"/site/secret": {kind: remotefiles.KindFile, data: "secret"},
	})

	recorder := serve(t, site, http.MethodGet, "/~alice/../secret", nil)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNotFound)
	}
}

type testFile struct {
	kind remotefiles.Kind
	data string
	hash string
}

type fakeUsers struct {
	uids map[string]int64
}

// UID implements Users.
func (u fakeUsers) UID(username string) (int64, bool) {
	if u.uids == nil {
		return 1, true
	}

	uid, ok := u.uids[username]

	return uid, ok
}

type fakeFiles struct {
	files map[int64]*fakeFileSystem
}

// For implements Files.
func (f fakeFiles) For(uid int64) FileSystem {
	return f.files[uid]
}

type fakeFileSystem struct {
	files map[string]testFile
}

// Stat implements FileSystem.
func (f *fakeFileSystem) Stat(_ context.Context, path string) (remotefiles.Inode, error) {
	file, ok := f.files[path]
	if !ok {
		return remotefiles.Inode{}, fmt.Errorf("%w: %s", remotefiles.ErrNotFound, path)
	}

	return remotefiles.Inode{
		Name: path,
		Kind: file.kind,
		Hash: remotefiles.Hash(file.hash),
	}, nil
}

// Open implements FileSystem.
func (f *fakeFileSystem) Open(_ context.Context, path string) (remotefiles.File, error) {
	file, ok := f.files[path]
	if !ok {
		return nil, fmt.Errorf("%w: %s", remotefiles.ErrNotFound, path)
	}

	return &fakeFile{
		Reader: bytes.NewReader([]byte(file.data)),
		inode:  remotefiles.Inode{Name: path, Kind: file.kind},
	}, nil
}

type fakeFile struct {
	*bytes.Reader

	inode remotefiles.Inode
}

// Close implements File.
func (f *fakeFile) Close() error {
	return nil
}

// Stat implements File.
func (f *fakeFile) Stat() (remotefiles.Inode, error) {
	return f.inode, nil
}

func newTestSite(files map[string]testFile) *StaticSite {
	return &StaticSite{
		logger: slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)),
		users:  fakeUsers{},
		files: fakeFiles{files: map[int64]*fakeFileSystem{
			1: {files: files},
		}},
	}
}

func serve(
	t *testing.T,
	site *StaticSite,
	method string,
	target string,
	headers http.Header,
) *httptest.ResponseRecorder {
	t.Helper()

	request := httptest.NewRequest(method, target, nil)

	for name, values := range headers {
		for _, value := range values {
			request.Header.Add(name, value)
		}
	}

	recorder := httptest.NewRecorder()
	site.handle(recorder, request)

	return recorder
}
