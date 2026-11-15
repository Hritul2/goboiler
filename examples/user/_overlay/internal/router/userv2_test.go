package router

import (
	"context"
	"net/http"
	"slices"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hritul2/goboiler/internal/api"
	"github.com/hritul2/goboiler/internal/gen"
	"github.com/hritul2/goboiler/internal/handler/user"
	"github.com/hritul2/goboiler/internal/handler/userv2"
	"github.com/hritul2/goboiler/internal/models"
	"github.com/hritul2/goboiler/internal/validation"
)

// stub satisfies user's (unexported) service interface structurally.
type stub struct{}

func (stub) Create(context.Context, validation.UserCreateInput) (*models.User, error) {
	return nil, nil
}
func (stub) GetByID(_ context.Context, id models.ID) (*models.User, error) {
	u := models.NewUser("ada@example.com", "Ada", "h")
	u.ID = id
	return u, nil
}
func (stub) Update(context.Context, models.ID, validation.UserUpdateInput) (*models.User, error) {
	return nil, nil
}
func (stub) Delete(context.Context, models.ID) error                        { return nil }
func (stub) List(context.Context, models.Pagination) ([]models.User, error) { return nil, nil }

func TestV2_ReusesV1AndReplacesGetUser(t *testing.T) {
	h1 := user.NewHandler(stub{}, zerolog.Nop())
	versions := []api.Version{
		{Name: "v1", Prefix: "/api/v1/rpc", Endpoints: user.Endpoints(h1)},
		{Name: "v2", Prefix: "/api/v2/rpc", Endpoints: userv2.Endpoints(h1)},
	}
	for _, v := range versions {
		require.NoError(t, v.Validate())
	}

	var names []string
	for _, e := range versions[1].Endpoints {
		names = append(names, e.Name)
	}
	assert.True(t, slices.Equal([]string{"getUser", "createUser", "listUsers", "updateUser", "deleteUser"}, names), names)

	// Each version generates to its own folder; the new type only exists in v2.
	files, err := gen.Generate(versions, gen.Options{})
	require.NoError(t, err)
	assert.Contains(t, string(files["v2/schemas.ts"]), "GetUserResSchema")
	assert.Contains(t, string(files["v2/client.ts"]), "/api/v2/rpc/getUser")
	assert.NotContains(t, string(files["v1/schemas.ts"]), "GetUserResSchema")

	// And it is served.
	engine, _ := newTestEngine()
	api.Mount(engine, versions[1])
	w := do(engine, http.MethodPost, "/api/v2/rpc/getUser", `{"userId":"abc"}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), `"displayName":"Ada"`)
}
