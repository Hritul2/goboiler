package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hritul2/goboiler/internal/api"
	errs "github.com/hritul2/goboiler/internal/errors"
	"github.com/hritul2/goboiler/internal/middleware"
)

type greetReq struct {
	Name string `json:"name" validate:"required,min=2"`
	Age  int    `json:"age"  validate:"omitempty,min=0"`
}

type greetRes struct {
	Message string `json:"message"`
}

func greet(_ context.Context, req greetReq) (greetRes, error) {
	if req.Name == "ghost" {
		return greetRes{}, errs.NotFound("no such person")
	}
	return greetRes{Message: "hello " + req.Name}, nil
}

func ping(_ context.Context, _ api.Empty) (api.Empty, error) { return api.Empty{}, nil }

// newEngine mounts a version with the same two middlewares that matter to
// the adapter in production: the global body-size guard (outer) and
// ErrorHandler (innermost).
func newEngine(t *testing.T, maxBody int64, eps ...api.Endpoint) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(middleware.MaxReqResSize(maxBody, 0), middleware.ErrorHandler())
	api.Mount(engine, api.Version{Name: "v1", Prefix: "/api/v1/rpc", Endpoints: eps})
	return engine
}

func post(engine *gin.Engine, path, body string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	engine.ServeHTTP(w, req)
	return w
}

func errBody(t *testing.T, w *httptest.ResponseRecorder) errs.ErrorResponse {
	t.Helper()
	var e errs.ErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &e), w.Body.String())
	return e
}

func TestAdapter_Success(t *testing.T) {
	engine := newEngine(t, 1<<20, api.Query("greet", greet))

	w := post(engine, "/api/v1/rpc/greet", `{"name":"Ada","age":36}`)

	require.Equal(t, http.StatusOK, w.Code)
	assert.JSONEq(t, `{"message":"hello Ada"}`, w.Body.String())
}

func TestAdapter_MalformedJSON_400(t *testing.T) {
	engine := newEngine(t, 1<<20, api.Mutation("greet", greet))

	for name, body := range map[string]string{
		"truncated":     `{"name":`,
		"garbage":       `not json`,
		"trailing data": `{"name":"Ada"} {"x":1}`,
		"wrong type":    `{"name":123}`,
		"not an object": `[1,2]`,
	} {
		t.Run(name, func(t *testing.T) {
			w := post(engine, "/api/v1/rpc/greet", body)
			require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
			assert.Equal(t, errs.CodeBadRequest, errBody(t, w).Code)
		})
	}
}

func TestAdapter_ValidationFailure_400WithFieldMessages(t *testing.T) {
	engine := newEngine(t, 1<<20, api.Mutation("greet", greet))

	w := post(engine, "/api/v1/rpc/greet", `{"name":"A"}`)

	require.Equal(t, http.StatusBadRequest, w.Code)
	e := errBody(t, w)
	assert.Equal(t, errs.CodeBadRequest, e.Code)
	assert.Contains(t, e.Message, "name")
	assert.Contains(t, e.Message, "at least 2")
}

type signupReq struct {
	Email   string     `json:"email"   validate:"required,email"`
	Age     int        `json:"age"     validate:"min=18"`
	Name    string     `json:"name"    validate:"min=2"`
	Address addressReq `json:"address"`
	Tags    []string   `json:"tags"    validate:"min=1"`
	NoTag   string     `validate:"required"`
}

type addressReq struct {
	City string `json:"city" validate:"required"`
}

func signup(_ context.Context, _ signupReq) (api.Empty, error) { return api.Empty{}, nil }

func TestAdapter_ValidationFailure_FieldsKeyedByJSONName(t *testing.T) {
	engine := newEngine(t, 1<<20, api.Mutation("signup", signup))

	w := post(engine, "/api/v1/rpc/signup", `{"email":"nope","age":3,"name":"A","tags":[]}`)

	require.Equal(t, http.StatusBadRequest, w.Code)
	e := errBody(t, w)
	assert.Equal(t, errs.CodeBadRequest, e.Code)
	assert.Equal(t, map[string][]string{
		"email":        {"must be a valid email address"},
		"age":          {"must be at least 18"},           // number: no "characters"
		"name":         {"must be at least 2 characters"}, // string
		"tags":         {"must be at least 1 item"},       // slice
		"address.city": {"is required"},                   // nested => dotted JSON path
		"noTag":        {"is required"},                   // no json tag => lowerCamel
	}, e.Fields)

	// The wire key is literally "fields" (what the generated ApiError reads).
	var raw map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &raw))
	assert.Contains(t, raw, "fields")
}

func TestAdapter_OtherErrors_HaveNoFieldsKey(t *testing.T) {
	engine := newEngine(t, 1<<20, api.Query("greet", greet))

	for name, body := range map[string]string{
		"not found":      `{"name":"ghost"}`,
		"malformed json": `{"name":`,
	} {
		t.Run(name, func(t *testing.T) {
			w := post(engine, "/api/v1/rpc/greet", body)
			var raw map[string]json.RawMessage
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &raw))
			assert.NotContains(t, raw, "fields")
		})
	}
}

func TestAdapter_EmptyBodyIsEmptyObject(t *testing.T) {
	engine := newEngine(t, 1<<20,
		api.Query("ping", ping),
		api.Query("greet", greet),
	)

	// api.Empty: no body at all is fine.
	w := post(engine, "/api/v1/rpc/ping", "")
	require.Equal(t, http.StatusOK, w.Code)
	assert.JSONEq(t, `{}`, w.Body.String())

	// An empty body is {} — which then fails *validation* (name required),
	// proving it decoded to the zero value rather than erroring in decode.
	w = post(engine, "/api/v1/rpc/greet", "   ")
	require.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, errBody(t, w).Message, "name: is required")
}

func TestAdapter_ServiceErrorMapsStatus(t *testing.T) {
	engine := newEngine(t, 1<<20, api.Query("greet", greet))

	w := post(engine, "/api/v1/rpc/greet", `{"name":"ghost"}`)

	require.Equal(t, http.StatusNotFound, w.Code)
	e := errBody(t, w)
	assert.Equal(t, errs.CodeNotFound, e.Code)
	assert.Equal(t, "no such person", e.Message)
}

func TestAdapter_OversizedBody_413(t *testing.T) {
	engine := newEngine(t, 16, api.Mutation("greet", greet))

	w := post(engine, "/api/v1/rpc/greet", `{"name":"`+strings.Repeat("x", 200)+`"}`)

	require.Equal(t, http.StatusRequestEntityTooLarge, w.Code, w.Body.String())
	assert.Equal(t, errs.CodePayloadTooLarge, errBody(t, w).Code)
}

func TestAdapter_OnlyPOST(t *testing.T) {
	engine := newEngine(t, 1<<20, api.Query("greet", greet))

	w := httptest.NewRecorder()
	engine.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/rpc/greet", nil))

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestMount_EndpointMiddlewareRunsFirst(t *testing.T) {
	var order []string
	mw := func(c *gin.Context) { order = append(order, "mw"); c.Next() }
	fn := func(_ context.Context, _ api.Empty) (api.Empty, error) {
		order = append(order, "fn")
		return api.Empty{}, nil
	}
	engine := newEngine(t, 1<<20, api.Query("ping", fn, api.WithMiddleware(mw)))

	post(engine, "/api/v1/rpc/ping", "")

	assert.Equal(t, []string{"mw", "fn"}, order)
}

func TestMount_EndpointMiddlewareCanReject(t *testing.T) {
	deny := func(c *gin.Context) {
		_ = c.Error(errs.Unauthorized("no token"))
		c.Abort()
	}
	engine := newEngine(t, 1<<20, api.Query("ping", ping, api.WithMiddleware(deny)))

	w := post(engine, "/api/v1/rpc/ping", "")

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestEndpoint_Metadata(t *testing.T) {
	q := api.Query("greet", greet, api.WithDescription("says hi"))
	m := api.Mutation("greet2", greet)

	assert.Equal(t, api.KindQuery, q.Kind)
	assert.Equal(t, api.KindMutation, m.Kind)
	assert.Equal(t, "greetReq", q.Req.Name())
	assert.Equal(t, "greetRes", q.Res.Name())
	assert.Equal(t, "says hi", q.Description)
	assert.Equal(t, "query", q.Kind.String())
}

type notStructReq = string
type Generic[T any] struct{ V T }

func TestMount_PanicsOnInvalidVersion(t *testing.T) {
	noop := func(_ context.Context, _ api.Empty) (api.Empty, error) { return api.Empty{}, nil }
	strFn := func(_ context.Context, _ notStructReq) (api.Empty, error) { return api.Empty{}, nil }
	anonFn := func(_ context.Context, _ struct{ A int }) (api.Empty, error) { return api.Empty{}, nil }
	genFn := func(_ context.Context, _ Generic[int]) (api.Empty, error) { return api.Empty{}, nil }
	ptrFn := func(_ context.Context, _ *greetReq) (api.Empty, error) { return api.Empty{}, nil }

	tests := []struct {
		name    string
		version api.Version
		want    string
	}{
		{"duplicate name", api.Version{Name: "v1", Prefix: "/rpc", Endpoints: []api.Endpoint{
			api.Query("a", noop), api.Mutation("a", noop)}}, "duplicate endpoint name"},
		{"not lowerCamel: upper", api.Version{Name: "v1", Prefix: "/rpc", Endpoints: []api.Endpoint{
			api.Query("GetUser", noop)}}, "lowerCamel"},
		{"not lowerCamel: dash", api.Version{Name: "v1", Prefix: "/rpc", Endpoints: []api.Endpoint{
			api.Query("get-user", noop)}}, "lowerCamel"},
		{"empty name", api.Version{Name: "v1", Prefix: "/rpc", Endpoints: []api.Endpoint{
			api.Query("", noop)}}, "lowerCamel"},
		{"non-struct request", api.Version{Name: "v1", Prefix: "/rpc", Endpoints: []api.Endpoint{
			api.Query("a", strFn)}}, "must be a struct"},
		{"pointer request", api.Version{Name: "v1", Prefix: "/rpc", Endpoints: []api.Endpoint{
			api.Query("a", ptrFn)}}, "must be a struct"},
		{"anonymous struct request", api.Version{Name: "v1", Prefix: "/rpc", Endpoints: []api.Endpoint{
			api.Query("a", anonFn)}}, "named struct"},
		{"generic request", api.Version{Name: "v1", Prefix: "/rpc", Endpoints: []api.Endpoint{
			api.Query("a", genFn)}}, "generic"},
		{"hand-built endpoint", api.Version{Name: "v1", Prefix: "/rpc", Endpoints: []api.Endpoint{
			{Name: "a"}}}, "api.Query or api.Mutation"},
		{"bad version name", api.Version{Name: "V1", Prefix: "/rpc"}, "version name"},
		{"bad prefix", api.Version{Name: "v1", Prefix: "rpc"}, "prefix"},
		{"trailing slash prefix", api.Version{Name: "v1", Prefix: "/rpc/"}, "prefix"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			var recovered any
			func() {
				defer func() { recovered = recover() }()
				api.Mount(gin.New(), tt.version)
			}()
			require.NotNil(t, recovered, "Mount must panic")
			err, ok := recovered.(error)
			require.True(t, ok, "panic value should be an error, got %T", recovered)
			assert.Contains(t, err.Error(), tt.want)
		})
	}
}

func TestVersion_Validate_ValidPasses(t *testing.T) {
	v := api.Version{Name: "v1", Prefix: "/api/v1/rpc", Endpoints: []api.Endpoint{
		api.Query("greet", greet), api.Mutation("ping", ping),
	}}
	assert.NoError(t, v.Validate())
	assert.Equal(t, "/api/v1/rpc/greet", v.Path(v.Endpoints[0]))
}

// Empty here is a different Go type with the same short name as api.Empty.
// Both would become `EmptySchema` in one zod file, so Validate must refuse.
type Empty struct{}

func TestVersion_Validate_RejectsSameNameDifferentType(t *testing.T) {
	a := func(_ context.Context, _ api.Empty) (api.Empty, error) { return api.Empty{}, nil }
	b := func(_ context.Context, _ Empty) (api.Empty, error) { return api.Empty{}, nil }
	v := api.Version{Name: "v1", Prefix: "/rpc", Endpoints: []api.Endpoint{api.Query("a", a), api.Query("b", b)}}

	err := v.Validate()

	require.Error(t, err)
	assert.Contains(t, err.Error(), `type name "Empty" is used by both`)
}

// A version with no endpoints is valid: it is the state of a fresh project.
// It validates, mounts without panicking, registers no routes, and any path
// under its prefix is a plain 404.
func TestEmptyVersion_ValidatesAndMountsNoRoutes(t *testing.T) {
	v := api.Version{Name: "v1", Prefix: "/api/v1/rpc"}
	require.NoError(t, v.Validate())
	require.NoError(t, api.Version{Name: "v1", Prefix: "/api/v1/rpc", Endpoints: []api.Endpoint{}}.Validate())

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(middleware.ErrorHandler())
	require.NotPanics(t, func() { api.Mount(engine, v) })

	assert.Empty(t, engine.Routes())
	assert.Equal(t, http.StatusNotFound, post(engine, "/api/v1/rpc/anything", `{}`).Code)
}
