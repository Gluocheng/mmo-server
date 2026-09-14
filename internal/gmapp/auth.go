package gmapp

import (
	"context"
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/example/mmo-server/internal/code"
	"github.com/example/mmo-server/internal/persistence"
)

const (
	gmSessionCookie = "gm_session"
	identCtxKey     = ctxKey("gm-ident")
)

type ctxKey string

// Identity 当前请求的运营身份。
type Identity struct {
	UserID      int64
	Username    string
	Role        string
	DisplayName string
}

func (id Identity) isAdmin() bool {
	return id.Role == persistence.GMRoleAdmin
}

func withIdentity(r *http.Request, id Identity) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), identCtxKey, id))
}

func identityFrom(r *http.Request) (Identity, bool) {
	v, ok := r.Context().Value(identCtxKey).(Identity)
	return v, ok
}

func sessionTokenFrom(r *http.Request) string {
	if c, err := r.Cookie(gmSessionCookie); err == nil {
		return strings.TrimSpace(c.Value)
	}
	return ""
}

func (a *App) authorizeRequest(r *http.Request) (Identity, bool) {
	if tok := sessionTokenFrom(r); tok != "" {
		sess, err := persistence.GetGMSession(tok)
		if err == nil && sess != nil && sess.Username != "" {
			return Identity{
				UserID:      sess.UserID,
				Username:    sess.Username,
				Role:        sess.Role,
				DisplayName: sess.DisplayName,
			}, true
		}
	}
	if a.token != "" && a.tokenOK(r) {
		return Identity{
			Username:    persistence.GMSystemOperator,
			Role:        persistence.GMRoleOperator,
			DisplayName: persistence.GMSystemOperator,
		}, true
	}
	return Identity{}, false
}

func (a *App) tokenOK(r *http.Request) bool {
	got := strings.TrimSpace(r.Header.Get("X-GM-Token"))
	if got == "" {
		auth := strings.TrimSpace(r.Header.Get("Authorization"))
		const prefix = "bearer "
		if len(auth) >= len(prefix) && strings.EqualFold(auth[:len(prefix)], prefix) {
			got = strings.TrimSpace(auth[len(prefix):])
		}
	}
	if a.token == "" || len(got) != len(a.token) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(got), []byte(a.token)) == 1
}

func (a *App) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := a.authorizeRequest(r)
		if !ok {
			writeJSON(w, http.StatusUnauthorized, errorRsp{
				Code:    code.GmUnauthorized,
				Message: "unauthorized",
			})
			return
		}
		next(w, withIdentity(r, id))
	}
}

func (a *App) requireAdmin(next http.HandlerFunc) http.HandlerFunc {
	return a.requireAuth(func(w http.ResponseWriter, r *http.Request) {
		id, ok := identityFrom(r)
		if !ok || !id.isAdmin() {
			writeJSON(w, http.StatusForbidden, errorRsp{
				Code:    code.GmForbidden,
				Message: "forbidden",
			})
			return
		}
		next(w, r)
	})
}

func operatorFrom(r *http.Request) string {
	if id, ok := identityFrom(r); ok && id.Username != "" {
		return id.Username
	}
	return persistence.GMSystemOperator
}

func setSessionCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     gmSessionCookie,
		Value:    token,
		Path:     "/",
		MaxAge:   int(persistence.GMSessionTTL.Seconds()),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

func clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     gmSessionCookie,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}
