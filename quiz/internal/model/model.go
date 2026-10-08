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
// Done 恒等于 Correct + Wrong：做过的题按最后一次的对错二分。
type Topic struct {
	Module    string `json:"module"`
	Slug      string `json:"slug"`
	Name      string `json:"name"`
	Source    string `json:"source"`
	Total     int    `json:"total"`
	Done      int    `json:"done"`
	Correct   int    `json:"correct"`
	Wrong     int    `json:"wrong"`
	Remaining int    `json:"remaining"`
}

// DrawMode 决定一轮抽题的范围。
// 新题只取从未做过的，错题只取上次做错的，两者互不重叠。
type DrawMode string

const (
	// ModeNew 抽取从未做过的题。
	ModeNew DrawMode = "new"
	// ModeWrong 抽取上次做错的题，用于错题重练。
	ModeWrong DrawMode = "wrong"
)

// Valid 判断模式取值是否受支持。
func (m DrawMode) Valid() bool {
	return m == ModeNew || m == ModeWrong
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
	// Previous 是该知识点上一次的成绩，用于结果页对比。没有历史时为 nil。
	Previous *AttemptScore `json:"previous,omitempty"`
}

// AttemptScore 是一次作答的成绩，只在对比上次时使用。
type AttemptScore struct {
	Correct int `json:"correct"`
	Total   int `json:"total"`
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
