// Package userv2 is the /api/v2 surface of the User feature. It reuses the
// v1 handler for everything that did not change and replaces only getUser,
// whose response gains a different shape.
package userv2

import (
	"context"
	"slices"
	"time"

	"github.com/hritul2/goboiler/internal/api"
	"github.com/hritul2/goboiler/internal/handler/user"
)

// GetUserRes is v2's getUser response: "name" is renamed "displayName".
// A new type name (not UserRes) keeps the generated v2 schemas unambiguous.
type GetUserRes struct {
	ID          string    `json:"id"`
	Email       string    `json:"email"`
	DisplayName string    `json:"displayName"`
	CreatedAt   time.Time `json:"createdAt"`
}

// Endpoints is v2's function list: v2's getUser first, then every other v1
// function unchanged (same DTOs, same handlers).
func Endpoints(v1 *user.Handler) []api.Endpoint {
	getUser := func(ctx context.Context, req user.GetUserReq) (GetUserRes, error) {
		u, err := v1.GetUser(ctx, req) // delegate, then reshape
		if err != nil {
			return GetUserRes{}, err
		}
		return GetUserRes{ID: u.ID, Email: u.Email, DisplayName: u.Name, CreatedAt: u.CreatedAt}, nil
	}

	return slices.Concat(
		[]api.Endpoint{
			api.Query("getUser", getUser,
				api.WithDescription("Fetches one user by id (v2: name is returned as displayName).")),
		},
		slices.DeleteFunc(user.Endpoints(v1), func(e api.Endpoint) bool { return e.Name == "getUser" }),
	)
}
