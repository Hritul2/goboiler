package gen

import (
	"context"
	"flag"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hritul2/goboiler/internal/api"
	errs "github.com/hritul2/goboiler/internal/errors"
)

// update rewrites the golden files: go test ./internal/gen -update
var update = flag.Bool("update", false, "rewrite golden files in testdata/")

// --- fixture API: small, but touches every template branch -----------------

type Address struct {
	Street string `json:"street" validate:"required"`
	Zip    string `json:"zip,omitempty"`
}

type CreateWidgetReq struct {
	Name    string   `json:"name"    validate:"required,min=2,max=50"`
	OwnerID string   `json:"ownerId"`
	Tags    []string `json:"tags"    validate:"required"`
	Address Address  `json:"address"`
	Note    *string  `json:"note"`
}

type WidgetRes struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"createdAt"`
}

type GetWidgetReq struct {
	WidgetID string `json:"widgetId" validate:"required,max=64"`
}

type ListWidgetsRes struct {
	Items []WidgetRes `json:"items" validate:"required"`
}

func nop[Req, Res any](context.Context, Req) (Res, error) {
	var r Res
	return r, nil
}

func fixtureVersion(name string) api.Version {
	return api.Version{
		Name:   name,
		Prefix: "/api/" + name + "/rpc",
		Endpoints: []api.Endpoint{
			api.Mutation("createWidget", nop[CreateWidgetReq, WidgetRes],
				api.WithDescription("Creates a widget.\nSecond line with a */ terminator.")),
			api.Query("getWidget", nop[GetWidgetReq, WidgetRes]),
			api.Query("listWidgets", nop[api.Empty, ListWidgetsRes],
				api.WithDescription("Lists every widget; takes no input.")),
			api.Mutation("resetWidgets", nop[api.Empty, api.Empty]),
		},
	}
}

// --- tests -----------------------------------------------------------------

func TestGenerate_Golden(t *testing.T) {
	for _, kind := range []ClientKind{ClientFetch, ClientAxios} {
		t.Run(string(kind), func(t *testing.T) {
			files, err := Generate([]api.Version{fixtureVersion("fixture")}, Options{Client: kind})
			require.NoError(t, err)

			require.Len(t, files, 4)
			for _, name := range fileNames {
				got, ok := files["fixture/"+name]
				require.True(t, ok, "missing fixture/%s", name)

				// schemas/hooks/index are identical between kinds, so they
				// share one golden; only client.ts has a per-kind golden
				// (fetch keeps the original name).
				golden := filepath.Join("testdata", "fixture_"+name+".golden")
				if name == "client.ts" && kind == ClientAxios {
					golden = filepath.Join("testdata", "fixture_client.axios.ts.golden")
				}
				if *update {
					require.NoError(t, os.MkdirAll("testdata", 0o755))
					require.NoError(t, os.WriteFile(golden, got, 0o644))
					continue
				}
				want, err := os.ReadFile(golden)
				require.NoError(t, err, "run `go test ./internal/gen -update` to create golden files")
				assert.Equal(t, string(want), string(got), "%s (%s) differs from %s (rerun with -update if the change is intended)", name, kind, golden)
			}
		})
	}
}

// TestGenerate_ClientKindsOnlyDifferInClientTS pins the contract of the
// --axios flag: schemas.ts, hooks.ts and index.ts are byte-identical, and
// client.ts keeps the same public surface.
func TestGenerate_ClientKindsOnlyDifferInClientTS(t *testing.T) {
	versions := []api.Version{fixtureVersion("v1")}
	fetch, err := Generate(versions, Options{Client: ClientFetch})
	require.NoError(t, err)
	axios, err := Generate(versions, Options{Client: ClientAxios})
	require.NoError(t, err)

	for _, name := range []string{"schemas.ts", "hooks.ts", "index.ts"} {
		assert.Equal(t, string(fetch["v1/"+name]), string(axios["v1/"+name]), "%s must not depend on the client kind", name)
	}
	assert.NotEqual(t, string(fetch["v1/client.ts"]), string(axios["v1/client.ts"]))

	f, a := string(fetch["v1/client.ts"]), string(axios["v1/client.ts"])
	for _, shared := range []string{
		"export function createClient(options: ClientOptions)",
		"export class ApiError extends Error",
		"readonly fields?: Record<string, string[]> | undefined",
		"export type Client = ReturnType<typeof createClient>",
		"createWidget: (input: CreateWidgetReq): Promise<WidgetRes> =>",
		`call("/api/v1/rpc/createWidget", input, WidgetResSchema)`,
		"listWidgets: (input: Empty = {}): Promise<ListWidgetsRes> =>",
		"schema.parse(",
		"ApiErrorSchema.safeParse(",
		`new ApiError(0, "NETWORK_ERROR"`, // both clients report "no response" the same way
	} {
		assert.Contains(t, f, shared, "fetch")
		assert.Contains(t, a, shared, "axios")
	}
}

func TestGenerate_ClientHeaderNamesKindAndRegenCommand(t *testing.T) {
	versions := []api.Version{fixtureVersion("v1")}
	fetch, err := Generate(versions, Options{})
	require.NoError(t, err)
	axios, err := Generate(versions, Options{Client: ClientAxios})
	require.NoError(t, err)

	f, a := string(fetch["v1/client.ts"]), string(axios["v1/client.ts"])
	assert.Contains(t, f, "// Regenerate with: make gen\n")
	assert.Contains(t, f, "Typed fetch client")
	assert.Contains(t, f, "Client kind: fetch")
	assert.Contains(t, a, "// Regenerate with: make gen GEN_FLAGS=--axios\n")
	assert.Contains(t, a, "Typed axios client")
	assert.Contains(t, a, "Client kind: axios")
}

func TestGenerate_AxiosClientBehaviour(t *testing.T) {
	files, err := Generate([]api.Version{fixtureVersion("v1")}, Options{Client: ClientAxios})
	require.NoError(t, err)
	c := string(files["v1/client.ts"])

	assert.Contains(t, c, `import axios from "axios"`)
	assert.NotContains(t, c, "doFetch")
	assert.NotContains(t, c, "baseUrl", "axios option is spelled baseURL")
	assert.Contains(t, c, "baseURL: string", "baseURL or axios must be supplied")
	assert.Contains(t, c, "axios: AxiosInstance")
	assert.Contains(t, c, `method: "POST"`)
	assert.Contains(t, c, "validateStatus: (status) => status >= 200 && status < 300")
	assert.Contains(t, c, `new ApiError(0, "NETWORK_ERROR"`)
	assert.Contains(t, c, "axios.isAxiosError(e)")
	assert.Contains(t, c, `"UNKNOWN"`)
	assert.Contains(t, c, "parsed.data.fields")
}

func TestGenerate_UnknownClientKind(t *testing.T) {
	files, err := Generate([]api.Version{fixtureVersion("v1")}, Options{Client: "xhr"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown client kind")
	assert.Nil(t, files)
}

func TestGenerate_Deterministic(t *testing.T) {
	versions := []api.Version{fixtureVersion("v1"), fixtureVersion("v2")}

	a, err := Generate(versions, Options{})
	require.NoError(t, err)
	for i := 0; i < 5; i++ {
		b, err := Generate(versions, Options{})
		require.NoError(t, err)
		require.Equal(t, a, b, "Generate must be byte-for-byte reproducible")
	}
}

func TestGenerate_VersionsAreIndependent(t *testing.T) {
	files, err := Generate([]api.Version{fixtureVersion("v1"), fixtureVersion("v2")}, Options{})
	require.NoError(t, err)

	assert.Len(t, files, 8)
	assert.Contains(t, string(files["v1/client.ts"]), "/api/v1/rpc/createWidget")
	assert.NotContains(t, string(files["v1/client.ts"]), "/api/v2/")
	assert.Contains(t, string(files["v2/client.ts"]), "/api/v2/rpc/createWidget")
	assert.Contains(t, string(files["v2/hooks.ts"]), `["v2", "getWidget", input]`)
}

func TestGenerate_Content(t *testing.T) {
	files, err := Generate([]api.Version{fixtureVersion("v1")}, Options{})
	require.NoError(t, err)

	schemas := string(files["v1/schemas.ts"])
	assert.Contains(t, schemas, "DO NOT EDIT")
	assert.Contains(t, schemas, `import { z } from "zod"`)
	assert.Contains(t, schemas, "export const CreateWidgetReqSchema")
	assert.Contains(t, schemas, "ownerId: z.string()", "string IDs map to z.string()")
	assert.Contains(t, schemas, "z.coerce.date()", "time.Time must map to coerced date")
	assert.Contains(t, schemas, "export const ApiErrorSchema")

	client := string(files["v1/client.ts"])
	assert.Contains(t, client, "export function createClient")
	assert.Contains(t, client, "export class ApiError")
	assert.Contains(t, client, "listWidgets: (input: Empty = {})", "api.Empty input must be optional")
	assert.NotContains(t, client, "*/ terminator", "description must not close the JSDoc early")
	assert.Contains(t, client, `*\/ terminator`)

	hooks := string(files["v1/hooks.ts"])
	assert.Contains(t, hooks, "useCreateWidget: ")
	assert.Contains(t, hooks, "useMutation(")
	assert.Contains(t, hooks, `useQuery({ queryKey: ["v1", "getWidget", input]`)
	assert.Contains(t, hooks, `useQuery({ queryKey: ["v1", "listWidgets", {}]`)

	assert.Contains(t, string(files["v1/index.ts"]), `export * from "./hooks"`)
}

func TestGenerate_OnlyMutationsImportsOnlyUseMutation(t *testing.T) {
	v := api.Version{Name: "v1", Prefix: "/rpc", Endpoints: []api.Endpoint{
		api.Mutation("resetWidgets", nop[api.Empty, api.Empty]),
	}}
	files, err := Generate([]api.Version{v}, Options{})
	require.NoError(t, err)

	hooks := string(files["v1/hooks.ts"])
	assert.Contains(t, hooks, `import { useMutation } from "@tanstack/react-query"`)
	assert.NotContains(t, hooks, "useQuery")
}

type BadTagReq struct {
	X string `json:"x" validate:"definitely_not_a_real_tag"`
}

func TestGenerate_ErrorsInsteadOfPanicking(t *testing.T) {
	tests := []struct {
		name     string
		versions []api.Version
		want     string
	}{
		{"invalid version", []api.Version{{Name: "V1", Prefix: "/x"}}, "version name"},
		{"duplicate version", []api.Version{fixtureVersion("v1"), fixtureVersion("v1")}, "duplicate version"},
		{"unsupported validate tag", []api.Version{{Name: "v1", Prefix: "/rpc", Endpoints: []api.Endpoint{
			api.Query("bad", nop[BadTagReq, api.Empty])}}}, "zen"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var files map[string][]byte
			var err error
			require.NotPanics(t, func() { files, err = Generate(tt.versions, Options{}) })
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.want)
			assert.Nil(t, files)
		})
	}
}

// TestApiErrorSchemaMirrorsErrorResponse guards the hand-written
// ApiErrorSchema in templates/schemas.ts.tmpl: if errs.ErrorResponse gains,
// loses or renames a JSON field, this fails until the template is updated.
func TestApiErrorSchemaMirrorsErrorResponse(t *testing.T) {
	typ := reflect.TypeFor[errs.ErrorResponse]()

	var fields []string
	for i := range typ.NumField() {
		name, _, _ := strings.Cut(typ.Field(i).Tag.Get("json"), ",")
		fields = append(fields, name)
	}
	assert.Equal(t, []string{"code", "message", "fields"}, fields,
		"errs.ErrorResponse changed: update ApiErrorSchema in templates/schemas.ts.tmpl")

	files, err := Generate([]api.Version{fixtureVersion("v1")}, Options{})
	require.NoError(t, err)
	schemas := string(files["v1/schemas.ts"])
	assert.Contains(t, schemas, "  code: z.string(),")
	assert.Contains(t, schemas, "  message: z.string(),")
	// fields is `map[string][]string` + omitempty on the Go side.
	assert.Contains(t, schemas, "  fields: z.record(z.string(), z.array(z.string())).optional(),")

	// The thrown ApiError must carry the validation fields through.
	client := string(files["v1/client.ts"])
	assert.Contains(t, client, "readonly fields?: Record<string, string[]> | undefined")
	assert.Contains(t, client, "parsed.data.fields")
}

func TestWrite_WritesFilesAndPrunesOnlyOwnedStaleTS(t *testing.T) {
	out := t.TempDir()
	versions := []api.Version{fixtureVersion("v1")}

	// Pre-existing state: a stale generated file and a hand-written file
	// inside the owned folder, plus an unrelated sibling folder and file.
	owned := filepath.Join(out, "v1")
	other := filepath.Join(out, "v0")
	require.NoError(t, os.MkdirAll(owned, 0o755))
	require.NoError(t, os.MkdirAll(other, 0o755))
	write := func(p string) { require.NoError(t, os.WriteFile(p, []byte("x"), 0o644)) }
	write(filepath.Join(owned, "stale.ts"))
	write(filepath.Join(owned, "README.md"))
	write(filepath.Join(other, "client.ts"))
	write(filepath.Join(out, "package.json"))

	require.NoError(t, Write(out, versions, Options{}))

	for _, name := range fileNames {
		_, err := os.Stat(filepath.Join(owned, name))
		assert.NoError(t, err, name)
	}
	_, err := os.Stat(filepath.Join(owned, "stale.ts"))
	assert.True(t, os.IsNotExist(err), "stale .ts inside an owned version folder must be removed")

	for _, keep := range []string{
		filepath.Join(owned, "README.md"),
		filepath.Join(other, "client.ts"),
		filepath.Join(out, "package.json"),
	} {
		_, err := os.Stat(keep)
		assert.NoError(t, err, "%s must be left alone", keep)
	}
}

func TestWrite_IsIdempotent(t *testing.T) {
	out := t.TempDir()
	versions := []api.Version{fixtureVersion("v1")}
	require.NoError(t, Write(out, versions, Options{}))
	first, err := os.ReadFile(filepath.Join(out, "v1", "client.ts"))
	require.NoError(t, err)

	require.NoError(t, Write(out, versions, Options{}))
	second, err := os.ReadFile(filepath.Join(out, "v1", "client.ts"))
	require.NoError(t, err)

	assert.Equal(t, first, second)
}

// A version with no endpoints (a fresh project) must generate a client that
// is valid TypeScript even under noUnusedLocals/noUnusedParameters: no unused
// imports, no empty import lists, no unused parameter.
func TestGenerate_ZeroFunctions(t *testing.T) {
	empty := []api.Version{{Name: "v1", Prefix: "/api/v1/rpc"}}

	for _, kind := range []ClientKind{ClientFetch, ClientAxios} {
		t.Run(string(kind), func(t *testing.T) {
			files, err := Generate(empty, Options{Client: kind})
			require.NoError(t, err)
			require.Len(t, files, 4)

			schemas := string(files["v1/schemas.ts"])
			assert.Contains(t, schemas, "export const ApiErrorSchema")

			client := string(files["v1/client.ts"])
			assert.Contains(t, client, `import { ApiErrorSchema } from "./schemas"`)
			assert.NotContains(t, client, "import type {  }", "no empty type import")
			assert.Contains(t, client, "void call", "the unused transport helper is referenced")
			assert.Contains(t, client, "export type Client = ReturnType<typeof createClient>")

			hooks := string(files["v1/hooks.ts"])
			assert.NotContains(t, hooks, "@tanstack/react-query", "no hooks, so no react-query import")
			assert.NotContains(t, hooks, "ApiError")
			assert.Contains(t, hooks, "export function createHooks(_client: Client)")

			assert.Contains(t, string(files["v1/index.ts"]), `export * from "./hooks"`)

			// Deterministic like every other output.
			again, err := Generate(empty, Options{Client: kind})
			require.NoError(t, err)
			assert.Equal(t, files, again)
		})
	}
}

// Once a function exists the zero-function special cases must not leak in.
func TestGenerate_NonEmptyHasNoZeroFunctionArtifacts(t *testing.T) {
	files, err := Generate([]api.Version{fixtureVersion("v1")}, Options{})
	require.NoError(t, err)
	assert.NotContains(t, string(files["v1/client.ts"]), "void call")
	assert.Contains(t, string(files["v1/hooks.ts"]), "createHooks(client: Client)")
}
