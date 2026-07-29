package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/index"
)

type User struct {
	ent.Schema
}

func (User) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("id"),
		field.String("username").Unique().MaxLen(32),
		field.String("email").Unique().MaxLen(128),
		field.String("phone").MaxLen(20).Optional(),
		field.Int("age").Default(0).Optional(),
		field.Time("created_at"),
		field.Time("updated_at"),
	}
}

func (User) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("posts", Post.Type),
	}
}

func (User) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("phone"),
	}
}
