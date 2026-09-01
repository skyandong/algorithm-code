package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/skyandong/go-code/trtc-demo/config"
	"github.com/skyandong/go-code/trtc-demo/sig"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestService(t *testing.T) *Service {
	cfg := &config.Config{
		SDKAppID:     1400000000,
		SDKSecretKey: "test_secret_key_1234567890abcdef",
		Expire:       3600,
		WebDir:       "../web",
	}
	return NewService(cfg)
}

func doGet(t *testing.T, svc *Service, url string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, url, nil)
	rec := httptest.NewRecorder()
	svc.ServeHTTP(rec, req)
	return rec
}

func TestHealth(t *testing.T) {
	svc := newTestService(t)
	rec := doGet(t, svc, "/healthz")
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.JSONEq(t, `{"status":"ok"}`, rec.Body.String())
}

func TestUserSigEndpoint(t *testing.T) {
	svc := newTestService(t)
	rec := doGet(t, svc, "/api/usersig?userId=alice_001")
	require.Equal(t, http.StatusOK, rec.Code)

	var body struct {
		UserID  string `json:"userId"`
		UserSig string `json:"userSig"`
		Expire  int    `json:"expire"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "alice_001", body.UserID)
	assert.NotEmpty(t, body.UserSig)
	assert.Equal(t, 3600, body.Expire)
}

func TestUserSigEndpointInvalidUser(t *testing.T) {
	svc := newTestService(t)
	// 用 URL 编码传入非法 userId，避免裸特殊字符破坏 URL 解析
	rec := doGet(t, svc, "/api/usersig?userId="+url.QueryEscape("bad user!@#"))
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestTokenEndpoint(t *testing.T) {
	svc := newTestService(t)
	rec := doGet(t, svc, "/api/token?userId=bob&roomId=999")
	require.Equal(t, http.StatusOK, rec.Code)

	var tok sig.Token
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &tok))
	assert.Equal(t, "bob", tok.UserID)
	assert.Equal(t, uint32(999), tok.RoomID)
	assert.NotEmpty(t, tok.UserSig)
	assert.NotEmpty(t, tok.PrivateMapKey)
}

func TestTokenEndpointInvalidRoom(t *testing.T) {
	svc := newTestService(t)

	rec1 := doGet(t, svc, "/api/token?userId=bob&roomId=abc")
	assert.Equal(t, http.StatusBadRequest, rec1.Code)

	rec2 := doGet(t, svc, "/api/token?userId=bob&roomId=0")
	assert.Equal(t, http.StatusBadRequest, rec2.Code)
}

func TestTokenEndpointCustomExpire(t *testing.T) {
	svc := newTestService(t)
	rec := doGet(t, svc, "/api/token?userId=bob&roomId=123&expire=60")
	require.Equal(t, http.StatusOK, rec.Code)
	// expire 参数目前用于签发，不在响应体中回显，这里仅验证能成功生成
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestCORSHeaders(t *testing.T) {
	svc := newTestService(t)

	// OPTIONS 预检请求
	req := httptest.NewRequest(http.MethodOptions, "/api/token", nil)
	rec := httptest.NewRecorder()
	svc.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusNoContent, rec.Code)
	assert.Equal(t, "*", rec.Header().Get("Access-Control-Allow-Origin"))

	// 普通请求带 CORS 头
	rec2 := doGet(t, svc, "/api/usersig?userId=cors_user")
	assert.Equal(t, "*", rec2.Header().Get("Access-Control-Allow-Origin"))
}

func TestWebPageServed(t *testing.T) {
	svc := newTestService(t)
	rec := doGet(t, svc, "/")
	assert.Equal(t, http.StatusOK, rec.Code)
	// 页面应包含 TRTC SDK 引用和标题
	body := rec.Body.String()
	assert.Contains(t, body, "trtc.js")
	assert.Contains(t, body, "TRTC 视频通话自测 Demo")
}
