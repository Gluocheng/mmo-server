package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	pomeloClient "github.com/cherry-game/cherry/net/parser/pomelo/client"
	"github.com/example/mmo-server/internal/protocol"
	"google.golang.org/protobuf/types/known/emptypb"
)

func main() {
	var (
		ws      = flag.String("ws", "127.0.0.1:10100", "gateway websocket host:port")
		timeout = flag.Duration("timeout", 3*time.Second, "request timeout")
		once    = flag.Bool("once", true, "run a single smoke request sequence and exit")
	)
	flag.Parse()

	c := pomeloClient.New(
		pomeloClient.WithRequestTimeout(*timeout),
	)

	exitCode := 0
	if err := c.ConnectToWS(*ws, ""); err != nil {
		fmt.Fprintf(os.Stderr, "connect ws failed: %v\n", err)
		os.Exit(1)
	}
	defer func() {
		c.Disconnect()
		os.Exit(exitCode)
	}()

	// 1) issueToken
	issueReq := &protocol.IssueTokenRequest{
		Nickname: "player1",
		Password: "123456",
		DeviceId: "pc-001",
	}
	issueRspMsg, err := c.Request("gate.user.issueToken", issueReq)
	if err != nil {
		fmt.Fprintf(os.Stderr, "issueToken failed: %v\n", err)
		exitCode = 1
		return
	}
	issueRsp := &protocol.IssueTokenResponse{}
	if err := c.Serializer().Unmarshal(issueRspMsg.Data, issueRsp); err != nil {
		fmt.Fprintf(os.Stderr, "unmarshal IssueTokenResponse failed: %v\n", err)
		exitCode = 1
		return
	}
	if issueRsp.AccessToken == "" {
		fmt.Fprintf(os.Stderr, "issueToken returned empty accessToken\n")
		exitCode = 1
		return
	}
	fmt.Printf("issueToken OK uid=%d\n", issueRsp.Uid)

	// 2) login
	loginReq := &protocol.TokenLoginRequest{
		AccessToken: issueRsp.AccessToken,
		ServerId:    10001,
		DeviceId:    "pc-001",
	}
	loginRspMsg, err := c.Request("gate.user.login", loginReq)
	if err != nil {
		fmt.Fprintf(os.Stderr, "login failed: %v\n", err)
		exitCode = 1
		return
	}
	loginRsp := &protocol.TokenLoginResponse{}
	if err := c.Serializer().Unmarshal(loginRspMsg.Data, loginRsp); err != nil {
		fmt.Fprintf(os.Stderr, "unmarshal TokenLoginResponse failed: %v\n", err)
		exitCode = 1
		return
	}
	if loginRsp.Uid < 1 {
		fmt.Fprintf(os.Stderr, "login returned invalid uid=%d\n", loginRsp.Uid)
		exitCode = 1
		return
	}
	fmt.Printf("login OK uid=%d\n", loginRsp.Uid)

	// 3) select
	selectRspMsg, err := c.Request("game.player.select", &emptypb.Empty{})
	if err != nil {
		fmt.Fprintf(os.Stderr, "select failed: %v\n", err)
		exitCode = 1
		return
	}
	selectRsp := &protocol.PlayerSelectResponse{}
	if err := c.Serializer().Unmarshal(selectRspMsg.Data, selectRsp); err != nil {
		fmt.Fprintf(os.Stderr, "unmarshal PlayerSelectResponse failed: %v\n", err)
		exitCode = 1
		return
	}
	fmt.Printf("select OK players=%d\n", len(selectRsp.List))

	// 4) enter
	enterReq := &protocol.EnterGameRequest{SceneId: 1}
	enterRspMsg, err := c.Request("game.player.enter", enterReq)
	if err != nil {
		fmt.Fprintf(os.Stderr, "enter failed: %v\n", err)
		exitCode = 1
		return
	}
	enterRsp := &protocol.EnterGameResponse{}
	if err := c.Serializer().Unmarshal(enterRspMsg.Data, enterRsp); err != nil {
		fmt.Fprintf(os.Stderr, "unmarshal EnterGameResponse failed: %v\n", err)
		exitCode = 1
		return
	}
	fmt.Printf("enter OK sceneId=%d online=%d\n", enterRsp.SceneId, len(enterRsp.Players))

	// 5) bag add
	addReq := &protocol.BagAddRequest{ItemId: 1001, Count: 2}
	addRspMsg, err := c.Request("game.bag.add", addReq)
	if err != nil {
		fmt.Fprintf(os.Stderr, "bag add failed: %v\n", err)
		exitCode = 1
		return
	}
	bagRsp := &protocol.BagListResponse{}
	if err := c.Serializer().Unmarshal(addRspMsg.Data, bagRsp); err != nil {
		fmt.Fprintf(os.Stderr, "unmarshal BagListResponse failed: %v\n", err)
		exitCode = 1
		return
	}
	fmt.Printf("bag add OK items=%d\n", len(bagRsp.Items))

	// 6) bag list
	listRspMsg, err := c.Request("game.bag.list", &emptypb.Empty{})
	if err != nil {
		fmt.Fprintf(os.Stderr, "bag list failed: %v\n", err)
		exitCode = 1
		return
	}
	bagRsp = &protocol.BagListResponse{}
	if err := c.Serializer().Unmarshal(listRspMsg.Data, bagRsp); err != nil {
		fmt.Fprintf(os.Stderr, "unmarshal BagListResponse failed: %v\n", err)
		exitCode = 1
		return
	}
	fmt.Printf("bag list OK items=%d\n", len(bagRsp.Items))

	// 7) move (no need to wait push)
	_, err = c.Request("game.player.move", &protocol.MoveRequest{X: 1, Y: 2, Z: 0})
	if err != nil {
		fmt.Fprintf(os.Stderr, "move failed: %v\n", err)
		exitCode = 1
		return
	}
	fmt.Println("move OK")

	if *once {
		return
	}

	select {}
}
