// Package store 封装题库服务对 MySQL 的全部读写。
package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"

	"quiz/internal/model"
)

// ErrNotEnoughQuestions 表示某个知识点剩下的新题已经凑不满要求数量。
// 调用方收到它应如实告诉用户「没有新题了」，而不是拿旧题或少量题凑数。
var ErrNotEnoughQuestions = errors.New("该知识点剩余未做的题目不足")

// Store 持有一个数据库连接池，并发安全。
type Store struct {
	db *sql.DB
}

// Open 建立连接并校验连通性。dsn 形如 user:pass@tcp(127.0.0.1:3306)/quiz?parseTime=true。
func Open(dsn string) (*Store, error) {
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("打开数据库失败: %w", err)
	}
	db.SetMaxOpenConns(16)
	db.SetMaxIdleConns(8)
	db.SetConnMaxLifetime(30 * time.Minute)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("连不上数据库: %w", err)
	}
	return &Store{db: db}, nil
}

// Close 释放连接池。
func (s *Store) Close() error { return s.db.Close() }

// Migrate 建表。全部用 IF NOT EXISTS，重复执行无副作用。
func (s *Store) Migrate(ctx context.Context) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS questions (
			id          BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
			qid         VARCHAR(64)      NOT NULL,
			module      VARCHAR(32)      NOT NULL,
			topic_slug  VARCHAR(64)      NOT NULL,
			topic_name  VARCHAR(128)     NOT NULL,
			stem        TEXT             NOT NULL,
			options     JSON             NOT NULL,
			answer      TINYINT UNSIGNED NOT NULL,
			explanation TEXT             NOT NULL,
			difficulty  TINYINT UNSIGNED NOT NULL DEFAULT 2,
			source      VARCHAR(256)     NOT NULL DEFAULT '',
			PRIMARY KEY (id),
			UNIQUE KEY uk_qid (qid),
			KEY idx_topic (topic_slug)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,

		`CREATE TABLE IF NOT EXISTS attempts (
			id            BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
			user_id       VARCHAR(64)     NOT NULL,
			topic_slug    VARCHAR(64)     NOT NULL,
			total         INT UNSIGNED    NOT NULL DEFAULT 0,
			correct       INT UNSIGNED    NOT NULL DEFAULT 0,
			created_at    DATETIME(3)     NOT NULL,
			finished_at   DATETIME(3)     NOT NULL,
			PRIMARY KEY (id),
			KEY idx_user_topic (user_id, topic_slug)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,

		`CREATE TABLE IF NOT EXISTS attempt_items (
			id          BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
			attempt_id  BIGINT UNSIGNED NOT NULL,
			question_id BIGINT UNSIGNED NOT NULL,
			chosen      TINYINT          NULL DEFAULT NULL,
			is_correct  TINYINT UNSIGNED NOT NULL DEFAULT 0,
			PRIMARY KEY (id),
			KEY idx_attempt (attempt_id),
			KEY idx_question (question_id)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,

		// answered 是「这个用户做过哪些题」的唯一真相：抽题时用它排除，
		// 保证同一道新题不会第二次出现在 newly 里。
		`CREATE TABLE IF NOT EXISTS answered (
			user_id      VARCHAR(64)     NOT NULL,
			question_id  BIGINT UNSIGNED NOT NULL,
			times        INT UNSIGNED    NOT NULL DEFAULT 1,
			last_correct TINYINT UNSIGNED NOT NULL DEFAULT 0,
			first_at     DATETIME(3)     NOT NULL,
			last_at      DATETIME(3)     NOT NULL,
			PRIMARY KEY (user_id, question_id)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
	}

	for _, stmt := range stmts {
		if _, err := s.db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("建表失败: %w", err)
		}
	}
	return nil
}

// UpsertQuestions 导入题库：同一 QID 存在则覆盖内容与出处，不存在则插入。
func (s *Store) UpsertQuestions(ctx context.Context, qs []model.Question) (int, error) {
	const stmt = `INSERT INTO questions
		(qid, module, topic_slug, topic_name, stem, options, answer, explanation, difficulty, source)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE
			module=VALUES(module), topic_slug=VALUES(topic_slug), topic_name=VALUES(topic_name),
			stem=VALUES(stem), options=VALUES(options), answer=VALUES(answer),
			explanation=VALUES(explanation), difficulty=VALUES(difficulty), source=VALUES(source)`

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback() // 回滚已提交的空事务在 database/sql 里是安全的

	for _, q := range qs {
		opts, err := json.Marshal(q.Options)
		if err != nil {
			return 0, fmt.Errorf("题 %s 的选项序列化失败: %w", q.QID, err)
		}
		if _, err := tx.ExecContext(ctx, stmt,
			q.QID, q.Module, q.TopicSlug, q.TopicName, q.Stem,
			string(opts), q.Answer, q.Explanation, q.Difficulty, q.Source,
		); err != nil {
			return 0, fmt.Errorf("写入题 %s 失败: %w", q.QID, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return len(qs), nil
}

// CountQuestions 返回题库里的题目总数。
func (s *Store) CountQuestions(ctx context.Context) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM questions`).Scan(&n)
	return n, err
}

const topicColumns = `module, topic_slug, topic_name, source, stem, options, answer, explanation, difficulty`

// ListTopics 汇总每个知识点的题量与作答进度。
func (s *Store) ListTopics(ctx context.Context, userID string) ([]model.Topic, error) {
	const query = `
		SELECT q.module, q.topic_slug, MIN(q.topic_name), MIN(q.source),
		       COUNT(*) AS total,
		       SUM(CASE WHEN a.question_id IS NULL THEN 0 ELSE 1 END) AS done
		FROM questions q
		LEFT JOIN answered a ON a.question_id = q.id AND a.user_id = ?
		GROUP BY q.module, q.topic_slug
		ORDER BY q.module, q.topic_slug`

	rows, err := s.db.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var topics []model.Topic
	for rows.Next() {
		var t model.Topic
		if err := rows.Scan(&t.Module, &t.Slug, &t.Name, &t.Source, &t.Total, &t.Done); err != nil {
			return nil, err
		}
		t.Remaining = t.Total - t.Done
		topics = append(topics, t)
	}
	return topics, rows.Err()
}

// PendingCount 返回该知识点尚未做过的题目数量。
func (s *Store) PendingCount(ctx context.Context, userID, topicSlug string) (int, error) {
	var n int
	const query = `
		SELECT COUNT(*) FROM questions q
		WHERE q.topic_slug = ?
		  AND NOT EXISTS (SELECT 1 FROM answered a WHERE a.user_id = ? AND a.question_id = q.id)`
	err := s.db.QueryRowContext(ctx, query, topicSlug, userID).Scan(&n)
	return n, err
}

// DrawQuestions 从指定知识点的未做题里随机抽 n 道。
// 题量不足时返回 ErrNotEnoughQuestions，绝不重复出已做过的题、也绝不返回空卷。
func (s *Store) DrawQuestions(ctx context.Context, userID, topicSlug string, n int) ([]model.Question, error) {
	pending, err := s.PendingCount(ctx, userID, topicSlug)
	if err != nil {
		return nil, err
	}
	if pending < n {
		return nil, fmt.Errorf("%w：需要 %d 道，仅剩 %d 道新题", ErrNotEnoughQuestions, n, pending)
	}

	// ORDER BY RAND() 在几千行的题库上足够快，不必为此引入额外的随机列。
	query := `
		SELECT qid, ` + topicColumns + `
		FROM questions q
		WHERE q.topic_slug = ?
		  AND NOT EXISTS (SELECT 1 FROM answered a WHERE a.user_id = ? AND a.question_id = q.id)
		ORDER BY RAND()
		LIMIT ` + itoa(n)

	return s.queryQuestions(ctx, query, topicSlug, userID)
}

func (s *Store) queryQuestions(ctx context.Context, query string, args ...any) ([]model.Question, error) {
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []model.Question
	for rows.Next() {
		var q model.Question
		var options string
		if err := rows.Scan(&q.QID, &q.Module, &q.TopicSlug, &q.TopicName, &q.Source,
			&q.Stem, &options, &q.Answer, &q.Explanation, &q.Difficulty); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(options), &q.Options); err != nil {
			return nil, fmt.Errorf("题 %s 的选项解析失败: %w", q.QID, err)
		}
		out = append(out, q)
	}
	return out, rows.Err()
}

// LoadByQIDs 按 QID 取题，用于批改时还原整卷题目。
func (s *Store) LoadByQIDs(ctx context.Context, topicSlug string, qids []string) (map[string]model.Question, error) {
	if len(qids) == 0 {
		return map[string]model.Question{}, nil
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(qids)), ",")
	query := `SELECT qid, ` + topicColumns + ` FROM questions WHERE topic_slug = ? AND qid IN (` + placeholders + `)`

	args := make([]any, 0, len(qids)+1)
	args = append(args, topicSlug)
	for _, id := range qids {
		args = append(args, id)
	}

	list, err := s.queryQuestions(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	out := make(map[string]model.Question, len(list))
	for _, q := range list {
		out[q.QID] = q
	}
	return out, nil
}

// Submit 批改一整卷：逐题判定、记录明细，并把做过的题写进 answered。
func (s *Store) Submit(ctx context.Context, req model.SubmitRequest, questions map[string]model.Question) (model.SubmitResult, error) {
	now := time.Now()
	result := model.SubmitResult{
		Topic: req.Topic,
		Total: len(req.Items),
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return result, err
	}
	defer tx.Rollback()

	res, err := tx.ExecContext(ctx,
		`INSERT INTO attempts (user_id, topic_slug, total, correct, created_at, finished_at)
		 VALUES (?, ?, ?, ?, ?, ?)`, req.UserID, req.Topic, 0, 0, now, now)
	if err != nil {
		return result, fmt.Errorf("创建作答记录失败: %w", err)
	}
	attemptID, err := res.LastInsertId()
	if err != nil {
		return result, err
	}
	result.AttemptID = attemptID

	var topicName string
	for _, item := range req.Items {
		q, ok := questions[item.QID]
		if !ok {
			return result, fmt.Errorf("题目 %s 不属于知识点 %s", item.QID, req.Topic)
		}
		topicName = q.TopicName

		answered := item.Choice >= 0 && item.Choice < len(q.Options)
		correct := answered && item.Choice == q.Answer
		if correct {
			result.Correct++
		}

		// chosen 需要写 NULL 表示「没选」，因此不能直接塞 int
		var chosen any
		if answered {
			chosen = item.Choice
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO attempt_items (attempt_id, question_id, chosen, is_correct)
			 SELECT ?, q.id, ?, ? FROM questions q WHERE q.qid = ?`,
			attemptID, chosen, boolToInt(correct), item.QID); err != nil {
			return result, fmt.Errorf("写入答题明细失败: %w", err)
		}

		if _, err := tx.ExecContext(ctx,
			`INSERT INTO answered (user_id, question_id, times, last_correct, first_at, last_at)
			 SELECT ?, q.id, 1, ?, ?, ? FROM questions q WHERE q.qid = ?
			 ON DUPLICATE KEY UPDATE
			   times = times + 1, last_correct = VALUES(last_correct), last_at = VALUES(last_at)`,
			req.UserID, boolToInt(correct), now, now, item.QID); err != nil {
			return result, fmt.Errorf("更新已做题记录失败: %w", err)
		}

		result.Items = append(result.Items, model.GradedItem{
			QID:         q.QID,
			Stem:        q.Stem,
			Options:     q.Options,
			Chosen:      item.Choice,
			Answer:      q.Answer,
			Correct:     correct,
			Answered:    answered,
			Explanation: q.Explanation,
			Source:      q.Source,
		})
	}

	if _, err := tx.ExecContext(ctx,
		`UPDATE attempts SET total = ?, correct = ? WHERE id = ?`,
		result.Total, result.Correct, attemptID); err != nil {
		return result, err
	}

	if err := tx.Commit(); err != nil {
		return result, err
	}
	_ = topicName
	return result, nil
}

// History 返回最近的作答记录，用于页面上的历史列表。
func (s *Store) History(ctx context.Context, userID string, limit int) ([]model.AttemptSummary, error) {
	query := `
		SELECT at.id, COALESCE(q.module, '') AS module, at.topic_slug,
		       COALESCE(MIN(q.topic_name), at.topic_slug) AS topic_name,
		       at.total, at.correct, at.finished_at
		FROM attempts at
		LEFT JOIN questions q ON q.topic_slug = at.topic_slug
		WHERE at.user_id = ?
		GROUP BY at.id, at.topic_slug, at.total, at.correct, at.finished_at
		ORDER BY at.finished_at DESC
		LIMIT ` + itoa(limit)

	rows, err := s.db.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []model.AttemptSummary
	for rows.Next() {
		var h model.AttemptSummary
		var finished time.Time
		if err := rows.Scan(&h.AttemptID, &h.Module, &h.TopicSlug, &h.TopicName,
			&h.Total, &h.Correct, &finished); err != nil {
			return nil, err
		}
		h.FinishedAt = finished.Format("2006-01-02 15:04")
		out = append(out, h)
	}
	return out, rows.Err()
}

// ResetProgress 清空某用户（默认清空全部）的作答记录，让题库回到「都没做过」。
func (s *Store) ResetProgress(ctx context.Context, userID string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if userID == "" {
		if _, err := tx.ExecContext(ctx, `DELETE FROM answered`); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM attempt_items`); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM attempts`); err != nil {
			return err
		}
		return tx.Commit()
	}

	if _, err := tx.ExecContext(ctx, `DELETE FROM answered WHERE user_id = ?`, userID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM attempt_items WHERE attempt_id IN (SELECT id FROM attempts WHERE user_id = ?)`, userID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM attempts WHERE user_id = ?`, userID); err != nil {
		return err
	}
	return tx.Commit()
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// itoa 把 LIMIT 的数量安全地拼进 SQL：它是 int 参与的字符串拼接，不接受外部字符串输入。
func itoa(n int) string {
	return fmt.Sprintf("%d", n)
}
