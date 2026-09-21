package gm

import (
	"strconv"
	"strings"

	"github.com/cherry-game/cherry/net/parser/pomelo"
	"github.com/example/mmo-server/internal/code"
	"github.com/example/mmo-server/internal/gameapp/world"
	"github.com/example/mmo-server/internal/persistence"
	"github.com/example/mmo-server/internal/protocol"
)

// actorGMWorld GM 场景在线列表与公告。
type actorGMWorld struct {
	pomelo.ActorBase
}

func (p *actorGMWorld) OnInit() {
	p.Remote().Register("online", p.onlineCluster)
	p.Remote().Register("notice", p.noticeCluster)
}

// onlineCluster 导出当前进场玩家；sceneId=0 为全部场景。
func (p *actorGMWorld) onlineCluster(req *protocol.GmOnlineRequest) (*protocol.GmOnlineResponse, int32) {
	var sceneID int32
	if req != nil {
		sceneID = req.SceneId
	}
	list := world.ListOnline(sceneID)
	out := make([]*protocol.GmOnlinePlayer, 0, len(list))
	for _, row := range list {
		out = append(out, &protocol.GmOnlinePlayer{Uid: row.UID, SceneId: row.SceneID})
	}
	return &protocol.GmOnlineResponse{List: out}, code.OK
}

// noticeCluster 向全服或指定场景推送公告。
func (p *actorGMWorld) noticeCluster(req *protocol.GmNoticeRequest) (*protocol.GmNoticeResponse, int32) {
	operator := ""
	text := ""
	var sceneID int32
	if req != nil {
		operator = req.Operator
		text = strings.TrimSpace(req.Text)
		sceneID = req.SceneId
	}
	if text == "" {
		persistence.WriteGMOpLog(operator, persistence.GMActionNotice, 0, 0, "", code.GmBadRequest)
		return nil, code.GmBadRequest
	}
	n := world.BroadcastNotice(p, sceneID, &protocol.GmNoticePush{Text: text, SceneId: sceneID})
	detail := text
	if sceneID > 0 {
		detail = detail + " sceneId=" + strconv.FormatInt(int64(sceneID), 10)
	}
	persistence.WriteGMOpLog(operator, persistence.GMActionNotice, 0, 0, detail, code.OK)
	return &protocol.GmNoticeResponse{Pushed: n}, code.OK
}
