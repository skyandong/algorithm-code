package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/edge"
)

// Post 文章
type Post struct {
	ent.Schema
}

func (Post) Fields() []ent.Field {
	return []ent.Field{
		field.String("title").NotEmpty().MaxLen(200),
		field.Text("content").NotEmpty(),
		field.Time("created_at"),
	}
}

// Edges 定义与其他实体的关系
func (Post) Edges() []ent.Edge {
	return []ent.Edge{
		// Post 属于一个 User（多对一）
		edge.From("author", User.Type).
			Ref("posts").
			Unique().
			Required(),
		// Post 有多个 Tag（多对多）
		edge.To("tags", Tag.Type),
	}
}
