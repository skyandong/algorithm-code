package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
)

// User 用户
type User struct {
	ent.Schema
}

func (User) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("id"),
		field.String("username").Unique().MaxLen(32),
		field.String("email").Unique().MaxLen(128),
		field.Int("age").Default(0).Optional(),
	}
}

// Edges 一个 User 有多个 Post
func (User) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("posts", Post.Type),
	}
}
