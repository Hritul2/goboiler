// Package gen turns registered api.Versions into a typed TypeScript client.
// For every version it emits four files into <out>/<version>/:
//
//	schemas.ts  zod schemas + inferred types for every request/response
//	            struct (zen does the Go struct -> zod conversion), plus
//	            ApiErrorSchema mirroring errs.ErrorResponse
//	client.ts   createClient(): one typed async function per endpoint, and
//	            the ApiError class thrown on non-2xx responses. Options.Client
//	            picks the transport (fetch, the default, or axios); the public
//	            surface is the same either way
//	hooks.ts    createHooks(client): TanStack Query v5 hooks (useQuery for
//	            api.KindQuery, useMutation for api.KindMutation)
//	index.ts    re-exports the three above
//
// Generate is PURE and DETERMINISTIC: it reads only its argument, does no
// I/O, and walks endpoints in registration order, so the same Go code always
// yields byte-identical output. That is what makes `make gen-check` (regen +
// git diff --exit-code) a reliable CI guard and what lets the golden test
// pin the exact text. File output lives in the separate Write function.
//
// This package and cmd/gen are the only importers of github.com/hypersequent/zen;
// the server binary never links it.
package gen

import (
	"bytes"
	"embed"
	"fmt"
	"reflect"
	"strings"
	"text/template"

	"github.com/hypersequent/zen"

	"github.com/hritul2/goboiler/internal/api"
)

//go:embed templates/*.tmpl
var templatesFS embed.FS

// ClientKind selects the transport the generated client.ts uses. Everything
// else (schemas.ts, hooks.ts, index.ts, the public client surface) is
// identical between kinds.
type ClientKind string

const (
	// ClientFetch is the default: the global fetch, no extra dependency.
	ClientFetch ClientKind = "fetch"
	// ClientAxios uses axios (peer dependency axios@^1) and accepts the
	// caller's own AxiosInstance.
	ClientAxios ClientKind = "axios"
)

// Options tunes Generate/Write. The zero value generates the fetch client.
type Options struct {
	Client ClientKind
}

func (o Options) client() ClientKind {
	if o.Client == "" {
		return ClientFetch
	}
	return o.Client
}

// regenCmd is the command printed in client.ts's header for this kind.
func (k ClientKind) regenCmd() string {
	if k == ClientAxios {
		return "make gen GEN_FLAGS=--axios"
	}
	return "make gen"
}

// fileNames are the generated files per version, in write order. The
// templates directory holds one <name>.tmpl for each.
var fileNames = []string{"schemas.ts", "client.ts", "hooks.ts", "index.ts"}

// fn is the template-facing view of one api.Endpoint.
type fn struct {
	Version     string // owning version, for query keys
	Name        string // lowerCamel; also the client method name
	Pascal      string // UpperCamel; hook name is "use" + Pascal
	Path        string // full URL path, e.g. /api/v1/rpc/createUser
	Description string
	IsQuery     bool
	Req, Res    string // TypeScript type names (Go type names)
	ReqEmpty    bool   // Req is api.Empty: input is optional / omitted
}

// versionData is the template input for one version.
type versionData struct {
	Version string
	Client  ClientKind // "fetch" | "axios": picks client.ts's transport
	// RegenCmd is shown in client.ts's header; the other files always say
	// "make gen" so they stay identical between client kinds.
	RegenCmd string
	Schemas  string // zen output, already trimmed
	Fns      []fn
	// Deduplicated, first-seen-order name lists for import statements, so
	// generated files never import something unused (noUnusedLocals-safe).
	ClientTypes   []string // types used in client.ts signatures
	ClientSchemas []string // response schemas used in client.ts
	HookTypes     []string // types used in hooks.ts signatures
	HasQuery      bool
	HasMutation   bool
}

// Generate returns every generated file keyed by slash-separated relative
// path ("v1/schemas.ts"). It returns an error — never panics — for an
// invalid version (see api.Version.Validate), an unknown client kind, or a
// struct zen cannot convert (e.g. an unsupported `validate` tag).
func Generate(versions []api.Version, opts Options) (map[string][]byte, error) {
	kind := opts.client()
	if kind != ClientFetch && kind != ClientAxios {
		return nil, fmt.Errorf("gen: unknown client kind %q (want %q or %q)", kind, ClientFetch, ClientAxios)
	}

	// The shared templates call blocks (clientImports, clientOptions,
	// clientTransport, ...) defined once per kind in client_<kind>.tmpl; only
	// the chosen kind's file is parsed so the block names do not collide.
	tmpl, err := template.New("").Funcs(template.FuncMap{"doc": doc}).ParseFS(templatesFS,
		"templates/schemas.ts.tmpl", "templates/client.ts.tmpl", "templates/hooks.ts.tmpl",
		"templates/index.ts.tmpl", "templates/client_"+string(kind)+".tmpl")
	if err != nil {
		return nil, fmt.Errorf("gen: parse templates: %w", err)
	}

	seen := make(map[string]struct{}, len(versions))
	out := make(map[string][]byte, len(versions)*len(fileNames))

	for _, v := range versions {
		if err := v.Validate(); err != nil {
			return nil, err
		}
		if _, dup := seen[v.Name]; dup {
			return nil, fmt.Errorf("gen: duplicate version name %q", v.Name)
		}
		seen[v.Name] = struct{}{}

		data, err := buildVersionData(v)
		if err != nil {
			return nil, err
		}
		data.Client = kind
		data.RegenCmd = kind.regenCmd()

		for _, name := range fileNames {
			var buf bytes.Buffer
			if err := tmpl.ExecuteTemplate(&buf, name+".tmpl", data); err != nil {
				return nil, fmt.Errorf("gen: version %s: render %s: %w", v.Name, name, err)
			}
			out[v.Name+"/"+name] = buf.Bytes()
		}
	}
	return out, nil
}

// buildVersionData runs zen over the version's types and flattens each
// endpoint into the template view.
func buildVersionData(v api.Version) (data versionData, err error) {
	// zen panics on input it cannot represent (unknown validate tags,
	// cyclic types, name collisions). Surface those as ordinary errors with
	// the version attached.
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("gen: version %s: zen: %v", v.Name, r)
		}
	}()

	// No custom type mappings: request/response DTOs use only plain Go types
	// (IDs are strings), so no storage-driver type ever needs a zod mapping.
	conv := zen.NewConverterWithOpts()

	emptyType := reflect.TypeFor[api.Empty]()
	data.Version = v.Name

	clientTypes, clientSchemas, hookTypes := newOrderedSet(), newOrderedSet(), newOrderedSet()
	for _, e := range v.Endpoints {
		conv.AddType(reflect.Zero(e.Req).Interface())
		conv.AddType(reflect.Zero(e.Res).Interface())

		f := fn{
			Version:     v.Name,
			Name:        e.Name,
			Pascal:      strings.ToUpper(e.Name[:1]) + e.Name[1:],
			Path:        v.Path(e),
			Description: e.Description,
			IsQuery:     e.Kind == api.KindQuery,
			Req:         e.Req.Name(),
			Res:         e.Res.Name(),
			ReqEmpty:    e.Req == emptyType,
		}
		data.Fns = append(data.Fns, f)

		clientTypes.add(f.Req)
		clientTypes.add(f.Res)
		clientSchemas.add(f.Res + "Schema")
		if !f.ReqEmpty {
			hookTypes.add(f.Req)
		}
		hookTypes.add(f.Res)

		if f.IsQuery {
			data.HasQuery = true
		} else {
			data.HasMutation = true
		}
	}

	data.ClientTypes = clientTypes.list
	data.ClientSchemas = clientSchemas.list
	data.HookTypes = hookTypes.list
	data.Schemas = strings.TrimRight(conv.Export(), "\n")
	return data, nil
}

// orderedSet is an insertion-ordered string set (deterministic imports).
type orderedSet struct {
	list []string
	have map[string]struct{}
}

func newOrderedSet() *orderedSet { return &orderedSet{have: map[string]struct{}{}} }

func (s *orderedSet) add(v string) {
	if _, ok := s.have[v]; ok {
		return
	}
	s.have[v] = struct{}{}
	s.list = append(s.list, v)
}

// doc renders text as a JSDoc block indented by indent, or "" when text is
// empty. "*/" inside the text is defused so a description can never close
// the comment early.
func doc(indent, text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	text = strings.ReplaceAll(text, "*/", "*\\/")

	var b strings.Builder
	b.WriteString(indent + "/**\n")
	for _, line := range strings.Split(text, "\n") {
		b.WriteString(strings.TrimRight(indent+" * "+line, " ") + "\n")
	}
	b.WriteString(indent + " */\n")
	return b.String()
}
