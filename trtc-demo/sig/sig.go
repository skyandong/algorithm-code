// Package sig 封装腾讯云 TRTC 进房凭证(UserSig + PrivateMapKey)的生成。
//
// 背景：TRTC(实时音视频)要求客户端进房前必须持有凭证。凭证由服务端用
// SDKSecretKey 计算，客户端通过业务 API 获取，绝不把密钥下发到客户端。
//
// 凭证分两层：
//   - UserSig      身份凭证，标识某个 userID 有权使用 TRTC 服务。
//   - PrivateMapKey 进房权限凭证，额外限定该 userID 能进哪个房间、能上行/下行音视频。
//     新版 SDK 进房时两者都要带上。
package sig

import (
	"github.com/tencentyun/tls-sig-api-v2-golang/tencentyun"
)

// UserSig 是 TRTC 身份凭证。expire 为有效期（秒）。
func UserSig(sdkAppID int, secretKey, userID string, expire int) (string, error) {
	return tencentyun.GenUserSig(sdkAppID, secretKey, userID, expire)
}

// Token 是完整的进房凭证：UserSig + PrivateMapKey。
//
// 参数：
//   - sdkAppID / secretKey / userID / expire 同上。
//   - roomID      目标房间号（数值型）。进房时需与 SDK 传入的房间号一致。
//   - privilegeMap 权限位，见下方常量。全权限用 PrivilegeAll。
//
// 说明：这里用数值房间号版本 GenPrivateMapKey。若业务用字符串房间号，
// 请改用 GenPrivateMapKeyWithStringRoomID（见 StringRoomToken）。
type Token struct {
	UserID        string `json:"userId"`
	RoomID        uint32 `json:"roomId,omitempty"`
	RoomIDStr     string `json:"roomIdStr,omitempty"`
	UserSig       string `json:"userSig"`
	PrivateMapKey string `json:"privateMapKey"`
	SdkAppID      int    `json:"sdkAppId"`
}

// Privilege 定义 PrivateMapKey 的权限位常量。
// 多个权限用 | 组合；全权限直接使用 PrivilegeAll(=255)。
const (
	PrivilegeCreateRoom   uint32 = 1  // 创建房间
	PrivilegeJoinRoom     uint32 = 2  // 加入房间
	PrivilegeSendAudio    uint32 = 4  // 发送语音
	PrivilegeRecvAudio    uint32 = 8  // 接收语音
	PrivilegeSendVideo    uint32 = 16 // 发送视频
	PrivilegeRecvVideo    uint32 = 32 // 接收视频
	PrivilegeSendSubVideo uint32 = 64 // 发送辅路(屏幕分享)视频
	PrivilegeRecvSubVideo uint32 = 128
	PrivilegeAll          uint32 = 255 // 所有权限
)

// NumericRoomToken 为指定数值房间号生成完整进房凭证。
func NumericRoomToken(sdkAppID int, secretKey, userID string, roomID uint32, expire int) (*Token, error) {
	userSig, err := tencentyun.GenUserSig(sdkAppID, secretKey, userID, expire)
	if err != nil {
		return nil, err
	}
	privMapKey, err := tencentyun.GenPrivateMapKey(sdkAppID, secretKey, userID, expire, roomID, PrivilegeAll)
	if err != nil {
		return nil, err
	}
	return &Token{
		UserID:        userID,
		RoomID:        roomID,
		UserSig:       userSig,
		PrivateMapKey: privMapKey,
		SdkAppID:      sdkAppID,
	}, nil
}

// StringRoomToken 为指定字符串房间号生成完整进房凭证。
// 适合用字符串作为房间标识（如会议号、业务单号）的场景。
func StringRoomToken(sdkAppID int, secretKey, userID, roomStr string, expire int) (*Token, error) {
	userSig, err := tencentyun.GenUserSig(sdkAppID, secretKey, userID, expire)
	if err != nil {
		return nil, err
	}
	privMapKey, err := tencentyun.GenPrivateMapKeyWithStringRoomID(sdkAppID, secretKey, userID, expire, roomStr, PrivilegeAll)
	if err != nil {
		return nil, err
	}
	return &Token{
		UserID:        userID,
		RoomIDStr:     roomStr,
		UserSig:       userSig,
		PrivateMapKey: privMapKey,
		SdkAppID:      sdkAppID,
	}, nil
}
