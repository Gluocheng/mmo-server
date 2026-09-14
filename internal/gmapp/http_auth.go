package gmapp

import (
	"errors"
	"net/http"

	"github.com/example/mmo-server/internal/code"
	"github.com/example/mmo-server/internal/persistence"
)

type loginReq struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type meRsp struct {
	Code        int32  `json:"code"`
	Message     string `json:"message,omitempty"`
	Username    string `json:"username,omitempty"`
	Role        string `json:"role,omitempty"`
	DisplayName string `json:"displayName,omitempty"`
}

type createUserReq struct {
	Username    string `json:"username"`
	Password    string `json:"password"`
	DisplayName string `json:"displayName"`
	Role        string `json:"role"`
}

type disableUserReq struct {
	ID       int64 `json:"id"`
	Disabled bool  `json:"disabled"`
}

type passwordUserReq struct {
	ID       int64  `json:"id"`
	Password string `json:"password"`
}

func (a *App) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, errorRsp{Code: -1, Message: "method not allowed, use POST"})
		return
	}
	var req loginReq
	if err := decodeJSONBody(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorRsp{Code: code.GmBadRequest, Message: err.Error()})
		return
	}
	if persistence.IsGMLoginBlocked(req.Username) {
		writeJSON(w, http.StatusTooManyRequests, errorRsp{Code: code.LoginRateLimited, Message: "too many login failures"})
		return
	}
	u, err := persistence.AuthenticateGMUser(req.Username, req.Password)
	if err != nil {
		if errors.Is(err, persistence.ErrInvalidPassword) {
			persistence.RecordGMLoginFailure(req.Username)
		}
		httpStatus := http.StatusUnauthorized
		c := persistence.GMAuthFailCode(err)
		if c == code.GmForbidden {
			httpStatus = http.StatusForbidden
		}
		writeJSON(w, httpStatus, errorRsp{Code: c, Message: "unauthorized"})
		return
	}
	persistence.ClearGMLoginFailure(req.Username)
	tok, err := persistence.NewGMSessionToken()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorRsp{Code: code.LoginFail, Message: "session"})
		return
	}
	if err := persistence.PutGMSession(tok, persistence.GMSession{
		UserID:      u.ID,
		Username:    u.Username,
		Role:        u.Role,
		DisplayName: u.DisplayName,
	}, persistence.GMSessionTTL); err != nil {
		writeJSON(w, http.StatusInternalServerError, errorRsp{Code: code.LoginFail, Message: "session"})
		return
	}
	setSessionCookie(w, tok)
	writeJSON(w, http.StatusOK, meRsp{
		Code:        0,
		Message:     "ok",
		Username:    u.Username,
		Role:        u.Role,
		DisplayName: u.DisplayName,
	})
}

func (a *App) handleLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, errorRsp{Code: -1, Message: "method not allowed, use POST"})
		return
	}
	persistence.DeleteGMSession(sessionTokenFrom(r))
	clearSessionCookie(w)
	writeJSON(w, http.StatusOK, errorRsp{Code: 0, Message: "ok"})
}

func (a *App) handleMe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, errorRsp{Code: -1, Message: "method not allowed, use GET"})
		return
	}
	id, _ := identityFrom(r)
	writeJSON(w, http.StatusOK, meRsp{
		Code:        0,
		Message:     "ok",
		Username:    id.Username,
		Role:        id.Role,
		DisplayName: id.DisplayName,
	})
}

func (a *App) handleUsersList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, errorRsp{Code: -1, Message: "method not allowed, use GET"})
		return
	}
	list, err := persistence.ListGMUsers()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorRsp{Code: code.LoginFail, Message: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"code": 0, "message": "ok", "list": list})
}

func (a *App) handleUsersCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, errorRsp{Code: -1, Message: "method not allowed, use POST"})
		return
	}
	var req createUserReq
	if err := decodeJSONBody(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorRsp{Code: code.GmBadRequest, Message: err.Error()})
		return
	}
	u, err := persistence.CreateGMUser(req.Username, req.Password, req.DisplayName, req.Role)
	if err != nil {
		writeGMUserErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"code": 0, "message": "ok",
		"id": u.ID, "username": u.Username, "displayName": u.DisplayName,
		"role": u.Role, "disabled": u.Disabled, "createdAtUnix": u.CreatedAtUnix,
	})
}

func (a *App) handleUsersDisable(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, errorRsp{Code: -1, Message: "method not allowed, use POST"})
		return
	}
	var req disableUserReq
	if err := decodeJSONBody(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorRsp{Code: code.GmBadRequest, Message: err.Error()})
		return
	}
	if req.ID < 1 {
		writeJSON(w, http.StatusBadRequest, errorRsp{Code: code.GmBadRequest, Message: "id required"})
		return
	}
	if err := persistence.SetGMUserDisabled(req.ID, req.Disabled); err != nil {
		writeGMUserErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, errorRsp{Code: 0, Message: "ok"})
}

func (a *App) handleUsersPassword(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, errorRsp{Code: -1, Message: "method not allowed, use POST"})
		return
	}
	var req passwordUserReq
	if err := decodeJSONBody(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorRsp{Code: code.GmBadRequest, Message: err.Error()})
		return
	}
	if req.ID < 1 {
		writeJSON(w, http.StatusBadRequest, errorRsp{Code: code.GmBadRequest, Message: "id required"})
		return
	}
	if err := persistence.SetGMUserPassword(req.ID, req.Password); err != nil {
		writeGMUserErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, errorRsp{Code: 0, Message: "ok"})
}

func (a *App) handleOpsLogs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, errorRsp{Code: -1, Message: "method not allowed, use GET"})
		return
	}
	from, _ := parseInt64Query(r, "from")
	to, _ := parseInt64Query(r, "to")
	page, _ := parseInt64Query(r, "page")
	pageSize, _ := parseInt64Query(r, "pageSize")
	list, total, p, ps, err := persistence.ListGMOpLogs(persistence.GMOpLogFilter{
		Operator: r.URL.Query().Get("operator"),
		Action:   r.URL.Query().Get("action"),
		FromUnix: from,
		ToUnix:   to,
		Page:     int(page),
		PageSize: int(pageSize),
	})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorRsp{Code: code.LoginFail, Message: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"code": 0, "message": "ok",
		"list": list, "total": total, "page": p, "pageSize": ps,
	})
}

func writeGMUserErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, persistence.ErrGMUserExists):
		writeJSON(w, http.StatusBadRequest, errorRsp{Code: code.GmBadRequest, Message: "user exists"})
	case errors.Is(err, persistence.ErrGMWeakPassword), errors.Is(err, persistence.ErrGMInvalidRole):
		writeJSON(w, http.StatusBadRequest, errorRsp{Code: code.GmBadRequest, Message: err.Error()})
	case errors.Is(err, persistence.ErrGMUserNotFound):
		writeJSON(w, http.StatusBadRequest, errorRsp{Code: code.GmTargetNotFound, Message: "user not found"})
	case errors.Is(err, persistence.ErrGMLastAdmin):
		writeJSON(w, http.StatusBadRequest, errorRsp{Code: code.GmBadRequest, Message: "cannot disable last admin"})
	default:
		writeJSON(w, http.StatusInternalServerError, errorRsp{Code: code.LoginFail, Message: err.Error()})
	}
}
