package user

import (
	"context"

	"github.com/rs/zerolog"

	"github.com/hritul2/goboiler/internal/api"
	"github.com/hritul2/goboiler/internal/models"
	"github.com/hritul2/goboiler/internal/validation"
)

// userService is the narrow slice of *usersvc.UserService this package
// needs. Depending on an interface (not the concrete type) keeps the typed
// functions unit-testable with a fake, and keeps this package from caring
// how the service is built. *user.UserService satisfies it structurally.
type userService interface {
	Create(ctx context.Context, in validation.UserCreateInput) (*models.User, error)
	GetByID(ctx context.Context, id models.ID) (*models.User, error)
	Update(ctx context.Context, id models.ID, in validation.UserUpdateInput) (*models.User, error)
	Delete(ctx context.Context, id models.ID) error
	List(ctx context.Context, p models.Pagination) ([]models.User, error)
}

// Handler holds the typed User RPC functions. Each method has the exact
// signature api.Query/api.Mutation expect —
// func(context.Context, Req) (Res, error) — so it knows nothing about gin,
// HTTP or JSON: decoding, validation and error rendering all happen in the
// api adapter.
type Handler struct {
	svc userService
	log zerolog.Logger
}

// NewHandler constructs a Handler. svc is constructor-injected (no global
// lookups), matching HealthHandler and UserService.
func NewHandler(svc userService, log zerolog.Logger) *Handler {
	return &Handler{svc: svc, log: log}
}

// CreateUser registers a new account. Field-level rules were already checked
// by the adapter; the service validates again, hashes the password and maps
// a duplicate email to 409.
func (h *Handler) CreateUser(ctx context.Context, req CreateUserReq) (UserRes, error) {
	u, err := h.svc.Create(ctx, validation.UserCreateInput{
		Email:    req.Email,
		Name:     req.Name,
		Password: req.Password,
	})
	if err != nil {
		return UserRes{}, err
	}
	return toUserRes(u), nil
}

// GetUser returns one user, or a 404 from the service when it doesn't exist.
func (h *Handler) GetUser(ctx context.Context, req GetUserReq) (UserRes, error) {
	u, err := h.svc.GetByID(ctx, models.ID(req.UserID))
	if err != nil {
		return UserRes{}, err
	}
	return toUserRes(u), nil
}

// ListUsers returns one page of users. Page/limit are normalised here (not
// only in the repo) so the response can echo the values actually used.
func (h *Handler) ListUsers(ctx context.Context, req ListUsersReq) (ListUsersRes, error) {
	p := models.Pagination{Page: req.Page, Limit: req.Limit}
	p.Normalize()

	users, err := h.svc.List(ctx, p)
	if err != nil {
		return ListUsersRes{}, err
	}

	// Always a non-nil slice: a nil slice marshals to JSON null, which the
	// generated zod array schema would reject.
	items := make([]UserRes, 0, len(users))
	for i := range users {
		items = append(items, toUserRes(&users[i]))
	}
	return ListUsersRes{Items: items, Page: p.Page, Limit: p.Limit}, nil
}

// UpdateUser renames a user and returns the updated record.
func (h *Handler) UpdateUser(ctx context.Context, req UpdateUserReq) (UserRes, error) {
	u, err := h.svc.Update(ctx, models.ID(req.UserID), validation.UserUpdateInput{Name: req.Name})
	if err != nil {
		return UserRes{}, err
	}
	return toUserRes(u), nil
}

// DeleteUser removes a user. It returns api.Empty (rendered as `{}`) since
// there is nothing useful to send back.
func (h *Handler) DeleteUser(ctx context.Context, req GetUserReq) (api.Empty, error) {
	if err := h.svc.Delete(ctx, models.ID(req.UserID)); err != nil {
		return api.Empty{}, err
	}
	return api.Empty{}, nil
}

// toUserRes is the single place a models.User becomes a wire value. Adding a
// field to the response is a deliberate edit here and in UserRes.
func toUserRes(u *models.User) UserRes {
	return UserRes{
		ID:        u.ID.String(),
		Email:     u.Email,
		Name:      u.Name,
		CreatedAt: u.CreatedAt,
		UpdatedAt: u.UpdatedAt,
	}
}
