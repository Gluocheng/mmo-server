package gmapp

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	clog "github.com/cherry-game/cherry/logger"
	cproto "github.com/cherry-game/cherry/net/proto"
	"github.com/example/mmo-server/internal/code"
	"github.com/example/mmo-server/internal/gtime"
	"github.com/example/mmo-server/internal/persistence"
	"github.com/example/mmo-server/internal/protocol"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

var errNATSDisconnected = errors.New("nats not connected")

var protoJSON = protojson.MarshalOptions{
	UseProtoNames:   false,
	EmitUnpopulated: true,
}

type errorRsp struct {
	Code    int32  `json:"code"`
	Message string `json:"message,omitempty"`
}

type healthRsp struct {
	Code          int32  `json:"code"`
	Message       string `json:"message"`
	NATSConnected bool   `json:"natsConnected"`
	RemoteSubject string `json:"remoteSubject"`
	TargetPath    string `json:"targetPath"`
}

type configReloadReq struct {
	TableName string `json:"tableName"`
}

type configReloadRsp struct {
	Code    int32  `json:"code"`
	Message string `json:"message,omitempty"`
	Version string `json:"version,omitempty"`
	Tables  int64  `json:"tables,omitempty"`
}

type grantReq struct {
	PlayerId int64 `json:"playerId"`
	ItemId   int32 `json:"itemId"`
	Count    int32 `json:"count"`
	BagType  int32 `json:"bagType"`
}

type kickReq struct {
	Uid      int64 `json:"uid"`
	PlayerId int64 `json:"playerId"`
}

type deductReq struct {
	PlayerId int64  `json:"playerId"`
	ItemId   *int32 `json:"itemId"`
	Slot     *int32 `json:"slot"`
	Count    int32  `json:"count"`
	BagType  int32  `json:"bagType"`
}

type banReq struct {
	Uid      int64  `json:"uid"`
	Nickname string `json:"nickname"`
	Reason   string `json:"reason"`
}

type noticeReq struct {
	SceneId int32  `json:"sceneId"`
	Text    string `json:"text"`
}

type timeSetReq struct {
	BiasSeconds int64 `json:"biasSeconds"`
}

func (a *App) registerRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/gm/health", a.handleHealth)
	mux.HandleFunc("/gm/auth/login", a.handleLogin)
	mux.HandleFunc("/gm/auth/logout", a.handleLogout)
	mux.HandleFunc("/gm/auth/me", a.requireAuth(a.handleMe))
	mux.HandleFunc("/gm/users", a.requireAdmin(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			a.handleUsersList(w, r)
			return
		}
		a.handleUsersCreate(w, r)
	}))
	mux.HandleFunc("/gm/users/disable", a.requireAdmin(a.handleUsersDisable))
	mux.HandleFunc("/gm/users/password", a.requireAdmin(a.handleUsersPassword))
	mux.HandleFunc("/gm/ops/logs", a.requireAuth(a.handleOpsLogs))
	mux.HandleFunc("/gm/config/reload", a.requireAuth(a.handleConfigReload))
	mux.HandleFunc("/gm/account", a.requireAuth(a.handleAccount))
	mux.HandleFunc("/gm/player", a.requireAuth(a.handlePlayer))
	mux.HandleFunc("/gm/bag", a.requireAuth(a.handleBagQuery))
	mux.HandleFunc("/gm/bag/grant", a.requireAuth(a.handleBagGrant))
	mux.HandleFunc("/gm/bag/deduct", a.requireAuth(a.handleBagDeduct))
	mux.HandleFunc("/gm/player/kick", a.requireAuth(a.handleKick))
	mux.HandleFunc("/gm/account/ban", a.requireAuth(a.handleAccountBan))
	mux.HandleFunc("/gm/account/unban", a.requireAuth(a.handleAccountUnban))
	mux.HandleFunc("/gm/world/online", a.requireAuth(a.handleWorldOnline))
	mux.HandleFunc("/gm/notice", a.requireAuth(a.handleNotice))
	mux.HandleFunc("/gm/time", a.requireAuth(a.handleTime))
	mux.Handle("/", spaHandler())
}

func (a *App) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, healthRsp{Code: -1, Message: "method not allowed, use GET"})
		return
	}
	writeJSON(w, http.StatusOK, healthRsp{
		Code:          0,
		Message:       "ok",
		NATSConnected: a.natsConn != nil && a.natsConn.IsConnected(),
		RemoteSubject: a.remoteSubject,
		TargetPath:    a.targetPath,
	})
}

func (a *App) handleConfigReload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, configReloadRsp{Code: -1, Message: "method not allowed, use POST"})
		return
	}
	var req configReloadReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, configReloadRsp{Code: -1, Message: "invalid request body: " + err.Error()})
		return
	}
	pbReq := &protocol.GmReloadRequest{TableName: req.TableName, Operator: operatorFrom(r)}
	rsp, err := a.callRemote("config", "reload", pbReq)
	if !a.writeRemoteErr(w, "reload", err) {
		return
	}
	if rsp.Code != 0 {
		writeJSON(w, http.StatusOK, configReloadRsp{Code: rsp.Code, Message: "reload failed"})
		return
	}
	var pbRsp protocol.GmReloadResponse
	unmarshalPayload(rsp, &pbRsp)
	writeJSON(w, http.StatusOK, configReloadRsp{
		Code:    0,
		Message: "ok",
		Version: strconv.FormatInt(pbRsp.Version, 10),
		Tables:  pbRsp.Tables,
	})
}

func (a *App) handleAccount(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, errorRsp{Code: -1, Message: "method not allowed, use GET"})
		return
	}
	uid, _ := parseInt64Query(r, "uid")
	nickname := r.URL.Query().Get("nickname")
	uidSet := uid > 0
	nickSet := nickname != ""
	if uidSet == nickSet {
		writeJSON(w, http.StatusBadRequest, errorRsp{Code: code.GmBadRequest, Message: "provide exactly one of uid or nickname"})
		return
	}
	rsp, err := a.callRemote("account", "query", &protocol.GmAccountQueryRequest{Uid: uid, Nickname: nickname})
	a.writeProtoResult(w, "account.query", err, rsp, &protocol.GmAccountView{})
}

func (a *App) handlePlayer(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, errorRsp{Code: -1, Message: "method not allowed, use GET"})
		return
	}
	playerID, _ := parseInt64Query(r, "playerId")
	uid, _ := parseInt64Query(r, "uid")
	name := r.URL.Query().Get("name")
	n := 0
	if playerID > 0 {
		n++
	}
	if name != "" {
		n++
	}
	if uid > 0 {
		n++
	}
	if n != 1 {
		writeJSON(w, http.StatusBadRequest, errorRsp{Code: code.GmBadRequest, Message: "provide exactly one of playerId, name or uid"})
		return
	}
	rsp, err := a.callRemote("player", "query", &protocol.GmPlayerQueryRequest{
		PlayerId: playerID,
		Name:     name,
		Uid:      uid,
	})
	a.writeProtoResult(w, "player.query", err, rsp, &protocol.GmPlayerQueryResponse{})
}

func (a *App) handleBagQuery(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, errorRsp{Code: -1, Message: "method not allowed, use GET"})
		return
	}
	playerID, _ := parseInt64Query(r, "playerId")
	bagType, _ := parseInt32Query(r, "bagType")
	if playerID < 1 || bagType < 1 {
		writeJSON(w, http.StatusBadRequest, errorRsp{Code: code.GmBadRequest, Message: "playerId and bagType required"})
		return
	}
	rsp, err := a.callRemote("bag", "query", &protocol.GmBagQueryRequest{PlayerId: playerID, BagType: bagType})
	a.writeProtoResult(w, "bag.query", err, rsp, &protocol.BagListResponse{})
}

func (a *App) handleBagGrant(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, errorRsp{Code: -1, Message: "method not allowed, use POST"})
		return
	}
	var req grantReq
	if err := decodeJSONBody(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorRsp{Code: code.GmBadRequest, Message: err.Error()})
		return
	}
	if req.PlayerId < 1 || req.ItemId < 1 {
		writeJSON(w, http.StatusBadRequest, errorRsp{Code: code.GmBadRequest, Message: "playerId and itemId required"})
		return
	}
	rsp, err := a.callRemote("bag", "grant", &protocol.GmGrantRequest{
		PlayerId: req.PlayerId,
		ItemId:   req.ItemId,
		Count:    req.Count,
		BagType:  req.BagType,
		Operator: operatorFrom(r),
	})
	a.writeProtoResult(w, "bag.grant", err, rsp, &protocol.GmGrantResponse{})
}

func (a *App) handleKick(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, errorRsp{Code: -1, Message: "method not allowed, use POST"})
		return
	}
	var req kickReq
	if err := decodeJSONBody(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorRsp{Code: code.GmBadRequest, Message: err.Error()})
		return
	}
	uidSet := req.Uid > 0
	pidSet := req.PlayerId > 0
	if uidSet == pidSet {
		writeJSON(w, http.StatusBadRequest, errorRsp{Code: code.GmBadRequest, Message: "provide exactly one of uid or playerId"})
		return
	}
	rsp, err := a.callRemote("player", "kick", &protocol.GmKickRequest{
		Uid:      req.Uid,
		PlayerId: req.PlayerId,
		Operator: operatorFrom(r),
	})
	a.writeProtoResult(w, "player.kick", err, rsp, &protocol.GmKickResponse{})
}

func (a *App) handleBagDeduct(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, errorRsp{Code: -1, Message: "method not allowed, use POST"})
		return
	}
	var req deductReq
	if err := decodeJSONBody(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorRsp{Code: code.GmBadRequest, Message: err.Error()})
		return
	}
	itemSet := req.ItemId != nil && *req.ItemId > 0
	slotSet := req.Slot != nil
	if req.PlayerId < 1 || itemSet == slotSet {
		writeJSON(w, http.StatusBadRequest, errorRsp{Code: code.GmBadRequest, Message: "playerId required; provide exactly one of itemId or slot"})
		return
	}
	pbReq := &protocol.GmDeductRequest{
		PlayerId: req.PlayerId,
		Count:    req.Count,
		BagType:  req.BagType,
		Operator: operatorFrom(r),
	}
	if itemSet {
		pbReq.ItemId = *req.ItemId
	} else {
		pbReq.Slot = *req.Slot
	}
	rsp, err := a.callRemote("bag", "deduct", pbReq)
	a.writeProtoResult(w, "bag.deduct", err, rsp, &protocol.GmGrantResponse{})
}

func (a *App) handleAccountBan(w http.ResponseWriter, r *http.Request) {
	a.handleAccountBanState(w, r, true)
}

func (a *App) handleAccountUnban(w http.ResponseWriter, r *http.Request) {
	a.handleAccountBanState(w, r, false)
}

func (a *App) handleAccountBanState(w http.ResponseWriter, r *http.Request, ban bool) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, errorRsp{Code: -1, Message: "method not allowed, use POST"})
		return
	}
	var req banReq
	if err := decodeJSONBody(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorRsp{Code: code.GmBadRequest, Message: err.Error()})
		return
	}
	uidSet := req.Uid > 0
	nickSet := strings.TrimSpace(req.Nickname) != ""
	if uidSet == nickSet {
		writeJSON(w, http.StatusBadRequest, errorRsp{Code: code.GmBadRequest, Message: "provide exactly one of uid or nickname"})
		return
	}
	fn := "unban"
	if ban {
		fn = "ban"
	}
	rsp, err := a.callRemote("account", fn, &protocol.GmBanRequest{
		Uid:      req.Uid,
		Nickname: req.Nickname,
		Reason:   req.Reason,
		Operator: operatorFrom(r),
	})
	a.writeProtoResult(w, "account."+fn, err, rsp, &protocol.GmBanResponse{})
}

func (a *App) handleWorldOnline(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, errorRsp{Code: -1, Message: "method not allowed, use GET"})
		return
	}
	sceneID, _ := parseInt32Query(r, "sceneId")
	rsp, err := a.callRemote("world", "online", &protocol.GmOnlineRequest{SceneId: sceneID})
	a.writeProtoResult(w, "world.online", err, rsp, &protocol.GmOnlineResponse{})
}

func (a *App) handleNotice(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, errorRsp{Code: -1, Message: "method not allowed, use POST"})
		return
	}
	var req noticeReq
	if err := decodeJSONBody(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorRsp{Code: code.GmBadRequest, Message: err.Error()})
		return
	}
	if strings.TrimSpace(req.Text) == "" {
		writeJSON(w, http.StatusBadRequest, errorRsp{Code: code.GmBadRequest, Message: "text required"})
		return
	}
	rsp, err := a.callRemote("world", "notice", &protocol.GmNoticeRequest{
		SceneId:  req.SceneId,
		Text:     strings.TrimSpace(req.Text),
		Operator: operatorFrom(r),
	})
	a.writeProtoResult(w, "world.notice", err, rsp, &protocol.GmNoticeResponse{})
}

func (a *App) handleTime(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		a.writeTimeJSON(w, false, false, false)
	case http.MethodPost:
		a.handleTimeSet(w, r)
	default:
		writeJSON(w, http.StatusMethodNotAllowed, errorRsp{Code: -1, Message: "method not allowed, use GET or POST"})
	}
}

func (a *App) handleTimeSet(w http.ResponseWriter, r *http.Request) {
	var req timeSetReq
	if err := decodeJSONBody(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorRsp{Code: code.GmBadRequest, Message: err.Error()})
		return
	}
	if req.BiasSeconds < 0 {
		writeJSON(w, http.StatusBadRequest, errorRsp{Code: code.GmBadRequest, Message: "biasSeconds must be >= 0"})
		return
	}
	if err := persistence.SaveTimeBias(r.Context(), req.BiasSeconds); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, errorRsp{Code: -1, Message: "save time bias: " + err.Error()})
		return
	}
	pbReq := &protocol.GmTimeSetRequest{BiasSeconds: req.BiasSeconds, Operator: operatorFrom(r)}
	gameOK := false
	if rsp, err := a.callRemote("time", "set", pbReq); err == nil && rsp.Code == 0 {
		gameOK = true
	} else if err != nil {
		logRemoteErr("time.set.game", err)
	}
	loginOK := false
	if rsp, err := a.callLogin("setGameTime", pbReq); err == nil && rsp.Code == 0 {
		loginOK = true
	} else if err != nil {
		logRemoteErr("time.set.login", err)
	}
	a.writeTimeJSON(w, true, gameOK, loginOK)
}

func (a *App) writeTimeJSON(w http.ResponseWriter, withFlags, gameUpdated, loginUpdated bool) {
	out := map[string]any{
		"code":        int32(0),
		"message":     "ok",
		"biasSeconds": gtime.BiasSeconds(),
		"unixNow":     gtime.UnixNow(),
		"realUnixNow": gtime.RealNow().Unix(),
	}
	if withFlags {
		out["gameUpdated"] = gameUpdated
		out["loginUpdated"] = loginUpdated
	}
	writeJSON(w, http.StatusOK, out)
}

func (a *App) writeRemoteErr(w http.ResponseWriter, op string, err error) bool {
	if err == nil {
		return true
	}
	logRemoteErr(op, err)
	if errors.Is(err, errNATSDisconnected) {
		writeJSON(w, http.StatusServiceUnavailable, errorRsp{Code: -1, Message: "nats not connected"})
		return false
	}
	writeJSON(w, http.StatusGatewayTimeout, errorRsp{Code: -1, Message: "game node timeout: " + err.Error()})
	return false
}

func (a *App) writeProtoResult(w http.ResponseWriter, op string, err error, rsp *cproto.Response, msg proto.Message) {
	if !a.writeRemoteErr(w, op, err) {
		return
	}
	httpStatus := http.StatusOK
	switch rsp.Code {
	case code.GmUnauthorized:
		httpStatus = http.StatusUnauthorized
	case code.GmBadRequest:
		httpStatus = http.StatusBadRequest
	case code.GmForbidden:
		httpStatus = http.StatusForbidden
	}
	if rsp.Code != 0 {
		writeJSON(w, httpStatus, errorRsp{Code: rsp.Code, Message: "failed"})
		return
	}
	unmarshalPayload(rsp, msg)
	writeMergedProtoJSON(w, httpStatus, rsp.Code, msg)
}

func unmarshalPayload(rsp *cproto.Response, msg proto.Message) {
	if rsp == nil || len(rsp.Data) == 0 {
		return
	}
	if err := proto.Unmarshal(rsp.Data, msg); err != nil {
		clog.Warnf("gm http unmarshal payload: %v", err)
	}
}

func writeMergedProtoJSON(w http.ResponseWriter, httpStatus int, bizCode int32, msg proto.Message) {
	out := map[string]any{"code": bizCode, "message": "ok"}
	if msg != nil {
		raw, err := protoJSON.Marshal(msg)
		if err == nil {
			var fields map[string]any
			if json.Unmarshal(raw, &fields) == nil {
				for k, v := range fields {
					if k == "code" {
						continue
					}
					out[k] = v
				}
			}
		}
	}
	writeJSON(w, httpStatus, out)
}

func decodeJSONBody(r *http.Request, v any) error {
	defer r.Body.Close()
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(v); err != nil {
		if errors.Is(err, io.EOF) {
			return errors.New("empty body")
		}
		return err
	}
	return nil
}

func parseInt64Query(r *http.Request, key string) (int64, bool) {
	s := r.URL.Query().Get(key)
	if s == "" {
		return 0, false
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, false
	}
	return n, true
}

func parseInt32Query(r *http.Request, key string) (int32, bool) {
	n, ok := parseInt64Query(r, key)
	if !ok {
		return 0, false
	}
	return int32(n), true
}

func writeJSON(w http.ResponseWriter, statusCode int, v interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(statusCode)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		clog.Warnf("gm http write json: %v", err)
	}
}
