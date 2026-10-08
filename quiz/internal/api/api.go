// Package api 把题库能力暴露成 HTTP 接口，并顺带托管答题页面的静态资源。
package api

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"

	"quiz/internal/model"
	"quiz/internal/store"
)

// DefaultUser 是没有携带用户标识时的兜底值。单人本地使用，进度按标识区分。
const DefaultUser = "local"

// DefaultDrawCount 是一次出题的题量。
const DefaultDrawCount = 5

// Server 是 HTTP 层的依赖集合。
type Server struct {
	store  *store.Store
	webDir string
}

// New 构造一个 Server，webDir 是静态资源目录。
func New(st *store.Store, webDir string) *Server {
	return &Server{store: st, webDir: webDir}
}

// Routes 注册全部路由。Go 1.22 起的方法前缀写法让路由表一眼能读完。
func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/topics", s.handleTopics)
	mux.HandleFunc("GET /api/quiz", s.handleQuiz)
	mux.HandleFunc("POST /api/submit", s.handleSubmit)
	mux.HandleFunc("GET /api/history", s.handleHistory)
	mux.HandleFunc("POST /api/reset", s.handleReset)
	mux.Handle("GET /", http.FileServer(http.Dir(s.webDir)))
	return mux
}

// handleTopics 返回知识点列表与各自的作答进度。
func (s *Server) handleTopics(w http.ResponseWriter, r *http.Request) {
	topics, err := s.store.ListTopics(r.Context(), userID(r))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "读取知识点失败: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"topics": topics})
}

// handleQuiz 按知识点抽题。mode 取 new（默认）或 wrong。
// 新题凑不满一轮时拒绝出题；错题有多少出多少，一道都没有才拒绝。
func (s *Server) handleQuiz(w http.ResponseWriter, r *http.Request) {
	topic := strings.TrimSpace(r.URL.Query().Get("topic"))
	if topic == "" {
		writeError(w, http.StatusBadRequest, "缺少 topic 参数")
		return
	}

	mode := model.DrawMode(strings.TrimSpace(r.URL.Query().Get("mode")))
	if mode == "" {
		mode = model.ModeNew
	}
	if !mode.Valid() {
		writeError(w, http.StatusBadRequest, "mode 只能是 new 或 wrong")
		return
	}

	n := DefaultDrawCount
	if raw := r.URL.Query().Get("n"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed <= 0 {
			writeError(w, http.StatusBadRequest, "n 必须是正整数")
			return
		}
		n = parsed
	}

	questions, err := s.store.DrawQuestions(r.Context(), userID(r), topic, n, mode)
	if errors.Is(err, store.ErrNotEnoughQuestions) || errors.Is(err, store.ErrNoWrongQuestions) {
		pending, countErr := s.store.CountPending(r.Context(), userID(r), topic, mode)
		if countErr != nil {
			writeError(w, http.StatusInternalServerError, "统计剩余题目失败: "+countErr.Error())
			return
		}
		message := "该知识点只剩 " + strconv.Itoa(pending) + " 道没做过的新题，凑不满一轮 " +
			strconv.Itoa(n) + " 道，暂不出题；先做别的知识点，或者清空进度重刷。"
		if mode == model.ModeWrong {
			message = "这个知识点还没有错题。做错的题会进错题池，可以反复重练，做对一次才算过关。"
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"available": false,
			"pending":   pending,
			"need":      n,
			"mode":      mode,
			"message":   message,
		})
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "抽题失败: "+err.Error())
		return
	}

	views := make([]model.QuestionView, 0, len(questions))
	for _, q := range questions {
		views = append(views, q.View())
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"available": true,
		"topic":     topic,
		"topicName": questions[0].TopicName,
		"mode":      mode,
		"count":     len(views),
		"questions": views,
	})
}

// handleSubmit 批改整卷：逐题给出对错、正确项与解析。
func (s *Server) handleSubmit(w http.ResponseWriter, r *http.Request) {
	var req model.SubmitRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "请求体不是合法 JSON: "+err.Error())
		return
	}
	if req.Topic == "" || len(req.Items) == 0 {
		writeError(w, http.StatusBadRequest, "topic 与 items 都不能为空")
		return
	}
	if req.UserID == "" {
		req.UserID = userID(r)
	}

	qids := make([]string, 0, len(req.Items))
	seen := make(map[string]bool, len(req.Items))
	for _, item := range req.Items {
		if seen[item.QID] {
			writeError(w, http.StatusBadRequest, "同一道题重复提交: "+item.QID)
			return
		}
		seen[item.QID] = true
		qids = append(qids, item.QID)
	}

	questions, err := s.store.LoadByQIDs(r.Context(), req.Topic, qids)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "读取题目失败: "+err.Error())
		return
	}
	for _, qid := range qids {
		if _, ok := questions[qid]; !ok {
			writeError(w, http.StatusBadRequest, "题目不存在或不属于该知识点: "+qid)
			return
		}
	}

	// 上一次成绩只是给结果页做对比，读失败不该挡住本次批改。
	previous, prevErr := s.store.LastAttempt(r.Context(), req.UserID, req.Topic)
	if prevErr != nil {
		log.Printf("读取上次成绩失败 user=%s topic=%s: %v", req.UserID, req.Topic, prevErr)
		previous = nil
	}

	result, err := s.store.Submit(r.Context(), req, questions)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "批改失败: "+err.Error())
		return
	}
	result.Previous = previous
	log.Printf("批改完成 user=%s topic=%s 得分=%d/%d", req.UserID, req.Topic, result.Correct, result.Total)
	writeJSON(w, http.StatusOK, result)
}

// handleHistory 返回最近的作答记录。
func (s *Server) handleHistory(w http.ResponseWriter, r *http.Request) {
	records, err := s.store.History(r.Context(), userID(r), 20)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "读取历史失败: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"records": records})
}

// handleReset 清空作答进度，让题库回到都没做过的状态。
func (s *Server) handleReset(w http.ResponseWriter, r *http.Request) {
	if err := s.store.ResetProgress(r.Context(), userID(r)); err != nil {
		writeError(w, http.StatusInternalServerError, "清空进度失败: "+err.Error())
		return
	}
	log.Printf("已清空作答进度 user=%s", userID(r))
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// userID 从查询串里取用户标识，取不到就用兜底值。
func userID(r *http.Request) string {
	if v := strings.TrimSpace(r.URL.Query().Get("user")); v != "" {
		return v
	}
	return DefaultUser
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		log.Printf("写响应失败: %v", err)
	}
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]any{"error": msg})
}
