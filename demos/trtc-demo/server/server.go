// Package server 提供 TRTC 签名服务的 HTTP 接口。
//
// 设计为"只写 Go API 后端"：客户端(Web/App)在进房前先调用这里的接口
// 换取 UserSig / PrivateMapKey，再交给 TRTC SDK 进房。SDKSecretKey
// 始终留在服务端，不下发到客户端。
package server

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"regexp"
	"strconv"

	"github.com/skyandong/go-code/trtc-demo/config"
	"github.com/skyandong/go-code/trtc-demo/sig"
)

// WebDir 是前端页面目录相对路径。从模块根目录运行时指向 ./web。
// 可通过环境变量 TRTC_WEB_DIR 覆盖。
const defaultWebDir = "./web"

// Service 持有配置并注册路由。
type Service struct {
	cfg *config.Config
	mux *http.ServeMux
}

// NewService 创建服务并注册所有路由。
func NewService(cfg *config.Config) *Service {
	s := &Service{cfg: cfg, mux: http.NewServeMux()}

	// 健康检查
	s.mux.HandleFunc("GET /healthz", s.handleHealth)

	// 只签发 UserSig（身份凭证），不进房
	s.mux.HandleFunc("GET /api/usersig", s.handleUserSig)

	// 签发完整进房凭证 UserSig + PrivateMapKey
	s.mux.HandleFunc("GET /api/token", s.handleToken)

	// 静态托管前端页面（web/index.html）
	s.mux.Handle("GET /", s.webHandler(cfg.WebDir))

	return s
}

// webHandler 托管前端静态页面目录。目录不存在时给出提示页。
func (s *Service) webHandler(dir string) http.Handler {
	if info, err := os.Stat(dir); err == nil && info.IsDir() {
		log.Printf("托管前端页面目录: %s", dir)
		return http.FileServer(http.Dir(dir))
	}
	log.Printf("警告: 前端页面目录 %q 不存在，静态页面不可用", dir)
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "前端页面目录未找到: "+dir+"\n请从 trtc-demo 目录启动服务", http.StatusNotFound)
	})
}

// Handler 返回可挂载的 http.Handler（已带 CORS 与日志）。
func (s *Service) Handler() http.Handler {
	return s.logMiddleware(s.corsMiddleware(s.mux))
}

// ServeHTTP 实现 http.Handler，便于直接对外监听。
func (s *Service) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.Handler().ServeHTTP(w, r)
}

// userID 校验：字母/数字/下划线/连词符，长度 ≤ 32。
var userIDRe = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,32}$`)

// handleUserSig 返回 UserSig 身份凭证。
// 参数：userId 必填。
func (s *Service) handleUserSig(w http.ResponseWriter, r *http.Request) {
	userID := r.URL.Query().Get("userId")
	if !userIDRe.MatchString(userID) {
		writeErr(w, http.StatusBadRequest, "invalid userId: 需为1-32位字母/数字/下划线/连词符")
		return
	}

	userSig, err := sig.UserSig(s.cfg.SDKAppID, s.cfg.SDKSecretKey, userID, s.cfg.Expire)
	if err != nil {
		log.Printf("GenUserSig error: %v", err)
		writeErr(w, http.StatusInternalServerError, "failed to generate userSig")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"userId":  userID,
		"userSig": userSig,
		"expire":  s.cfg.Expire,
	})
}

// handleToken 返回完整进房凭证 UserSig + PrivateMapKey。
// 参数：
//   - userId 必填
//   - roomId 必填，数值房间号
//   - expire 可选，覆盖默认过期时间（秒）
func (s *Service) handleToken(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	userID := q.Get("userId")
	if !userIDRe.MatchString(userID) {
		writeErr(w, http.StatusBadRequest, "invalid userId: 需为1-32位字母/数字/下划线/连词符")
		return
	}

	roomID64, err := strconv.ParseUint(q.Get("roomId"), 10, 32)
	if err != nil || roomID64 == 0 {
		writeErr(w, http.StatusBadRequest, "invalid roomId: 需为正整数房间号")
		return
	}

	expire := s.cfg.Expire
	if e := q.Get("expire"); e != "" {
		if ev, err := strconv.Atoi(e); err == nil && ev > 0 {
			expire = ev
		}
	}

	tok, err := sig.NumericRoomToken(s.cfg.SDKAppID, s.cfg.SDKSecretKey, userID, uint32(roomID64), expire)
	if err != nil {
		log.Printf("GenPrivateMapKey error: %v", err)
		writeErr(w, http.StatusInternalServerError, "failed to generate token")
		return
	}

	writeJSON(w, http.StatusOK, tok)
}

func (s *Service) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}

// ---------- 工具函数 ----------

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]any{"error": msg})
}

// corsMiddleware 允许跨域，便于 Web 前端直接调用。
func (s *Service) corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// logMiddleware 打印访问日志。
func (s *Service) logMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("%s %s", r.Method, r.URL.Path)
		next.ServeHTTP(w, r)
	})
}
