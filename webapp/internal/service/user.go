package service

import (
	"context"
	"fmt"

	"github.com/skyandong/go-code/webapp/internal/data/gen"
	"github.com/skyandong/go-code/webapp/internal/repository"
)

type UserService struct {
	repo *repository.UserRepo
}

func NewUserService(repo *repository.UserRepo) *UserService {
	return &UserService{repo: repo}
}

type CreateUserReq struct {
	Username string `json:"username" binding:"required"`
	Email    string `json:"email" binding:"required"`
	Phone    string `json:"phone"`
	Age      int    `json:"age"`
}

func (s *UserService) Create(ctx context.Context, req CreateUserReq) (*gen.User, error) {
	// 业务校验
	if len(req.Username) < 2 {
		return nil, fmt.Errorf("用户名至少2个字符")
	}
	return s.repo.Create(ctx, repository.CreateUserInput{
		Username: req.Username,
		Email:    req.Email,
		Phone:    req.Phone,
		Age:      req.Age,
	})
}

func (s *UserService) GetByID(ctx context.Context, id int64) (*gen.User, error) {
	return s.repo.GetByID(ctx, id)
}

func (s *UserService) List(ctx context.Context, page, size int) ([]*gen.User, error) {
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 100 {
		size = 20
	}
	offset := (page - 1) * size
	return s.repo.List(ctx, offset, size)
}

func (s *UserService) Delete(ctx context.Context, id int64) error {
	return s.repo.Delete(ctx, id)
}
