package combat

import (
	"github.com/cherry-game/cherry/net/parser/pomelo"
	cproto "github.com/cherry-game/cherry/net/proto"
	"github.com/example/mmo-server/internal/code"
	"github.com/example/mmo-server/internal/protocol"
	"github.com/example/mmo-server/internal/sessionkey"
	"google.golang.org/protobuf/types/known/emptypb"
)

type actorCombat struct {
	pomelo.ActorBase
}

func (p *actorCombat) OnInit() {
	p.Local().Register("cast", p.cast)
}

func (p *actorCombat) cast(session *cproto.Session, req *protocol.CombatCastRequest) {
	if !session.Contains(sessionkey.PlayerID) {
		p.ResponseCode(session, code.PlayerNotEntered)
		return
	}
	if req == nil || req.SkillId < 1 {
		p.ResponseCode(session, code.CombatSkillInvalid)
		return
	}
	if c := Cast(session.Uid, req.SkillId, req.TargetUid); c != code.OK {
		p.ResponseCode(session, c)
		return
	}
	p.Response(session, &emptypb.Empty{})
}
