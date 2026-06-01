package gmapp

import (
	"encoding/json"
	"net/http"
	"time"

	clog "github.com/cherry-game/cherry/logger"
	cproto "github.com/cherry-game/cherry/net/proto"
	"github.com/example/mmo-server/internal/protocol"
	"google.golang.org/protobuf/proto"
)

// configReloadReq HTTP 请求体：gm 配置热更。
type configReloadReq struct {
	TableName string `json:"tableName"` // 为空时重载全部
}

// configReloadRsp HTTP 响应体。
type configReloadRsp struct {
	Code    int32  `json:"code"`
	Message string `json:"message,omitempty"`
	Version string `json:"version,omitempty"`
	Tables  int64  `json:"tables,omitempty"`
}

// registerRoutes 注册 HTTP 路由。
func (a *App) registerRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/gm/config/reload", a.handleConfigReload)
}

// handleConfigReload 接收配置热更请求，通过 NATS 转发到 game 节点 actor。
func (a *App) handleConfigReload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, configReloadRsp{
			Code:    -1,
			Message: "method not allowed, use POST",
		})
		return
	}

	var req configReloadReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, configReloadRsp{
			Code:    -1,
			Message: "invalid request body: " + err.Error(),
		})
		return
	}

	// 构造 protobuf 请求：复用 RefreshTokenRequest.RefreshToken 承载表名
	pbReq := &protocol.RefreshTokenRequest{
		RefreshToken: req.TableName,
	}
	argBytes, err := proto.Marshal(pbReq)
	if err != nil {
		clog.Warnf("gm http marshal reload req: %v", err)
		writeJSON(w, http.StatusInternalServerError, configReloadRsp{
			Code:    -1,
			Message: "marshal error",
		})
		return
	}

	// 构造跨节点 ClusterPacket 发送到 game 节点 gm config actor
	clusterPacket := &cproto.ClusterPacket{
		SourcePath: a.sourcePath,
		TargetPath: a.targetPath,
		FuncName:   "reload",
		ArgBytes:   argBytes,
	}
	cpBytes, err := proto.Marshal(clusterPacket)
	if err != nil {
		clog.Warnf("gm http marshal cluster packet: %v", err)
		writeJSON(w, http.StatusInternalServerError, configReloadRsp{
			Code:    -1,
			Message: "marshal cluster packet error",
		})
		return
	}

	// 通过 NATS 发送到 game 节点，5 秒超时
	msg, err := a.natsConn.Request(a.remoteSubject, cpBytes, 5*time.Second)
	if err != nil {
		clog.Warnf("gm http nats request: %v", err)
		writeJSON(w, http.StatusGatewayTimeout, configReloadRsp{
			Code:    -1,
			Message: "game node timeout: " + err.Error(),
		})
		return
	}

	// 解析 game 节点返回的 Response{code,data}
	var rsp cproto.Response
	if err := proto.Unmarshal(msg.Data, &rsp); err != nil {
		clog.Warnf("gm http unmarshal response: %v", err)
		writeJSON(w, http.StatusInternalServerError, configReloadRsp{
			Code:    -1,
			Message: "unmarshal response error",
		})
		return
	}

	if rsp.Code != 0 {
		writeJSON(w, http.StatusOK, configReloadRsp{
			Code:    rsp.Code,
			Message: "reload failed",
		})
		return
	}

	// 解析 ReflexTokenResponse 获取结果细节
	var pbRsp protocol.RefreshTokenResponse
	if len(rsp.Data) > 0 {
		if err := proto.Unmarshal(rsp.Data, &pbRsp); err != nil {
			clog.Warnf("gm http unmarshal reload rsp: %v", err)
		}
	}

	writeJSON(w, http.StatusOK, configReloadRsp{
		Code:    0,
		Message: "ok",
		Version: pbRsp.AccessToken,
		Tables:  pbRsp.RefreshExpireAt,
	})
}

// writeJSON 写入 JSON 响应。
func writeJSON(w http.ResponseWriter, statusCode int, v interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(v)
}
