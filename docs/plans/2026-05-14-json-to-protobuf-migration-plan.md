# MMO JSON 鍒?Protobuf 杩佺Щ璁″垝

## 鐩爣

鍦ㄤ笉褰卞搷褰撳墠鍙繍琛岄摼璺殑鍓嶆彁涓嬶紝灏嗛珮棰戞秷鎭粠 JSON 杩佺Щ鍒?Protobuf锛岄檷浣庡甫瀹戒笌搴忓垪鍖栧紑閿€锛涗腑浣庨绠＄悊鍗忚淇濈暀鍦ㄥ悗缁樁娈佃縼绉汇€?
## 鑼冨洿

- 绗竴闃舵锛歚scene` / `move` 楂樻秷鎭鐜囬摼璺?- 绗簩闃舵锛歚player` 涓氬姟閾捐矾锛堟煡瑙?鍒涜/杩涘満锛?- 绗笁闃舵锛歚auth` 閾捐矾锛坕ssue/login/refresh/logout锛?
## 杩佺Щ姝ラ

- [ ] 鍐荤粨鐜版湁鍗忚瀛楁锛岄伩鍏嶈縼绉昏繃绋嬩腑棰戠箒鏀瑰悕
- [ ] 鏂板缓 proto 鐩綍锛歚internal/protocolpb/proto/`
- [ ] 鎸夐鍩熷畾涔?proto 鏂囦欢锛?  - [ ] `common.proto`
  - [ ] `scene.proto`
  - [ ] `player.proto`
  - [ ] `auth.proto`
- [ ] 鐢熸垚 Go 浠ｇ爜鍒帮細`internal/protocolpb/gen/`
- [ ] 鏇挎崲涓氬姟浠ｇ爜涓殑鍗忚绫诲瀷锛堝厛浠?scene 寮€濮嬶級锛?  - [ ] `internal/gameapp/player/actor_player.go`
  - [ ] `internal/gatewayapp/actor/agent.go`
  - [ ] `internal/loginapp/actor/actor_session.go`
- [ ] 缁熶竴鍒囨崲搴忓垪鍖栧櫒涓?Protobuf锛堣妭鐐圭骇涓€鑷达級锛?  - [ ] `internal/gatewayapp/app.go`
  - [ ] `internal/loginapp/app.go`
  - [ ] `internal/gameapp/app.go`
- [ ] 鏇存柊 README 鍗忚绀轰緥涓庣敓鎴愬懡浠?
## 鍏煎涓庨闄╂帶鍒?
- [ ] 鍏抽敭璺敱鍙姞鐗堟湰鍚庣紑锛堜緥濡?`game.player.move.v2`锛夎繘琛岀伆搴?- [ ] 鍒犻櫎瀛楁鍓嶄娇鐢?`reserved`锛岀姝㈠鐢ㄥ巻鍙插瓧娈靛彿
- [ ] 閬垮厤璺ㄨ妭鐐瑰嚭鐜?JSON/Protobuf 娣峰簭鍒楀寲鐘舵€?
## 楠岃瘉娓呭崟

- [ ] `go test ./...`
- [ ] `go build -o bin/gateway.exe ./cmd/gateway`
- [ ] `go build -o bin/login.exe ./cmd/login`
- [ ] `go build -o bin/game.exe ./cmd/game`
- [ ] 鎵嬪伐鑱旇皟闂幆锛?  - [ ] `issueToken`
  - [ ] `login`
  - [ ] `select/create/enter`
  - [ ] `move` 骞挎挱

## 閲岀▼纰?
1. M1锛歚scene` 鍗忚杩佺Щ瀹屾垚骞堕€氳繃鍘嬫祴
2. M2锛歚player` 鍗忚杩佺Щ瀹屾垚骞剁ǔ瀹氳繍琛?3. M3锛歚auth` 鍗忚杩佺Щ瀹屾垚骞舵枃妗ｉ綈鍏?
