package staticsite

import (
	"bytes"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"github.com/teapotovh/teapot/lib/httplog"
	"github.com/teapotovh/teapot/lib/ldap"
	"github.com/teapotovh/teapot/lib/remotefiles"
	"github.com/teapotovh/teapot/lib/s3cache"
)

func TestNewStaticSite(t *testing.T) {
	t.Parallel()

	site, err := NewStaticSite(testConfig(t), testLogger())
	if err != nil {
		t.Fatalf("NewStaticSite: %v", err)
	}

	defer func() {
		if err := site.connection.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	}()

	if _, ok := site.users.UID("alice"); ok {
		t.Fatal("UID(alice) is cached, want an empty cache")
	}

	if len(site.ReadinessChecks()) == 0 {
		t.Fatal("ReadinessChecks() is empty")
	}

	if len(site.LivenessChecks()) == 0 {
		t.Fatal("LivenessChecks() is empty")
	}
}

func TestNewStaticSiteInvalidInterval(t *testing.T) {
	t.Parallel()

	config := testConfig(t)
	config.Users.Interval = 0

	if _, err := NewStaticSite(config, testLogger()); err == nil {
		t.Fatal("NewStaticSite error = nil, want an error")
	}
}

func TestStaticSiteFlagSetDefaults(t *testing.T) {
	t.Parallel()

	_, getConfig := StaticSiteFlagSet()

	config := getConfig()

	if config.Users.Interval != 5*time.Minute {
		t.Fatalf("Interval = %v, want %v", config.Users.Interval, 5*time.Minute)
	}

	if config.Files.Address != "localhost:8147" {
		t.Fatalf("Address = %q, want %q", config.Files.Address, "localhost:8147")
	}
}

func testConfig(t *testing.T) Config {
	t.Helper()

	dir := t.TempDir()

	return Config{
		HTTPLog: httplog.HTTPLogConfig{Level: "info"},
		LDAP:    ldap.LDAPConfig{},
		Users: UserCacheConfig{
			Interval: time.Minute,
		},
		Files: FilesConfig{
			Address: "localhost:8147",
		},
		RemoteFiles: remotefiles.Config{
			Cache: remotefiles.CacheConfig{
				Inodes:  remotefiles.TreeConfig{Path: filepath.Join(dir, "inodes")},
				Dirents: remotefiles.TreeConfig{Path: filepath.Join(dir, "dirents")},
			},
		},
		S3: S3Config{
			URL: "http://localhost:9000",
			Cache: s3cache.S3CacheConfig{
				Bucket: "test",
				Path:   dir,
			},
		},
	}
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
}
