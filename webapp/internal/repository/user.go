package repository

import (
	"context"
	"time"

	"github.com/skyandong/go-code/webapp/internal/data/gen"
	"github.com/skyandong/go-code/webapp/internal/data/gen/user"
)

type UserRepo struct {
	client *gen.Client
}

func NewUserRepo(client *gen.Client) *UserRepo {
	return &UserRepo{client: client}
}

type CreateUserInput struct {
	Username string
	Email    string
	Phone    string
	Age      int
}

func (r *UserRepo) Create(ctx context.Context, input CreateUserInput) (*gen.User, error) {
	return r.client.User.Create().
		SetUsername(input.Username).
		SetEmail(input.Email).
		SetPhone(input.Phone).
		SetAge(input.Age).
		SetCreatedAt(time.Now()).
		SetUpdatedAt(time.Now()).
		Save(ctx)
}

func (r *UserRepo) GetByID(ctx context.Context, id int64) (*gen.User, error) {
	return r.client.User.Get(ctx, id)
}

func (r *UserRepo) GetByUsername(ctx context.Context, name string) (*gen.User, error) {
	return r.client.User.Query().
		Where(user.UsernameEQ(name)).
		Only(ctx)
}

func (r *UserRepo) List(ctx context.Context, offset, limit int) ([]*gen.User, error) {
	return r.client.User.Query().
		Offset(offset).
		Limit(limit).
		Order(gen.Desc(user.FieldID)).
		All(ctx)
}

func (r *UserRepo) UpdateAge(ctx context.Context, id int64, age int) error {
	return r.client.User.UpdateOneID(id).
		SetAge(age).
		SetUpdatedAt(time.Now()).
		Exec(ctx)
}

func (r *UserRepo) Delete(ctx context.Context, id int64) error {
	return r.client.User.DeleteOneID(id).Exec(ctx)
}
