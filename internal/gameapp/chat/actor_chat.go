package chat

import (
	"strings"

	clog "github.com/cherry-game/cherry/logger"
	"github.com/cherry-game/cherry/net/parser/pomelo"
	cproto "github.com/cherry-game/cherry/net/proto"
	"github.com/example/mmo-server/internal/code"
	"github.com/example/mmo-server/internal/gameapp/broadcast"
	"github.com/example/mmo-server/internal/gameapp/world"
	"github.com/example/mmo-server/internal/persistence"
	"github.com/example/mmo-server/internal/protocol"
	"github.com/example/mmo-server/internal/sessionkey"
	"google.golang.org/protobuf/types/known/emptypb"
)

type actorChat struct {
	pomelo.ActorBase
}

func (p *actorChat) OnInit() {
	p.Local().Register("send", p.send)
}

func (p *actorChat) send(session *cproto.Session, req *protocol.ChatSendRequest) {
	if !session.Contains(sessionkey.PlayerID) {
		p.ResponseCode(session, code.PlayerNotEntered)
		return
	}
	if req == nil || strings.TrimSpace(req.Text) == "" {
		p.ResponseCode(session, code.LoginFail)
		return
	}
	muted, err := persistence.IsAccountMuted(session.Uid)
	if err != nil {
		clog.Warnf("chat mute check uid=%d err=%v", session.Uid, err)
		p.ResponseCode(session, code.LoginFail)
		return
	}
	if muted {
		p.ResponseCode(session, code.ChatMuted)
		return
	}

	text := strings.TrimSpace(req.Text)
	sceneID := world.DefaultSceneID
	if sid, ok := world.SceneID(session.Uid); ok {
		sceneID = sid
	}
	// 先占广播名额再回成功。队列满了这次不算发出去。
	if err := broadcast.EnqueueChat(session.Uid, sceneID, text); err != nil {
		p.ResponseCode(session, code.ChatBusy)
		return
	}
	clog.Debugf("chat send uid=%d len=%d", session.Uid, len(text))
	p.Response(session, &emptypb.Empty{})
}
