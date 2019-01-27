/*
Package grpc provides user grpc server
*/
package grpc

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/golang/protobuf/ptypes/empty"
	"github.com/vardius/go-api-boilerplate/pkg/common/application/errors"
	"github.com/vardius/go-api-boilerplate/pkg/common/application/jwt"
	"github.com/vardius/go-api-boilerplate/pkg/common/infrastructure/commandbus"
	"github.com/vardius/go-api-boilerplate/pkg/common/infrastructure/eventbus"
	"github.com/vardius/go-api-boilerplate/pkg/common/infrastructure/eventstore"
	"github.com/vardius/go-api-boilerplate/pkg/user/application"
	"github.com/vardius/go-api-boilerplate/pkg/user/domain/user"
	"github.com/vardius/go-api-boilerplate/pkg/user/infrastructure/persistence/mysql"
	"github.com/vardius/go-api-boilerplate/pkg/user/infrastructure/proto"
	"github.com/vardius/go-api-boilerplate/pkg/user/infrastructure/repository"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type userServer struct {
	commandBus commandbus.CommandBus
	eventBus   eventbus.EventBus
	eventStore eventstore.EventStore
	db         *sql.DB
	jwt        jwt.Jwt
}

// NewServer returns new user server object
func NewServer(cb commandbus.CommandBus, eb eventbus.EventBus, es eventstore.EventStore, db *sql.DB, j jwt.Jwt) proto.UserServiceServer {
	s := &userServer{cb, eb, es, db, j}

	userRepository := repository.NewUserRepository(es, eb)
	userMYSQLRepository := mysql.NewUserRepository(db)

	s.registerCommandHandlers(userRepository)
	s.registerEventHandlers(userMYSQLRepository)

	return s
}

// DispatchCommand implements proto.UserServiceServer interface
func (s *userServer) DispatchCommand(ctx context.Context, r *proto.DispatchCommandRequest) (*empty.Empty, error) {
	out := make(chan error)
	defer close(out)

	go func() {
		defer func() {
			if rec := recover(); rec != nil {
				out <- errors.Newf(errors.INTERNAL, "Recovered in f %v", rec)
			}
		}()

		c, err := buildDomainCommand(ctx, r.GetName(), r.GetPayload())
		if err != nil {
			out <- err
			return
		}

		s.commandBus.Publish(ctx, fmt.Sprintf("%T", c), c, out)
	}()

	select {
	case <-ctx.Done():
		return new(empty.Empty), ctx.Err()
	case err := <-out:
		return new(empty.Empty), err
	}
}

// GetUser implements proto.UserServiceServer interface
func (s *userServer) GetUser(ctx context.Context, r *proto.GetUserRequest) (*proto.User, error) {
	repository := mysql.NewUserRepository(s.db)

	u, err := repository.Get(ctx, r.GetId())
	if err != nil {
		return nil, status.Error(codes.NotFound, "User not found")
	}

	response := &proto.User{
		Id:         u.ID.String(),
		Email:      u.Email,
		FacebookId: u.FacebookID,
		GoogleId:   u.GoogleID,
	}

	return response, nil
}

// ListUsers implements proto.UserServiceServer interface
func (s *userServer) ListUsers(ctx context.Context, r *proto.ListUserRequest) (*proto.ListUserResponse, error) {
	var totalUsers int32
	var users []*proto.User

	row := s.db.QueryRowContext(ctx, `SELECT COUNT(distinctId) FROM users`)
	err := row.Scan(&totalUsers)
	if err != nil {
		return nil, status.Error(codes.Internal, "Failed to count users")
	}

	if totalUsers < 1 {
		return &proto.ListUserResponse{
			Users: users,
			Total: 0,
		}, nil
	}

	repository := mysql.NewUserRepository(s.db)
	offset := (r.GetPage() * r.GetLimit()) - r.GetLimit()

	results, err := repository.FindAll(ctx, r.GetLimit(), offset)
	if err != nil {
		return nil, status.Error(codes.Internal, "Failed to fetch users")
	}

	for _, u := range results {
		user := &proto.User{
			Id:         u.ID.String(),
			Email:      u.Email,
			FacebookId: u.FacebookID,
			GoogleId:   u.GoogleID,
		}

		users = append(users, user)
	}

	response := &proto.ListUserResponse{
		Users: users,
		Total: totalUsers,
	}

	return response, nil
}

func (s *userServer) registerCommandHandlers(r user.Repository) {
	s.commandBus.Subscribe(fmt.Sprintf("%T", &user.RegisterWithEmail{}), user.OnRegisterWithEmail(r))
	s.commandBus.Subscribe(fmt.Sprintf("%T", &user.RegisterWithGoogle{}), user.OnRegisterWithGoogle(r))
	s.commandBus.Subscribe(fmt.Sprintf("%T", &user.RegisterWithFacebook{}), user.OnRegisterWithFacebook(r))
	s.commandBus.Subscribe(fmt.Sprintf("%T", &user.ChangeEmailAddress{}), user.OnChangeEmailAddress(r))
}

func (s *userServer) registerEventHandlers(r mysql.UserRepository) {
	s.eventBus.Subscribe(fmt.Sprintf("%T", &user.WasRegisteredWithEmail{}), application.WhenUserWasRegisteredWithEmail(s.db, r))
	s.eventBus.Subscribe(fmt.Sprintf("%T", &user.WasRegisteredWithGoogle{}), application.WhenUserWasRegisteredWithGoogle(s.db, r))
	s.eventBus.Subscribe(fmt.Sprintf("%T", &user.WasRegisteredWithFacebook{}), application.WhenUserWasRegisteredWithFacebook(s.db, r))
	s.eventBus.Subscribe(fmt.Sprintf("%T", &user.EmailAddressWasChanged{}), application.WhenUserEmailAddressWasChanged(s.db, r))
}
