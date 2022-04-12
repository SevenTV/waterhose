package instance

import (
	"context"

	"github.com/nicklaw5/helix"
)

type Twitch interface {
	GetOAuth(ctx context.Context, id string) (helix.AccessCredentials, error)
	GetUser(id string) (helix.User, error)
}
