package code

const (
	OK                int32 = 0
	LoginFail         int32 = 40001
	NotLoggedIn       int32 = 40002
	EnterSceneFail    int32 = 40003
	InvalidServer     int32 = 40004
	LoginRPCFail      int32 = 40005
	PlayerDenyLogin   int32 = 40006
	PlayerCreateFail  int32 = 40007
	PlayerNotFound    int32 = 40008
	PlayerNotEntered  int32 = 40009
	InvalidPassword   int32 = 40010
	AccessTokenInvalid int32 = 40011
	RefreshTokenInvalid int32 = 40012
	RefreshTokenReplay int32 = 40013
	LoginRateLimited   int32 = 40014
)

func IsFail(c int32) bool {
	return c != OK
}
