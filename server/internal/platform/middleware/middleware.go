package middleware

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"math"
	"net"
	"net/http"
	"net/netip"
	"strconv"
	"strings"
	"time"

	"github.com/findardi/rakda/server/internal/platform/permission"
	"github.com/findardi/rakda/server/internal/platform/response"
	"github.com/findardi/rakda/server/internal/platform/token"
	"github.com/go-chi/chi/v5"
)

type TokenVerifier interface {
	VerifyToken(tokenString string) (*token.JwtClaims, error)
}

type StatusReader interface {
	UserStatus(ctx context.Context, userID string) (string, error)
}

// ErrResourceNotFound is returned by an OwnerResolver when the resource does
// not exist; RequireOwner maps it to 404.
var ErrResourceNotFound = errors.New("resource not found")

// OwnerResolver resolves the owner (creator) user id of the resource identified
// by id. Each domain (workspace, folder, ...) supplies its own resolver so the
// middleware stays decoupled from any domain. Return ErrResourceNotFound when
// the resource is absent.
type OwnerResolver func(ctx context.Context, id string) (ownerID string, err error)

// Membership is the caller's resolved standing in a workspace: their role name,
// the flattened permission set of that role, and the member status. RequireMember
// loads it into the request context; RequirePermission reads it.
type Membership struct {
	Role            string   `json:"role"`
	Permissions     []string `json:"permissions"`
	Status          string   `json:"status"`
	WorkspaceStatus string   `json:"workspace_status"`
}

type MemberResolver func(ctx context.Context, workspaceID string, userID string) (*Membership, error)

type RateStore interface {
	Allow(key string, limit int, window time.Duration) (allowed bool, retryAfter time.Duration)
}

type KeyFunc func(r *http.Request) string

type RateConfig struct {
	Name   string
	Limit  int
	Window time.Duration
	Key    KeyFunc
}

type ctxKey string

const claimsKey ctxKey = "auth_claims"

const membershipKey ctxKey = "auth_membership"

const clientIPKey ctxKey = "client_ip"

type Middleware struct {
	verifier TokenVerifier
	status   StatusReader
	limiter  RateStore
}

func New(verifier TokenVerifier, status StatusReader, limiter RateStore) *Middleware {
	return &Middleware{
		verifier: verifier,
		status:   status,
		limiter:  limiter,
	}
}

func (m *Middleware) RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header := r.Header.Get("Authorization")
		parts := strings.SplitN(header, " ", 2)

		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			response.Error(w, http.StatusUnauthorized, "missing or invalid authorization", nil)
			return
		}

		claims, err := m.verifier.VerifyToken(parts[1])
		if err != nil {
			response.Error(w, http.StatusUnauthorized, "invalid or expired token", nil)
			return
		}

		// only access tokens may pass; reject anything not minted as token_login
		if claims.Typ != token.TokenLogin {
			response.Error(w, http.StatusUnauthorized, "invalid token type", nil)
			return
		}

		ctx := context.WithValue(r.Context(), claimsKey, claims)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (m *Middleware) RequireActive(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims, ok := ClaimsFromContext(r.Context())
		if !ok {
			response.Error(w, http.StatusUnauthorized, "unauthorized", nil)
			return
		}

		status, err := m.status.UserStatus(r.Context(), claims.ID)
		if err != nil {
			log.Printf("require active internal error: %v", err)
			response.Error(w, http.StatusInternalServerError, "internal server error", nil)
			return
		}

		if status != "active" {
			response.Error(w, http.StatusForbidden, "account not active", nil)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func (m *Middleware) RequireOwner(param string, resolve OwnerResolver) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims, ok := ClaimsFromContext(r.Context())
			if !ok {
				response.Error(w, http.StatusUnauthorized, "unauthorized", nil)
				return
			}

			ownerID, err := resolve(r.Context(), chi.URLParam(r, param))
			switch {
			case errors.Is(err, ErrResourceNotFound):
				response.Error(w, http.StatusNotFound, "not found", nil)
			case err != nil:
				log.Printf("require owner internal error: %v", err)
				response.Error(w, http.StatusInternalServerError, "internal server error", nil)
			case ownerID != claims.ID:
				response.Error(w, http.StatusForbidden, "forbidden", nil)
			default:
				next.ServeHTTP(w, r)
			}
		})
	}
}

func (m *Middleware) RequireMember(param string, resolver MemberResolver) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims, ok := ClaimsFromContext(r.Context())
			if !ok {
				response.Error(w, http.StatusUnauthorized, "unauthorized", nil)
				return
			}

			ms, err := resolver(r.Context(), chi.URLParam(r, param), claims.ID)
			switch {
			case errors.Is(err, ErrResourceNotFound):
				response.Error(w, http.StatusForbidden, "forbidden", nil)
			case err != nil:
				log.Printf("require member internal error: %v", err)
				response.Error(w, http.StatusInternalServerError, "internal server error", nil)
			default:
				ctx := context.WithValue(r.Context(), membershipKey, ms)
				next.ServeHTTP(w, r.WithContext(ctx))
			}
		})
	}
}

func (m *Middleware) RequirePermission(perm string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ms, ok := MembershipFromContext(r.Context())
			if !ok {
				log.Printf("require permission: membership missing in context (RequireMember not applied?)")
				response.Error(w, http.StatusInternalServerError, "internal server error", nil)
				return
			}

			for _, p := range ms.Permissions {
				if p == perm {
					next.ServeHTTP(w, r)
					return
				}
			}

			response.Error(w, http.StatusForbidden, "forbidden", nil)
		})
	}
}

func (m *Middleware) RequireRoomOpenForGuests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ms, ok := MembershipFromContext(r.Context())
		if !ok {
			log.Printf("require room open: membership missing in context (RequireMember not applied?)")
			response.Error(w, http.StatusInternalServerError, "internal server error", nil)
			return
		}

		if ms.Role == permission.RoleGuest && ms.WorkspaceStatus == permission.RoomPrepare {
			response.Error(w, http.StatusForbidden, "room is not open yet", nil)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func (m *Middleware) RequireRoomWritable(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ms, ok := MembershipFromContext(r.Context())
		if !ok {
			log.Printf("require room writable: membership missing in context (RequireMember not applied?)")
			response.Error(w, http.StatusInternalServerError, "internal server error", nil)
			return
		}

		if ms.WorkspaceStatus != permission.RoomArchive {
			next.ServeHTTP(w, r)
			return
		}

		if r.Method == http.MethodGet || r.Method == http.MethodHead {
			next.ServeHTTP(w, r)
			return
		}

		response.Error(w, http.StatusLocked, "room is archived", nil)
	})
}

func ClaimsFromContext(ctx context.Context) (*token.JwtClaims, bool) {
	claims, ok := ctx.Value(claimsKey).(*token.JwtClaims)
	return claims, ok
}

func MembershipFromContext(ctx context.Context) (*Membership, bool) {
	ms, ok := ctx.Value(membershipKey).(*Membership)
	return ms, ok
}

func (m *Middleware) RateLimit(cfg RateConfig) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			id := ""
			if cfg.Key != nil {
				id = cfg.Key(r)
			}

			key := cfg.Name + "|" + ClientIP(r) + "|" + id

			allowed, retryAfter := m.limiter.Allow(key, cfg.Limit, cfg.Window)
			if !allowed {
				secs := int(math.Ceil(retryAfter.Seconds()))
				if secs < 1 {
					secs = 1
				}
				w.Header().Set("Retry-After", strconv.Itoa(secs))
				response.Error(w, http.StatusTooManyRequests, "too many requests, please try again later", nil)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func KeyFromClaims(r *http.Request) string {
	if claims, ok := ClaimsFromContext(r.Context()); ok {
		return strings.ToLower(claims.Email)
	}
	return ""
}

func KeyFromJSONField(field string) KeyFunc {
	return func(r *http.Request) string {
		if r.Body == nil {
			return ""
		}
		buf, err := io.ReadAll(io.LimitReader(r.Body, MaxBodyBytesPeek))
		r.Body = io.NopCloser(bytes.NewReader(buf))
		if err != nil {
			return ""
		}
		var body map[string]any
		if err := json.Unmarshal(buf, &body); err != nil {
			return ""
		}
		if v, ok := body[field].(string); ok {
			return strings.ToLower(strings.TrimSpace(v))
		}
		return ""
	}
}

const MaxBodyBytesPeek = 1 << 20

// RealIP menghitung IP klien sekali per request dan menaruhnya di context.
// XFF hanya dihormati kalau peer (RemoteAddr) ada di daftar tepercaya, dan
// yang diambil adalah entri paling kanan yang bukan proxy tepercaya — entri
// kiri adalah yang paling mudah dipalsukan klien. Default daftar kosong =
// XFF tidak pernah dipercaya: salah-konfigurasi berujung IP proxy yang tidak
// berguna, bukan IP yang bisa dipalsukan.
func RealIP(trusted []netip.Prefix) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := resolveClientIP(r, trusted)
			ctx := context.WithValue(r.Context(), clientIPKey, ip)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func ClientIP(r *http.Request) string {
	if ip, ok := r.Context().Value(clientIPKey).(string); ok && ip != "" {
		return ip
	}
	return peerIP(r)
}

// resolveClientIP — aturan persis 9.5-a:
//  1. peer = host dari RemoteAddr (SplitHostPort gagal → RemoteAddr apa adanya).
//  2. peer tidak tepercaya → peer, XFF diabaikan total.
//  3. Kumpulkan hop XFF (nilai bisa muncul berkali-kali), dipecah koma, di-trim,
//     entri kosong dibuang; kalau kosong, X-Real-IP sebagai satu hop.
//  4. Telusuri hop dari kanan ke kiri, kembalikan yang pertama tidak tepercaya.
//  5. Semua hop tepercaya (atau tidak ada hop) → peer.
func resolveClientIP(r *http.Request, trusted []netip.Prefix) string {
	peer := peerIP(r)
	if !isTrustedProxy(peer, trusted) {
		return peer
	}

	var hops []string
	for _, v := range r.Header.Values("X-Forwarded-For") {
		for _, part := range strings.Split(v, ",") {
			if part = strings.TrimSpace(part); part != "" {
				hops = append(hops, part)
			}
		}
	}
	if len(hops) == 0 {
		if xr := strings.TrimSpace(r.Header.Get("X-Real-IP")); xr != "" {
			hops = append(hops, xr)
		}
	}

	for i := len(hops) - 1; i >= 0; i-- {
		if !isTrustedProxy(hops[i], trusted) {
			return hops[i]
		}
	}

	return peer
}

func isTrustedProxy(ip string, trusted []netip.Prefix) bool {
	addr, err := netip.ParseAddr(ip)
	if err != nil {
		return false
	}

	// IPv4-mapped-IPv6 tidak pernah cocok dengan prefix IPv4 tanpa Unmap.
	addr = addr.Unmap()

	for _, p := range trusted {
		if p.Contains(addr) {
			return true
		}
	}
	return false
}

func peerIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
