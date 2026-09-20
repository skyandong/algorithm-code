// Package model 定义答题评测服务的领域结构。
package model

// Question 是题库里的一道选择题。
// QID 在整套题库内唯一且稳定：同一个 QID 重复导入时覆盖内容，不会产生重复记录。
type Question struct {
	QID         string   `json:"qid"`
	Module      string   `json:"module"`
	TopicSlug   string   `json:"topicSlug"`
	TopicName   string   `json:"topicName"`
	Stem        string   `json:"stem"`
	Options     []string `json:"options"`
	Answer      int      `json:"answer"`
	Explanation string   `json:"explanation"`
	Difficulty  int      `json:"difficulty"`
	Source      string   `json:"source"`
}

// QuestionView 下发给答题页面的题目。
// 它刻意不带 Answer 与 Explanation，客户端在提交前拿不到答案。
type QuestionView struct {
	QID        string   `json:"qid"`
	TopicSlug  string   `json:"topicSlug"`
	TopicName  string   `json:"topicName"`
	Stem       string   `json:"stem"`
	Options    []string `json:"options"`
	Difficulty int      `json:"difficulty"`
}

// View 把题目转成不含答案的下发视图。
func (q Question) View() QuestionView {
	return QuestionView{
		QID:        q.QID,
		TopicSlug:  q.TopicSlug,
		TopicName:  q.TopicName,
		Stem:       q.Stem,
		Options:    q.Options,
		Difficulty: q.Difficulty,
	}
}

// Topic 汇总一个知识点的题量与作答进度。
type Topic struct {
	Module    string `json:"module"`
	Slug      string `json:"slug"`
	Name      string `json:"name"`
	Source    string `json:"source"`
	Total     int    `json:"total"`
	Done      int    `json:"done"`
	Remaining int    `json:"remaining"`
}

// AnswerPayload 是用户对一道题的选择。Choice 为 -1 表示未作答。
type AnswerPayload struct {
	QID    string `json:"qid"`
	Choice int    `json:"choice"`
}

// SubmitRequest 是一次整卷提交。
type SubmitRequest struct {
	UserID string          `json:"userId"`
	Topic  string          `json:"topic"`
	Items  []AnswerPayload `json:"items"`
}

// GradedItem 是单道题的批改结果：做对做错都带完整解析。
type GradedItem struct {
	QID         string   `json:"qid"`
	Stem        string   `json:"stem"`
	Options     []string `json:"options"`
	Chosen      int      `json:"chosen"`
	Answer      int      `json:"answer"`
	Correct     bool     `json:"correct"`
	Answered    bool     `json:"answered"`
	Explanation string   `json:"explanation"`
	Source      string   `json:"source"`
}

// SubmitResult 是整卷批改结果。
type SubmitResult struct {
	AttemptID int64        `json:"attemptId"`
	Topic     string       `json:"topic"`
	Total     int          `json:"total"`
	Correct   int          `json:"correct"`
	Items     []GradedItem `json:"items"`
}

// AttemptSummary 是历史记录里的一次作答摘要。
type AttemptSummary struct {
	AttemptID  int64  `json:"attemptId"`
	Module     string `json:"module"`
	TopicSlug  string `json:"topicSlug"`
	TopicName  string `json:"topicName"`
	Total      int    `json:"total"`
	Correct    int    `json:"correct"`
	FinishedAt string `json:"finishedAt"`
}
