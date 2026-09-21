package protocol

import (
	pb "github.com/example/mmo-server/internal/protocolpb/gen"
	"google.golang.org/protobuf/types/known/emptypb"
)

// 以下为生成类型别名，业务仍从 internal/protocol 引用，便于与架构约定对齐。

type (
	StringKeyValue        = pb.StringKeyValue
	CodeOnly              = pb.CodeOnly
	EnterGameRequest      = pb.EnterGameRequest
	EnterGameResponse     = pb.EnterGameResponse
	MoveRequest           = pb.MoveRequest
	MoveBroadcast         = pb.MoveBroadcast
	PlayerInfo            = pb.PlayerInfo
	PlayerSelectResponse  = pb.PlayerSelectResponse
	PlayerCreateRequest   = pb.PlayerCreateRequest
	PlayerCreateResponse  = pb.PlayerCreateResponse
	PlayerDeleteRequest   = pb.PlayerDeleteRequest
	IssueTokenRequest     = pb.IssueTokenRequest
	IssueTokenResponse    = pb.IssueTokenResponse
	TokenLoginRequest     = pb.TokenLoginRequest
	TokenLoginResponse    = pb.TokenLoginResponse
	LogoutRequest         = pb.LogoutRequest
	LogoutResponse        = pb.LogoutResponse
	RefreshTokenRequest   = pb.RefreshTokenRequest
	RefreshTokenResponse  = pb.RefreshTokenResponse
	ChatSendRequest       = pb.ChatSendRequest
	ChatBroadcast         = pb.ChatBroadcast
	BagItem               = pb.BagItem
	BagListResponse       = pb.BagListResponse
	BagListRequest        = pb.BagListRequest
	BagAddRequest         = pb.BagAddRequest
	BagRemoveRequest      = pb.BagRemoveRequest
	BagMoveRequest        = pb.BagMoveRequest
	BagSplitRequest       = pb.BagSplitRequest
	GmReloadRequest       = pb.GmReloadRequest
	GmReloadResponse      = pb.GmReloadResponse
	GmAccountQueryRequest = pb.GmAccountQueryRequest
	GmAccountView         = pb.GmAccountView
	GmPlayerQueryRequest  = pb.GmPlayerQueryRequest
	GmPlayerRecord        = pb.GmPlayerRecord
	GmPlayerQueryResponse = pb.GmPlayerQueryResponse
	GmBagQueryRequest     = pb.GmBagQueryRequest
	GmGrantRequest        = pb.GmGrantRequest
	GmGrantResponse       = pb.GmGrantResponse
	GmKickRequest         = pb.GmKickRequest
	GmKickResponse        = pb.GmKickResponse
	GmDeductRequest       = pb.GmDeductRequest
	GmBanRequest          = pb.GmBanRequest
	GmBanResponse         = pb.GmBanResponse
	GmOnlineRequest       = pb.GmOnlineRequest
	GmOnlinePlayer        = pb.GmOnlinePlayer
	GmOnlineResponse      = pb.GmOnlineResponse
	GmNoticeRequest       = pb.GmNoticeRequest
	GmNoticeResponse      = pb.GmNoticeResponse
	GmNoticePush          = pb.GmNoticePush
	GmTimeSetRequest      = pb.GmTimeSetRequest
	GmTimeGetResponse     = pb.GmTimeGetResponse
)

// None 表示无请求体（如 select），与 google.protobuf.Empty 兼容。
type None = emptypb.Empty
