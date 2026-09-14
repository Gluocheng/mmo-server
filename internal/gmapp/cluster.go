package gmapp

import (
	"fmt"
	"time"

	clog "github.com/cherry-game/cherry/logger"
	cproto "github.com/cherry-game/cherry/net/proto"
	"google.golang.org/protobuf/proto"
)

func (a *App) pathsFor(domain string) (source, target string) {
	return fmt.Sprintf("%s.gm.%s", gmNodeID, domain), fmt.Sprintf("%s.gm.%s", a.gameNodeID, domain)
}

func (a *App) callRemote(domain, funcName string, req proto.Message) (*cproto.Response, error) {
	if a.natsConn == nil {
		return nil, errNATSDisconnected
	}
	argBytes, err := proto.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal req: %w", err)
	}
	source, target := a.pathsFor(domain)
	clusterPacket := &cproto.ClusterPacket{
		SourcePath: source,
		TargetPath: target,
		FuncName:   funcName,
		ArgBytes:   argBytes,
	}
	cpBytes, err := proto.Marshal(clusterPacket)
	if err != nil {
		return nil, fmt.Errorf("marshal cluster: %w", err)
	}
	msg, err := a.natsConn.Request(a.remoteSubject, cpBytes, 5*time.Second)
	if err != nil {
		return nil, fmt.Errorf("nats request: %w", err)
	}
	var rsp cproto.Response
	if err := proto.Unmarshal(msg.Data, &rsp); err != nil {
		return nil, fmt.Errorf("unmarshal rsp: %w", err)
	}
	return &rsp, nil
}

func logRemoteErr(op string, err error) {
	clog.Warnf("gm http %s: %v", op, err)
}
