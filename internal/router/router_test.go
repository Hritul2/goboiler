package router

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hritul2/goboiler/internal/api"
	"github.com/hritul2/goboiler/internal/gen"
	"github.com/hritul2/goboiler/internal/handler"
	"github.com/hritul2/goboiler/internal/services"
	"github.com/hritul2/goboiler/internal/utils/config"
)

type okPinger struct{}

func (okPinger) Ping(context.Context) error { return nil }

func testConfig() *config.Config {
	return &config.Config{
		App: config.AppConfig{Name: "router-test", Env: "dev"},
		Server: config.ServerConfig{
			CORSAllowedOrigins:   []string{"*"},
			MaxRequestBodyBytes:  1 << 20,
			MaxResponseBodyBytes: 0,
			RateLimitPerMinute:   6000,
			RateLimitBurst:       1000,
		},
	}
}

// newTestEngine builds the real engine (real middleware stack, the real —
// currently empty — versions registry).
func newTestEngine() (*gin.Engine, *handler.Handlers) {
	h := handler.New(okPinger{}, okPinger{}, &services.Services{}, zerolog.Nop())
	return New(testConfig(), zerolog.Nop(), h), h
}

func routeSet(engine *gin.Engine) map[string]bool {
	set := map[string]bool{}
	for _, r := range engine.Routes() {
		set[r.Method+" "+r.Path] = true
	}
	return set
}

func do(engine *gin.Engine, method, path, body string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, httptest.NewRequest(method, path, strings.NewReader(body)))
	return w
}

// The router tests use their own fixture endpoints, mounted onto the real
// engine after New(), so they keep working whatever domains a project adds.
type greetReq struct {
	Name string `json:"name" validate:"required,min=2"`
}

type greetRes struct {
	Message string `json:"message"`
}

func fixtureVersion(name string) api.Version {
	greet := func(_ context.Context, req greetReq) (greetRes, error) {
		return greetRes{Message: "hello " + req.Name}, nil
	}
	return api.Version{
		Name:      name,
		Prefix:    "/api/" + name + "/rpc",
		Endpoints: []api.Endpoint{api.Query("greet", greet)},
	}
}

func TestNew_MountsPingAndEveryRegisteredEndpoint(t *testing.T) {
	engine, h := newTestEngine()
	routes := routeSet(engine)

	assert.True(t, routes["GET /ping"])
	for _, v := range APIVersions(h) {
		for _, e := range v.Endpoints {
			assert.True(t, routes["POST "+v.Path(e)], "missing POST %s", v.Path(e))
		}
	}
}

func TestNew_RPCIsPostOnly(t *testing.T) {
	engine, _ := newTestEngine()
	api.Mount(engine, fixtureVersion("v9"))

	for _, method := range []string{http.MethodGet, http.MethodDelete, http.MethodPatch} {
		w := do(engine, method, "/api/v9/rpc/greet", "")
		assert.Equal(t, http.StatusNotFound, w.Code, method)
	}
	w := do(engine, http.MethodPost, "/api/v9/rpc/greet", `{"name":"Ada"}`)
	assert.Equal(t, http.StatusOK, w.Code, w.Body.String())
}

func TestNew_RPCGoesThroughMiddlewareAndAdapter(t *testing.T) {
	engine, _ := newTestEngine()
	api.Mount(engine, fixtureVersion("v9"))

	// A missing field is caught by the DTO's `required` validate tag before
	// the function runs, and rendered by the real ErrorHandler with the
	// per-field `fields` map.
	w := do(engine, http.MethodPost, "/api/v9/rpc/greet", `{}`)

	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), `"code":"BAD_REQUEST"`)
	assert.Contains(t, w.Body.String(), `"fields":{"name":["is required"]}`)
	assert.NotEmpty(t, w.Header().Get("X-Request-Id"), "global middleware stack must apply to RPC routes")
}

func TestNew_WebhookGroupHasNoRoutesYet(t *testing.T) {
	engine, _ := newTestEngine()

	for path := range routeSet(engine) {
		assert.False(t, strings.Contains(path, "/webhook"), "unexpected webhook route %s", path)
	}
}

// TestVersionsAndWebhooksAreIndependent proves the property the layout is
// built for: a second API version and a webhook route can be added to the
// engine later without touching (or being affected by) each other or /ping.
func TestVersionsAndWebhooksAreIndependent(t *testing.T) {
	engine, _ := newTestEngine()

	api.Mount(engine, fixtureVersion("v2"))
	engine.Group("/webhook").POST("/clerk", func(c *gin.Context) {
		raw, _ := c.GetRawData()
		c.String(http.StatusOK, "clerk:"+string(raw))
	})

	// v2 works.
	w := do(engine, http.MethodPost, "/api/v2/rpc/greet", `{"name":"Ada"}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.JSONEq(t, `{"message":"hello Ada"}`, w.Body.String())

	// The webhook sees the RAW body untouched (no JSON adapter involved).
	w = do(engine, http.MethodPost, "/webhook/clerk", `{"raw":  "payload"}`)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, `clerk:{"raw":  "payload"}`, w.Body.String())

	// v2's functions do not leak into v1, and /ping is unaffected.
	w = do(engine, http.MethodPost, "/api/v1/rpc/greet", `{"name":"Ada"}`)
	assert.Equal(t, http.StatusNotFound, w.Code)
	w = do(engine, http.MethodGet, "/ping", "")
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestAPIVersions_AreValid(t *testing.T) {
	h := handler.New(okPinger{}, okPinger{}, &services.Services{}, zerolog.Nop())
	versions := APIVersions(h)

	require.NotEmpty(t, versions)
	names := map[string]bool{}
	prefixes := map[string]bool{}
	for _, v := range versions {
		assert.NoError(t, v.Validate(), v.Name)
		assert.False(t, names[v.Name], "duplicate version name %s", v.Name)
		assert.False(t, prefixes[v.Prefix], "duplicate prefix %s", v.Prefix)
		names[v.Name], prefixes[v.Prefix] = true, true
	}
	assert.Equal(t, "/api/v1/rpc", versions[0].Prefix)
}

// A version with no endpoints — the state of v1 in a fresh project — must be
// fully valid: it mounts without panicking, adds no routes, and the generator
// produces a (zero-function) client in both client kinds, because `make gen`
// and `make check` run before any domain is added.
func TestEmptyVersionIsValidMountableAndGenerates(t *testing.T) {
	empty := api.Version{Name: "v1", Prefix: "/api/v1/rpc"}
	require.NoError(t, empty.Validate())

	engine := gin.New()
	require.NotPanics(t, func() { api.Mount(engine, empty) })
	assert.Empty(t, engine.Routes())

	for _, kind := range []gen.ClientKind{gen.ClientFetch, gen.ClientAxios} {
		files, err := gen.Generate([]api.Version{empty}, gen.Options{Client: kind})
		require.NoError(t, err, kind)
		for _, name := range []string{"schemas.ts", "client.ts", "hooks.ts", "index.ts"} {
			assert.NotEmpty(t, files["v1/"+name], "%s %s", kind, name)
		}
	}
}

// Whatever APIVersions currently registers must generate in both client kinds.
func TestAPIVersions_GenerateInBothClientKinds(t *testing.T) {
	h := handler.New(okPinger{}, okPinger{}, &services.Services{}, zerolog.Nop())
	for _, kind := range []gen.ClientKind{gen.ClientFetch, gen.ClientAxios} {
		_, err := gen.Generate(APIVersions(h), gen.Options{Client: kind})
		require.NoError(t, err, kind)
	}
}
