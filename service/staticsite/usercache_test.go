package staticsite

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/teapotovh/teapot/lib/ldap"
	"github.com/teapotovh/teapot/lib/run"
)

var errLDAPDown = errors.New("LDAP is down")

func TestUserCacheRefresh(t *testing.T) {
	t.Parallel()

	lister := &fakeUserLister{users: []*ldap.User{
		{Username: "alice", UID: 1000},
		{Username: "bob", UID: 1001},
	}}

	cache, err := newUserCache(UserCacheConfig{Interval: time.Minute}, nil, lister, nil)
	if err != nil {
		t.Fatalf("newUserCache: %v", err)
	}

	if err := cache.refresh(context.Background()); err != nil {
		t.Fatalf("refresh: %v", err)
	}

	uid, ok := cache.UID("alice")
	if !ok || uid != 1000 {
		t.Fatalf("UID(alice) = (%d, %v), want (1000, true)", uid, ok)
	}

	uid, ok = cache.UID("bob")
	if !ok || uid != 1001 {
		t.Fatalf("UID(bob) = (%d, %v), want (1001, true)", uid, ok)
	}

	if _, ok := cache.UID("carol"); ok {
		t.Fatal("UID(carol) is cached, want no entry")
	}
}

func TestUserCacheRefreshKeepsOldUsersOnError(t *testing.T) {
	t.Parallel()

	lister := &fakeUserLister{users: []*ldap.User{{Username: "alice", UID: 1000}}}

	cache, err := newUserCache(UserCacheConfig{Interval: time.Minute}, nil, lister, nil)
	if err != nil {
		t.Fatalf("newUserCache: %v", err)
	}

	if err := cache.refresh(context.Background()); err != nil {
		t.Fatalf("refresh: %v", err)
	}

	lister.err = errLDAPDown

	if err := cache.refresh(context.Background()); err == nil {
		t.Fatal("refresh error = nil, want an error")
	}

	uid, ok := cache.UID("alice")
	if !ok || uid != 1000 {
		t.Fatalf("UID(alice) after failed refresh = (%d, %v), want (1000, true)", uid, ok)
	}
}

func TestUserCacheRunRefreshesPeriodically(t *testing.T) {
	t.Parallel()

	lister := &fakeUserLister{users: []*ldap.User{{Username: "alice", UID: 1000}}}

	cache, err := newUserCache(UserCacheConfig{Interval: 10 * time.Millisecond}, nil, lister, nil)
	if err != nil {
		t.Fatalf("newUserCache: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	step := &fakeStep{ctx: ctx, notified: make(chan struct{}, 1)}

	done := make(chan error, 1)

	go func() { done <- cache.Run(step) }()

	select {
	case <-step.notified:
	case <-time.After(time.Second):
		t.Fatal("Run did not notify")
	}

	deadline := time.Now().Add(time.Second)
	for lister.calls.Load() < 2 {
		if time.Now().After(deadline) {
			t.Fatalf("ListUsers calls = %d, want at least 2", lister.calls.Load())
		}

		time.Sleep(time.Millisecond)
	}

	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Run did not stop")
	}
}

func TestNewUserCacheInvalidInterval(t *testing.T) {
	t.Parallel()

	_, err := NewUserCache(UserCacheConfig{}, nil, nil)
	if !errors.Is(err, ErrInvalidInterval) {
		t.Fatalf("NewUserCache error = %v, want %v", err, ErrInvalidInterval)
	}
}

type fakeUserLister struct {
	users []*ldap.User
	err   error
	calls atomic.Int32
}

// ListUsers implements UserLister.
func (l *fakeUserLister) ListUsers(context.Context) ([]*ldap.User, error) {
	l.calls.Add(1)

	if l.err != nil {
		return nil, l.err
	}

	return l.users, nil
}

type fakeStep struct {
	ctx      context.Context
	notified chan struct{}
}

// Context implements run.Step.
func (s *fakeStep) Context() context.Context {
	return s.ctx
}

// Notify implements run.Step.
func (s *fakeStep) Notify() {
	select {
	case s.notified <- struct{}{}:
	default:
	}
}

// Add implements run.Step.
func (s *fakeStep) Add(string, run.Runnable) {}

// Wait implements run.Step.
func (s *fakeStep) Wait() {}
