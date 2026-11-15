package user

import "github.com/hritul2/goboiler/internal/api"

// Endpoints lists the User domain's RPC functions in the order they appear
// in the generated TypeScript client. The result is spliced into a
// api.Version in internal/router/versions.go; one Handler can feed several
// versions (or be wrapped for a v2 that diverges).
//
// Kinds: reads (getUser, listUsers) are Queries => useQuery hooks; writes
// (createUser, updateUser, deleteUser) are Mutations => useMutation hooks.
//
// ⚠️ UNAUTHENTICATED until auth lands. When it does, add
// api.WithMiddleware(auth) to the endpoints that need it (or set
// api.Version.Middleware for the whole version).
func Endpoints(h *Handler) []api.Endpoint {
	return []api.Endpoint{
		api.Mutation("createUser", h.CreateUser,
			api.WithDescription("Creates a user account. Fails with CONFLICT if the email is already registered.")),
		api.Query("getUser", h.GetUser,
			api.WithDescription("Fetches one user by id. Fails with NOT_FOUND if the user does not exist.")),
		api.Query("listUsers", h.ListUsers,
			api.WithDescription("Lists users one page at a time. page defaults to 1 and limit to 20 (max 100).")),
		api.Mutation("updateUser", h.UpdateUser,
			api.WithDescription("Updates a user's name and returns the updated user.")),
		api.Mutation("deleteUser", h.DeleteUser,
			api.WithDescription("Permanently deletes a user by id.")),
	}
}
