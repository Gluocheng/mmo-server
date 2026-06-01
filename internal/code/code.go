package code

// 业务错误码从 40000 起，与框架码区分；0 表示成功。
const (
	OK int32 = 0 // 成功

	LoginFail           int32 = 40001 // 登录/鉴权通用失败（参数无效、持久化异常、绑定失败等）
	NotLoggedIn         int32 = 40002 // 未登录或 Session 无有效 Uid（如进场前未完成 gate 登录）
	EnterSceneFail      int32 = 40003 // 进场/移动请求无效（如 move 请求体为空）
	InvalidServer       int32 = 40004 // 区服 ID 无效（authToken 时 ServerId < 1）
	LoginRPCFail        int32 = 40005 // 网关调用登录服失败（无 login 节点或 RPC 调用异常）
	PlayerDenyLogin     int32 = 40006 // 未登录访问需登录路由，被踢下线
	PlayerCreateFail    int32 = 40007 // 创角失败（名称为空、持久化错误或角色已存在）
	PlayerNotFound      int32 = 40008 // 角色不存在或与请求 PlayerId 不匹配
	PlayerNotEntered    int32 = 40009 // 未进场（Session 无 PlayerID，无法移动等 gameplay 操作）
	InvalidPassword     int32 = 40010 // 帐号密码错误（issueToken）
	AccessTokenInvalid  int32 = 40011 // 访问令牌无效或已过期
	RefreshTokenInvalid int32 = 40012 // 刷新令牌无效或已过期
	RefreshTokenReplay  int32 = 40013 // 刷新令牌重放（已使用过的 refresh token 再次提交）
	LoginRateLimited    int32 = 40014 // 登录失败次数过多，IP/昵称被临时封禁
	DeviceIDRequired    int32 = 40015 // 缺少设备 ID（issueToken/login 必填）
	DeviceLimitReached  int32 = 40016 // 同帐号在线设备数达上限（device_limit 策略）
	DeviceMismatch      int32 = 40017 // 设备 ID 与签发令牌时不一致
	BagItemInvalid      int32 = 40018 // 物品 ID 或数量非法
	BagItemNotEnough    int32 = 40019 // 背包物品数量不足
	BagLoadFail         int32 = 40020 // 背包加载失败
	BagSlotInvalid      int32 = 40021 // 槽位非法或源槽为空
	BagFull             int32 = 40022 // 背包槽位已满
	ItemNotFound        int32 = 40023 // 道具 id 不在配置表
	ConfigReloadDenied  int32 = 40024 // 未开启配置 reload
	ConfigReloadFail    int32 = 40025 // 配置 reload 失败
)

func IsFail(c int32) bool {
	return c != OK
}
