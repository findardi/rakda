package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"

	"github.com/findardi/Riksa-App/server/internal/platform/token"
	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func reqWithClaims(param, val, userID string) *http.Request {
	req := httptest.NewRequest(http.MethodGet, "/", nil)

	rctx := chi.NewRouteContext()
	if param != "" {
		rctx.URLParams.Add(param, val)
	}
	ctx := context.WithValue(req.Context(), chi.RouteCtxKey, rctx)

	if userID != "" {
		ctx = context.WithValue(ctx, claimsKey, &token.JwtClaims{ID: userID, Typ: token.TokenLogin})
	}

	return req.WithContext(ctx)
}

func spyHandler(called *bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*called = true
		w.WriteHeader(http.StatusOK)
	})
}

func TestRequirePermission(t *testing.T) {
	m := New(nil, nil, nil)

	t.Run("grants when permission present", func(t *testing.T) {
		called := false
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req = req.WithContext(context.WithValue(req.Context(), membershipKey, &Membership{
			Role:        "guest",
			Permissions: []string{"member:view", "role:view"},
		}))

		rec := httptest.NewRecorder()
		m.RequirePermission("member:view")(spyHandler(&called)).ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		assert.True(t, called)
	})

	t.Run("forbids when permission absent", func(t *testing.T) {
		called := false
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req = req.WithContext(context.WithValue(req.Context(), membershipKey, &Membership{
			Role:        "guest",
			Permissions: []string{"member:view"},
		}))

		rec := httptest.NewRecorder()
		m.RequirePermission("member:delete")(spyHandler(&called)).ServeHTTP(rec, req)

		assert.Equal(t, http.StatusForbidden, rec.Code)
		assert.False(t, called)
	})

	t.Run("fails closed when membership missing in context", func(t *testing.T) {
		called := false
		req := httptest.NewRequest(http.MethodGet, "/", nil)

		rec := httptest.NewRecorder()
		m.RequirePermission("member:view")(spyHandler(&called)).ServeHTTP(rec, req)

		assert.Equal(t, http.StatusInternalServerError, rec.Code)
		assert.False(t, called)
	})
}

func TestRequireMember(t *testing.T) {
	m := New(nil, nil, nil)

	t.Run("unauthorized when claims missing", func(t *testing.T) {
		called := false
		resolver := func(ctx context.Context, workspaceID, userID string) (*Membership, error) {
			return &Membership{}, nil
		}
		req := reqWithClaims("workspaceID", "ws1", "")

		rec := httptest.NewRecorder()
		m.RequireMember("workspaceID", resolver)(spyHandler(&called)).ServeHTTP(rec, req)

		assert.Equal(t, http.StatusUnauthorized, rec.Code)
		assert.False(t, called)
	})

	t.Run("forbids non-member (ErrResourceNotFound)", func(t *testing.T) {
		called := false
		resolver := func(ctx context.Context, workspaceID, userID string) (*Membership, error) {
			return nil, ErrResourceNotFound
		}
		req := reqWithClaims("workspaceID", "ws1", "user1")

		rec := httptest.NewRecorder()
		m.RequireMember("workspaceID", resolver)(spyHandler(&called)).ServeHTTP(rec, req)

		assert.Equal(t, http.StatusForbidden, rec.Code)
		assert.False(t, called)
	})

	t.Run("internal error on generic resolver failure", func(t *testing.T) {
		called := false
		resolver := func(ctx context.Context, workspaceID, userID string) (*Membership, error) {
			return nil, errors.New("db down")
		}
		req := reqWithClaims("workspaceID", "ws1", "user1")

		rec := httptest.NewRecorder()
		m.RequireMember("workspaceID", resolver)(spyHandler(&called)).ServeHTTP(rec, req)

		assert.Equal(t, http.StatusInternalServerError, rec.Code)
		assert.False(t, called)
	})

	t.Run("passes and stashes membership in context", func(t *testing.T) {
		want := &Membership{Role: "owner", Permissions: []string{"member:delete"}, Status: "active"}
		resolver := func(ctx context.Context, workspaceID, userID string) (*Membership, error) {
			assert.Equal(t, "ws1", workspaceID)
			assert.Equal(t, "user1", userID)
			return want, nil
		}

		var got *Membership
		var ok bool
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			got, ok = MembershipFromContext(r.Context())
			w.WriteHeader(http.StatusOK)
		})
		req := reqWithClaims("workspaceID", "ws1", "user1")

		rec := httptest.NewRecorder()
		m.RequireMember("workspaceID", resolver)(next).ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		assert.True(t, ok)
		assert.Equal(t, want, got)
	})
}

func mustPrefix(t *testing.T, s string) netip.Prefix {
	t.Helper()
	p, err := netip.ParsePrefix(s)
	require.NoError(t, err)
	return p
}

func TestResolveClientIP(t *testing.T) {
	loopback := []netip.Prefix{mustPrefix(t, "127.0.0.0/8")}

	t.Run("no trusted proxies: XFF ignored entirely", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = "10.0.0.1:5555"
		req.Header.Set("X-Forwarded-For", "1.1.1.1, 2.2.2.2")
		assert.Equal(t, "10.0.0.1", resolveClientIP(req, nil))
	})

	t.Run("trusted peer: rightmost non-trusted hop wins", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = "127.0.0.1:5555"
		req.Header.Set("X-Forwarded-For", "1.1.1.1, 2.2.2.2")
		assert.Equal(t, "2.2.2.2", resolveClientIP(req, loopback))
	})

	t.Run("all hops trusted: falls back to peer", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = "127.0.0.1:5555"
		req.Header.Set("X-Forwarded-For", "127.0.0.1")
		assert.Equal(t, "127.0.0.1", resolveClientIP(req, loopback))
	})

	t.Run("no hops: falls back to peer", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = "127.0.0.1:5555"
		assert.Equal(t, "127.0.0.1", resolveClientIP(req, loopback))
	})

	t.Run("X-Real-IP as single hop", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = "127.0.0.1:5555"
		req.Header.Set("X-Real-IP", "3.3.3.3")
		assert.Equal(t, "3.3.3.3", resolveClientIP(req, loopback))
	})

	t.Run("repeated XFF headers are all collected", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = "127.0.0.1:5555"
		req.Header.Add("X-Forwarded-For", "1.1.1.1")
		req.Header.Add("X-Forwarded-For", "2.2.2.2, 3.3.3.3")
		assert.Equal(t, "3.3.3.3", resolveClientIP(req, loopback))
	})

	t.Run("IPv4-mapped IPv6 peer matches IPv4 prefix after Unmap", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = "[::ffff:127.0.0.1]:5555"
		req.Header.Set("X-Forwarded-For", "4.4.4.4")
		assert.Equal(t, "4.4.4.4", resolveClientIP(req, loopback))
	})

	t.Run("garbage IP never trusted", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = "not-an-ip:5555"
		req.Header.Set("X-Forwarded-For", "5.5.5.5")
		assert.Equal(t, "not-an-ip", resolveClientIP(req, loopback))
	})
}

func TestRealIPStoresContext(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "10.0.0.9:9999"

	got := ""
	handler := RealIP(nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = ClientIP(r)
	}))
	handler.ServeHTTP(httptest.NewRecorder(), req)

	assert.Equal(t, "10.0.0.9", got)
}

func TestResolveClientIPIPv6LoopbackPeer(t *testing.T) {
	// Regresi: request dari localhost bisa datang sebagai [::1] (IPv6
	// loopback); tanpa prefix ::1, XFF akan diabaikan meski proxy tepercaya
	// dikonfigurasi untuk 127.0.0.0/8 saja.
	loopback := []netip.Prefix{mustPrefix(t, "127.0.0.0/8"), mustPrefix(t, "::1/128")}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "[::1]:5555"
	req.Header.Set("X-Forwarded-For", "203.0.113.5")
	assert.Equal(t, "203.0.113.5", resolveClientIP(req, loopback))
}
