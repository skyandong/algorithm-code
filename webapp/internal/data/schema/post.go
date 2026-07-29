package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/edge"
)

type Post struct {
	ent.Schema
}

func (Post) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("id"),
		field.String("title").NotEmpty().MaxLen(200),
		field.Text("content").NotEmpty(),
		field.Int64("author_id"),
		field.Time("created_at"),
		field.Time("updated_at"),
	}
}

func (Post) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("author", User.Type).
			Ref("posts").
			Unique().
			Required().
			Field("author_id"),
	}
}
