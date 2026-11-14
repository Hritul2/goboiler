package user

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hritul2/goboiler/internal/api"
	errs "github.com/hritul2/goboiler/internal/errors"
	"github.com/hritul2/goboiler/internal/middleware"
	"github.com/hritul2/goboiler/internal/models"
	usersvc "github.com/hritul2/goboiler/internal/services/user"
	"github.com/hritul2/goboiler/internal/validation"
)

// Compile-time proof the real service satisfies the narrow interface, so the
// fake below can't drift from what production wiring passes in.
var _ userService = (*usersvc.UserService)(nil)

// fakeService records what the handler passed it and returns canned results.
type fakeService struct {
	user *models.User
	err  error

	gotID    models.ID
	gotPage  models.Pagination
	gotName  string
	gotEmail string
	deleted  models.ID
	list     []models.User
}

func (f *fakeService) Create(_ context.Context, in validation.UserCreateInput) (*models.User, error) {
	f.gotEmail = in.Email
	return f.user, f.err
}

func (f *fakeService) GetByID(_ context.Context, id models.ID) (*models.User, error) {
	f.gotID = id
	return f.user, f.err
}

func (f *fakeService) Update(_ context.Context, id models.ID, in validation.UserUpdateInput) (*models.User, error) {
	f.gotID, f.gotName = id, in.Name
	return f.user, f.err
}

func (f *fakeService) Delete(_ context.Context, id models.ID) error {
	f.deleted = id
	return f.err
}

func (f *fakeService) List(_ context.Context, p models.Pagination) ([]models.User, error) {
	f.gotPage = p
	return f.list, f.err
}

func newTestUser() *models.User {
	u := models.NewUser("ada@example.com", "Ada", "$2a$10$SUPERSECRETHASH")
	u.ID = "user-1" // any string: the handler is ID-format agnostic
	u.CreatedAt = time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	u.UpdatedAt = u.CreatedAt
	return u
}

// example:region handler-test
// newEngine mounts the real Endpoints(h) over the fake service, so these
// tests cover DTO decoding, validation, handler logic and JSON encoding.
func newEngine(svc userService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(middleware.ErrorHandler())
	api.Mount(engine, api.Version{
		Name:      "v1",
		Prefix:    "/api/v1/rpc",
		Endpoints: Endpoints(NewHandler(svc, zerolog.Nop())),
	})
	return engine
}

func call(engine *gin.Engine, fn, body string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/v1/rpc/"+fn, strings.NewReader(body)))
	return w
}

// example:endregion handler-test

func TestEndpoints_Registered(t *testing.T) {
	eps := Endpoints(NewHandler(&fakeService{}, zerolog.Nop()))

	kinds := map[string]api.Kind{}
	for _, e := range eps {
		kinds[e.Name] = e.Kind
	}
	assert.Equal(t, map[string]api.Kind{
		"createUser": api.KindMutation,
		"updateUser": api.KindMutation,
		"deleteUser": api.KindMutation,
		"getUser":    api.KindQuery,
		"listUsers":  api.KindQuery,
	}, kinds)

	v := api.Version{Name: "v1", Prefix: "/api/v1/rpc", Endpoints: eps}
	assert.NoError(t, v.Validate())
}

func TestDTOs_EveryFieldHasCamelCaseJSONTag(t *testing.T) {
	// Reflect over every registered Req/Res: a missing or PascalCase tag
	// would leak a Go field name into the generated TypeScript.
	for _, e := range Endpoints(NewHandler(&fakeService{}, zerolog.Nop())) {
		for _, typ := range []reflect.Type{e.Req, e.Res} {
			for i := range typ.NumField() {
				f := typ.Field(i)
				name, _, _ := strings.Cut(f.Tag.Get("json"), ",")
				assert.Regexp(t, `^[a-z][a-zA-Z0-9]*$`, name, "%s.%s needs a camelCase json tag", typ.Name(), f.Name)
			}
		}
	}
}

func TestCreateUser_OK_NoPasswordHashInResponse(t *testing.T) {
	svc := &fakeService{user: newTestUser()}
	engine := newEngine(svc)

	w := call(engine, "createUser", `{"email":"ada@example.com","name":"Ada","password":"correct-horse"}`)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, "ada@example.com", svc.gotEmail)

	body := w.Body.String()
	assert.NotContains(t, strings.ToLower(body), "password")
	assert.NotContains(t, body, "SUPERSECRETHASH")

	var res map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &res))
	assert.ElementsMatch(t, []string{"id", "email", "name", "createdAt", "updatedAt"}, keys(res))
	assert.Equal(t, "ada@example.com", res["email"])
	assert.Equal(t, svc.user.ID.String(), res["id"])
	assert.Equal(t, "2026-01-02T03:04:05Z", res["createdAt"])
}

// example:region handler-test
func TestCreateUser_ValidationFailure_400(t *testing.T) {
	w := call(newEngine(&fakeService{}), "createUser", `{"email":"nope","name":"A","password":"short"}`)

	require.Equal(t, http.StatusBadRequest, w.Code)

	var body errs.ErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, errs.CodeBadRequest, body.Code)
	// Keyed by the DTO's JSON names, so a form can map them to inputs.
	assert.Equal(t, map[string][]string{
		"email":    {"must be a valid email address"},
		"name":     {"must be at least 2 characters"},
		"password": {"must be at least 8 characters"},
	}, body.Fields)
}

// example:endregion handler-test

func TestCreateUser_ServiceConflict_409(t *testing.T) {
	svc := &fakeService{err: errs.Conflict("an account with this email already exists")}

	w := call(newEngine(svc), "createUser", `{"email":"ada@example.com","name":"Ada","password":"correct-horse"}`)

	assert.Equal(t, http.StatusConflict, w.Code)
}

func TestGetUser_InvalidUserID_400(t *testing.T) {
	svc := &fakeService{user: newTestUser()}
	engine := newEngine(svc)

	// Only what the DTO can judge without knowing the database: presence and
	// size (`required,max=64`).
	for name, body := range map[string]string{
		"missing":      `{}`,
		"empty string": `{"userId":""}`,
		"too long":     `{"userId":"` + strings.Repeat("a", 65) + `"}`,
	} {
		t.Run(name, func(t *testing.T) {
			w := call(engine, "getUser", body)
			require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
			assert.Contains(t, w.Body.String(), errs.CodeBadRequest)
			assert.Contains(t, w.Body.String(), `"userId"`, "error is keyed by the JSON field name")
		})
	}
	assert.True(t, svc.gotID.IsZero(), "service must not be reached with a bad id")
}

// TestGetUser_UnparseableID_PassedThrough documents the DB-agnostic contract:
// the handler no longer judges ID FORMAT. An ID the store cannot parse (not
// an ObjectID, not a UUID, ...) is handed to the service and comes back as
// the store's NOT FOUND.
func TestGetUser_UnparseableID_PassedThrough(t *testing.T) {
	svc := &fakeService{err: errs.ErrRecordNotFound}

	w := call(newEngine(svc), "getUser", `{"userId":"not-an-object-id"}`)

	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Equal(t, models.ID("not-an-object-id"), svc.gotID)
}

func TestGetUser_OK_And_NotFound(t *testing.T) {
	u := newTestUser()
	svc := &fakeService{user: u}
	engine := newEngine(svc)

	w := call(engine, "getUser", `{"userId":"`+u.ID.String()+`"}`)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, u.ID, svc.gotID)
	assert.NotContains(t, w.Body.String(), "SUPERSECRETHASH")

	svc.err = errs.NotFound("user not found")
	w = call(engine, "getUser", `{"userId":"`+u.ID.String()+`"}`)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestUpdateUser_OK(t *testing.T) {
	u := newTestUser()
	svc := &fakeService{user: u}

	w := call(newEngine(svc), "updateUser", `{"userId":"`+u.ID.String()+`","name":"Ada L"}`)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, "Ada L", svc.gotName)
	assert.Equal(t, u.ID, svc.gotID)
}

func TestDeleteUser_OK_ReturnsEmptyObject(t *testing.T) {
	id := models.ID("user-2")
	svc := &fakeService{}

	w := call(newEngine(svc), "deleteUser", `{"userId":"`+string(id)+`"}`)

	require.Equal(t, http.StatusOK, w.Code)
	assert.JSONEq(t, `{}`, w.Body.String())
	assert.Equal(t, id, svc.deleted)
}

func TestDeleteUser_MissingID_400(t *testing.T) {
	svc := &fakeService{}

	w := call(newEngine(svc), "deleteUser", `{}`)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.True(t, svc.deleted.IsZero())
}

func TestListUsers_DefaultsAndItemsNeverNull(t *testing.T) {
	svc := &fakeService{list: nil}

	w := call(newEngine(svc), "listUsers", ``)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.JSONEq(t, `{"items":[],"page":1,"limit":20}`, w.Body.String())
	assert.Equal(t, models.Pagination{Page: 1, Limit: 20}, svc.gotPage)
}

func TestListUsers_PassesPagingAndMapsItems(t *testing.T) {
	a, b := newTestUser(), newTestUser()
	svc := &fakeService{list: []models.User{*a, *b}}

	w := call(newEngine(svc), "listUsers", `{"page":2,"limit":50}`)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, models.Pagination{Page: 2, Limit: 50}, svc.gotPage)

	var res ListUsersRes
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &res))
	require.Len(t, res.Items, 2)
	assert.Equal(t, 2, res.Page)
	assert.Equal(t, 50, res.Limit)
	assert.NotContains(t, w.Body.String(), "SUPERSECRETHASH")
}

func TestListUsers_LimitOverMax_400(t *testing.T) {
	w := call(newEngine(&fakeService{}), "listUsers", `{"limit":1000}`)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func keys(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
