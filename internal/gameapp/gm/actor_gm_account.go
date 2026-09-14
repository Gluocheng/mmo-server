package gm

import (
	"strings"

	"github.com/cherry-game/cherry/net/parser/pomelo"
	"github.com/example/mmo-server/internal/code"
	"github.com/example/mmo-server/internal/persistence"
	"github.com/example/mmo-server/internal/protocol"
)

// actorGMAccount GM 账号查询域。
type actorGMAccount struct {
	pomelo.ActorBase
}

func (p *actorGMAccount) OnInit() {
	p.Remote().Register("query", p.queryCluster)
}

// queryCluster 按 uid 或 nickname 二选一查账号。
func (p *actorGMAccount) queryCluster(req *protocol.GmAccountQueryRequest) (*protocol.GmAccountView, int32) {
	if req == nil {
		return nil, code.GmBadRequest
	}
	uidSet := req.Uid > 0
	nick := strings.TrimSpace(req.Nickname)
	nickSet := nick != ""
	if !exactlyOne(uidSet, nickSet) {
		return nil, code.GmBadRequest
	}
	var (
		view  *protocol.GmAccountView
		found bool
		err   error
	)
	if uidSet {
		view, found, err = persistence.GetAccountByUID(req.Uid)
	} else {
		view, found, err = persistence.GetAccountByNickname(nick)
	}
	if err != nil {
		return nil, code.GmTargetNotFound
	}
	if !found {
		return nil, code.GmTargetNotFound
	}
	return view, code.OK
}
