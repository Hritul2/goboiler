// Package user exposes the User domain over the typed-RPC layer
// (internal/api): request/response DTOs (dto.go), the typed functions that
// translate between DTOs and the UserService (handler.go), and the list of
// endpoints registered into a version (endpoints.go).
//
// ⚠️ These endpoints are UNAUTHENTICATED until auth lands: anyone who can
// reach the server can create, read, update and delete any user. When auth
// exists, attach it per endpoint with api.WithMiddleware (or version-wide
// via api.Version.Middleware) — do not bake auth into the typed functions.
package user

import "time"

// DTO RULES (enforced by convention and documented in docs/API_HANDLERS.md):
//
//   - Every field has an explicit camelCase `json` tag. The tag IS the wire
//     contract AND the generated zod/TypeScript property name; a missing tag
//     would leak Go's PascalCase field name to the frontend.
//   - Inputs carry `validate` tags. They are enforced server-side by
//     validation.Validate in the api adapter AND translated to zod checks by
//     the generator, so client and server validate identically.
//   - IDs are plain strings (`validate:"required,max=64"`). The wire format
//     says nothing about how the database formats IDs (ObjectID hex, UUID,
//     a Clerk "user_..." string): the DTO only bounds the size, the handler
//     passes the string through as a models.ID, and the repository decides
//     whether it exists — an ID the store cannot parse is simply NOT FOUND.
//   - Response DTOs are separate from models.User on purpose, so a new
//     model field (or PasswordHash) can never reach a client by accident.

// CreateUserReq is the input of createUser. The validate rules mirror
// validation.UserCreateInput (the service re-validates; keeping the two in
// sync means the client gets the same rules the service enforces, one layer
// earlier).
type CreateUserReq struct {
	Email    string `json:"email"    validate:"required,email"`
	Name     string `json:"name"     validate:"required,min=2,max=100"`
	Password string `json:"password" validate:"required,min=8,max=72"`
}

// GetUserReq is the input of getUser and deleteUser: just the user's ID.
type GetUserReq struct {
	UserID string `json:"userId" validate:"required,max=64"`
}

// ListUsersReq is the input of listUsers. Both fields are optional;
// zero/omitted values fall back to models.DefaultPage / DefaultLimit
// (models.Pagination.Normalize), so `{}` is a valid request.
type ListUsersReq struct {
	Page  int `json:"page,omitempty"  validate:"omitempty,min=1"`
	Limit int `json:"limit,omitempty" validate:"omitempty,min=1,max=100"`
}

// UpdateUserReq is the input of updateUser. Only Name is updatable today
// (see validation.UserUpdateInput).
type UpdateUserReq struct {
	UserID string `json:"userId" validate:"required,max=64"`
	Name   string `json:"name"   validate:"required,min=2,max=100"`
}

// UserRes is the public representation of a user. There is deliberately no
// password-hash field: it cannot be serialised because it does not exist
// here.
type UserRes struct {
	ID        string    `json:"id"`
	Email     string    `json:"email"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// ListUsersRes is one page of users plus the effective (normalised) paging
// parameters that produced it. Items carries `validate:"required"` purely
// for the generator: a Go slice can be nil (JSON null), so zen emits
// `.nullable()` for it unless the tag says it is never nil. The handler
// guarantees a non-nil slice, so the TypeScript type is UserRes[] instead of
// UserRes[] | null. (Response structs are never run through the validator.)
type ListUsersRes struct {
	Items []UserRes `json:"items" validate:"required"`
	Page  int       `json:"page"`
	Limit int       `json:"limit"`
}
