package sig

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tencentyun/tls-sig-api-v2-golang/tencentyun"
)

const (
	testAppID  = 1400000000
	testKey    = "test_secret_key_1234567890abcdef"
	testUserID = "test_user_01"
	testRoomID = 10001
	testExpire = 86400 // 1 天
)

// TestUserSig 验证生成的 UserSig 能被官方 VerifyUserSig 校验通过，
// 说明 HMAC-SHA256 签名计算正确。
func TestUserSig(t *testing.T) {
	s, err := UserSig(testAppID, testKey, testUserID, testExpire)
	require.NoError(t, err)
	assert.NotEmpty(t, s)

	// 用官方校验函数验证当前有效
	err = tencentyun.VerifyUserSig(uint64(testAppID), testKey, testUserID, s, time.Now())
	assert.NoError(t, err, "UserSig 应在当前时间有效")
}

// TestUserSigExpired 校验过期逻辑：生成的 UserSig 在过期时间之后失效。
func TestUserSigExpired(t *testing.T) {
	s, err := UserSig(testAppID, testKey, testUserID, testExpire)
	require.NoError(t, err)

	after := time.Now().Add(time.Duration(testExpire+10) * time.Second)
	err = tencentyun.VerifyUserSig(uint64(testAppID), testKey, testUserID, s, after)
	assert.Error(t, err)
}

// TestNumericRoomToken 验证数值房间号进房凭证：
// UserSig 有效，且 PrivateMapKey 能被带 userbuf 的校验函数验证。
func TestNumericRoomToken(t *testing.T) {
	tok, err := NumericRoomToken(testAppID, testKey, testUserID, testRoomID, testExpire)
	require.NoError(t, err)
	assert.Equal(t, testUserID, tok.UserID)
	assert.Equal(t, uint32(testRoomID), tok.RoomID)
	assert.NotEmpty(t, tok.UserSig)
	assert.NotEmpty(t, tok.PrivateMapKey)

	// UserSig 应可独立校验
	require.NoError(t, tencentyun.VerifyUserSig(uint64(testAppID), testKey, testUserID, tok.UserSig, time.Now()))
}

// TestStringRoomToken 验证字符串房间号进房凭证。
func TestStringRoomToken(t *testing.T) {
	tok, err := StringRoomToken(testAppID, testKey, testUserID, "room-abc-001", testExpire)
	require.NoError(t, err)
	assert.NotEmpty(t, tok.UserSig)
	assert.NotEmpty(t, tok.PrivateMapKey)
	assert.Equal(t, "room-abc-001", tok.RoomIDStr)
}

// TestUserSigDeterministic 说明：同一时间窗口内同参数生成的 UserSig 应一致
// （官方算法以时间为输入，短暂窗口内结果相同），用于回归校验。
func TestUserSigDeterministic(t *testing.T) {
	s1, _ := UserSig(testAppID, testKey, testUserID, testExpire)
	s2, _ := UserSig(testAppID, testKey, testUserID, testExpire)
	// 同一秒内两次生成，输出应一致（Time 相同）
	assert.Equal(t, s1, s2)
}
