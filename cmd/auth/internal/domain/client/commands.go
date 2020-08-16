package client

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"runtime/debug"

	"github.com/google/uuid"
	"gopkg.in/oauth2.v4"

	"github.com/vardius/go-api-boilerplate/pkg/commandbus"
	"github.com/vardius/go-api-boilerplate/pkg/errors"
	"github.com/vardius/go-api-boilerplate/pkg/executioncontext"
)

// Remove command
type Remove struct {
	ID uuid.UUID `json:"id"`
}

// GetName returns command name
func (c Remove) GetName() string {
	return fmt.Sprintf("%T", c)
}

// OnRemove creates command handler
func OnRemove(repository Repository, db *sql.DB) commandbus.CommandHandler {
	fn := func(ctx context.Context, c Remove, out chan<- error) {
		// this goroutine runs independently to request's goroutine,
		// therefore recover middleware will not recover from panic to prevent crash
		defer recoverCommandHandler(out)

		client, err := repository.Get(c.ID)
		if err != nil {
			out <- errors.Wrap(err)
			return
		}

		if err := client.Remove(); err != nil {
			out <- errors.Wrap(err)
			return
		}

		out <- repository.Save(executioncontext.WithFlag(ctx, executioncontext.LIVE), client)
	}

	return commandbus.CommandHandler(fn)
}

// Create command
type Create struct {
	ClientInfo oauth2.ClientInfo
}

// GetName returns command name
func (c Create) GetName() string {
	return fmt.Sprintf("%T", c)
}

// OnCreate creates command handler
func OnCreate(repository Repository, db *sql.DB) commandbus.CommandHandler {
	fn := func(ctx context.Context, c Create, out chan<- error) {
		// this goroutine runs independently to request's goroutine,
		// therefore recover middleware will not recover from panic to prevent crash
		defer recoverCommandHandler(out)

		client := New()
		err := client.Create(c.ClientInfo)
		if err != nil {
			out <- errors.Wrap(err)
			return
		}

		out <- repository.Save(executioncontext.WithFlag(ctx, executioncontext.LIVE), client)
	}

	return commandbus.CommandHandler(fn)
}

func recoverCommandHandler(out chan<- error) {
	if r := recover(); r != nil {
		out <- errors.Wrap(fmt.Errorf("[CommandHandler] Recovered in %v", r))

		// Log the Go stack trace for this panic'd goroutine.
		log.Printf("%s\n", debug.Stack())
	}
}
