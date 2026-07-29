package repository

import (
	"context"
	"time"

	"github.com/skyandong/go-code/webapp/internal/data/gen"
	"github.com/skyandong/go-code/webapp/internal/data/gen/post"
	"github.com/skyandong/go-code/webapp/internal/data/gen/user"
)

type PostRepo struct {
	client *gen.Client
}

func NewPostRepo(client *gen.Client) *PostRepo {
	return &PostRepo{client: client}
}

type CreatePostInput struct {
	Title    string
	Content  string
	AuthorID int64
}

func (r *PostRepo) Create(ctx context.Context, input CreatePostInput) (*gen.Post, error) {
	return r.client.Post.Create().
		SetTitle(input.Title).
		SetContent(input.Content).
		SetAuthorID(input.AuthorID).
		SetCreatedAt(time.Now()).
		SetUpdatedAt(time.Now()).
		Save(ctx)
}

func (r *PostRepo) GetByID(ctx context.Context, id int64) (*gen.Post, error) {
	return r.client.Post.Get(ctx, id)
}

func (r *PostRepo) ListByAuthor(ctx context.Context, authorID int64) ([]*gen.Post, error) {
	return r.client.Post.Query().
		Where(post.AuthorIDEQ(authorID)).
		Order(gen.Desc(post.FieldCreatedAt)).
		All(ctx)
}

func (r *PostRepo) Delete(ctx context.Context, id int64) error {
	return r.client.Post.DeleteOneID(id).Exec(ctx)
}

// ---- 联表查询三种姿势 ----

// 1️⃣ Eager Loading：文章 + 作者（LEFT JOIN）
func (r *PostRepo) ListWithAuthor(ctx context.Context, offset, limit int) ([]*gen.Post, error) {
	return r.client.Post.Query().
		WithAuthor().
		Offset(offset).
		Limit(limit).
		Order(gen.Desc(post.FieldCreatedAt)).
		All(ctx)
}

// 2️⃣ 图遍历：从用户→文章（跨 Edge 导航，不写 JOIN）
func (r *PostRepo) ListByUserGraph(ctx context.Context, userID int64) ([]*gen.Post, error) {
	return r.client.User.Query().
		Where(user.IDEQ(userID)).
		QueryPosts(). // ← 沿着 User → Post 的 Edge 跳过去
		All(ctx)
}

// 3️⃣ 嵌套预加载：查作者时把文章一起带出来
func (r *PostRepo) GetAuthorWithPosts(ctx context.Context, userID int64) (*gen.User, error) {
	return r.client.User.Query().
		Where(user.IDEQ(userID)).
		WithPosts(). // ← 把用户的文章一起查出
		Only(ctx)
}
