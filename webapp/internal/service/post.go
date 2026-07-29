package service

import (
	"context"
	"fmt"

	"github.com/skyandong/go-code/webapp/internal/data/gen"
	"github.com/skyandong/go-code/webapp/internal/repository"
)

type PostService struct {
	postRepo *repository.PostRepo
	userRepo *repository.UserRepo
}

func NewPostService(postRepo *repository.PostRepo, userRepo *repository.UserRepo) *PostService {
	return &PostService{postRepo: postRepo, userRepo: userRepo}
}

type CreatePostReq struct {
	Title    string `json:"title" binding:"required"`
	Content  string `json:"content" binding:"required"`
	AuthorID int64  `json:"author_id" binding:"required"`
}

func (s *PostService) Create(ctx context.Context, req CreatePostReq) (*gen.Post, error) {
	// 检查作者是否存在
	_, err := s.userRepo.GetByID(ctx, req.AuthorID)
	if err != nil {
		return nil, fmt.Errorf("作者不存在: %w", err)
	}
	return s.postRepo.Create(ctx, repository.CreatePostInput{
		Title:    req.Title,
		Content:  req.Content,
		AuthorID: req.AuthorID,
	})
}

func (s *PostService) GetByID(ctx context.Context, id int64) (*gen.Post, error) {
	return s.postRepo.GetByID(ctx, id)
}

func (s *PostService) ListByAuthor(ctx context.Context, authorID int64) ([]*gen.Post, error) {
	return s.postRepo.ListByAuthor(ctx, authorID)
}

// ListWithAuthor 文章列表（含作者信息），返回数据已预加载作者
type PostWithAuthor struct {
	*gen.Post
	Author *gen.User `json:"author"`
}

func (s *PostService) ListWithAuthor(ctx context.Context, page, size int) ([]*gen.Post, error) {
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 100 {
		size = 20
	}
	offset := (page - 1) * size
	return s.postRepo.ListWithAuthor(ctx, offset, size)
}

func (s *PostService) Delete(ctx context.Context, id int64) error {
	return s.postRepo.Delete(ctx, id)
}
