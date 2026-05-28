package chat

import (
	"strings"

	clog "github.com/cherry-game/cherry/logger"
	"github.com/cherry-game/cherry/net/parser/pomelo"
	cproto "github.com/cherry-game/cherry/net/proto"
	"github.com/example/mmo-server/internal/code"
	"github.com/example/mmo-server/internal/gameapp/world"
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

	b := &protocol.ChatBroadcast{
		Uid:  session.Uid,
		Text: strings.TrimSpace(req.Text),
	}
	sceneID := world.DefaultSceneID
	if sid, ok := world.SceneID(session.Uid); ok {
		sceneID = sid
	}
	world.BroadcastChat(p, session.Uid, sceneID, b)

	clog.Debugf("chat send uid=%d len=%d", session.Uid, len(b.Text))
	p.Response(session, &emptypb.Empty{})
}

