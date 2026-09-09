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
		ws       = flag.String("ws", "127.0.0.1:10100", "gateway websocket host:port")
		nickname = flag.String("nickname", "player1", "account nickname")
		password = flag.String("password", "123456", "account password")
		deviceID = flag.String("device", "pc-001", "device id")
		player   = flag.String("player", "", "player name, default to nickname")
		timeout  = flag.Duration("timeout", 3*time.Second, "request timeout")
		once     = flag.Bool("once", true, "run a single smoke request sequence and exit")
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
		Nickname: *nickname,
		Password: *password,
		DeviceId: *deviceID,
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
		DeviceId:    *deviceID,
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

	var playerID int64
	if len(selectRsp.List) > 0 {
		playerID = selectRsp.List[0].PlayerId
	} else {
		playerName := *player
		if playerName == "" {
			playerName = *nickname
		}
		createRspMsg, err := c.Request("game.player.create", &protocol.PlayerCreateRequest{Name: playerName})
		if err != nil {
			fmt.Fprintf(os.Stderr, "create player failed: %v\n", err)
			exitCode = 1
			return
		}
		createRsp := &protocol.PlayerCreateResponse{}
		if err := c.Serializer().Unmarshal(createRspMsg.Data, createRsp); err != nil {
			fmt.Fprintf(os.Stderr, "unmarshal PlayerCreateResponse failed: %v\n", err)
			exitCode = 1
			return
		}
		if createRsp.Player == nil || createRsp.Player.PlayerId < 1 {
			fmt.Fprintf(os.Stderr, "create player returned invalid player\n")
			exitCode = 1
			return
		}
		playerID = createRsp.Player.PlayerId
		fmt.Printf("create player OK playerId=%d name=%s\n", playerID, createRsp.Player.Name)
	}

	// 4) enter
	enterReq := &protocol.EnterGameRequest{PlayerId: playerID, SceneId: 1}
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

	// 5) bag add（自动路由：1001 生命药水 → 消耗品背包 bag_type=2）
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

	// 6) bag list（消耗品背包 2）
	listRspMsg, err := c.Request("game.bag.list", &protocol.BagListRequest{BagType: 2})
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

	// 6b) bag move + split smoke（消耗品背包 2 内）
	if len(bagRsp.Items) > 0 {
		fromSlot := bagRsp.Items[0].Slot
		moveReq := &protocol.BagMoveRequest{FromSlot: fromSlot, ToSlot: fromSlot + 1, BagType: 2}
		if _, err := c.Request("game.bag.move", moveReq); err != nil {
			fmt.Fprintf(os.Stderr, "bag move failed: %v\n", err)
			exitCode = 1
			return
		}
		fmt.Println("bag move OK")
		splitReq := &protocol.BagSplitRequest{FromSlot: fromSlot + 1, Count: 1, BagType: 2}
		if _, err := c.Request("game.bag.split", splitReq); err != nil {
			fmt.Fprintf(os.Stderr, "bag split failed: %v\n", err)
			exitCode = 1
			return
		}
		fmt.Println("bag split OK")
	}

	// 6c) GM 显式指定错误背包应被拒绝（2001 新手木剑 → 装备背包 4，指定到 2 应 40028）
	gmReq := &protocol.BagAddRequest{ItemId: 2001, Count: 1, BagType: 2}
	if _, err := c.Request("game.bag.add", gmReq); err != nil {
		fmt.Fprintf(os.Stderr, "bag add (wrong bag) failed as expected: %v\n", err)
	} else {
		fmt.Println("bag add (wrong bag) unexpectedly succeeded")
		exitCode = 1
		return
	}

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
