package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/edge"
)

// Tag 标签
type Tag struct {
	ent.Schema
}

func (Tag) Fields() []ent.Field {
	return []ent.Field{
		field.String("name").Unique().MaxLen(32),
	}
}

func (Tag) Edges() []ent.Edge {
	return []ent.Edge{
		// Tag 被多个 Post 引用（多对多的反向）
		edge.From("posts", Post.Type).
			Ref("tags"),
	}
}
