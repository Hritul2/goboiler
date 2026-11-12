package api

import (
	"fmt"
	"reflect"
	"regexp"
	"strings"

	"github.com/gin-gonic/gin"
)

// Version is one independently mounted, independently generated API surface
// (v1, v2, ...). Versions share nothing at the routing level: each is its
// own gin group under its own Prefix with its own middleware, and the
// generator writes each to its own output folder. Adding v2 therefore never
// touches v1's routes or generated client.
type Version struct {
	// Name is the short identifier ("v1"). It names the generated output
	// folder and is the first element of every TanStack query key.
	Name string

	// Prefix is the URL path the version's functions live under, e.g.
	// "/api/v1/rpc". A function is served at POST <Prefix>/<function name>.
	Prefix string

	// Middleware runs for every endpoint in this version (before any
	// endpoint-level middleware). This is where version-wide auth belongs.
	Middleware []gin.HandlerFunc

	// Endpoints lists the functions in registration order. That order is
	// preserved in the generated TypeScript so regenerating produces
	// minimal diffs.
	Endpoints []Endpoint
}

var (
	// versionNameRe: lowercase letters/digits, starting with a letter. It is
	// used as a directory name and a string literal, so keep it boring.
	versionNameRe = regexp.MustCompile(`^[a-z][a-z0-9]*$`)

	// endpointNameRe: lowerCamel identifier. The name becomes a TypeScript
	// function name (createUser) and the PascalCase form becomes a hook name
	// (useCreateUser), so it must be a valid identifier that starts lowercase.
	endpointNameRe = regexp.MustCompile(`^[a-z][a-zA-Z0-9]*$`)
)

// Path returns the full URL path an endpoint of this version is served at.
// Mount and the generator both use it, so the server route and the client's
// fetch URL cannot drift apart.
func (v Version) Path(e Endpoint) string {
	return v.Prefix + "/" + e.Name
}

// Validate checks every structural rule that would otherwise surface later
// as a confusing runtime or generator failure. It returns the first problem
// found, with the version and function named so the fix is obvious.
//
// Rules:
//   - version Name is a short lowercase identifier; Prefix starts with "/"
//     and has no trailing "/",
//   - every endpoint was built with Query/Mutation (non-nil handler, valid
//     Kind),
//   - endpoint names are lowerCamel identifiers and unique within the version,
//   - Req and Res are NAMED STRUCT types (zen derives schema names from the
//     Go type name, and cannot name an anonymous struct),
//   - two different Go types with the same short name (e.g. a.Req and b.Req)
//     are rejected, because they would collide on one zod schema name.
func (v Version) Validate() error {
	if !versionNameRe.MatchString(v.Name) {
		return fmt.Errorf("api: version name %q must match %s", v.Name, versionNameRe)
	}
	if !strings.HasPrefix(v.Prefix, "/") || (len(v.Prefix) > 1 && strings.HasSuffix(v.Prefix, "/")) {
		return fmt.Errorf("api: version %s: prefix %q must start with / and not end with /", v.Name, v.Prefix)
	}

	seen := make(map[string]struct{}, len(v.Endpoints))
	typeNames := make(map[string]reflect.Type)

	for _, e := range v.Endpoints {
		if e.handle == nil || (e.Kind != KindQuery && e.Kind != KindMutation) {
			return fmt.Errorf("api: version %s: endpoint %q was not built with api.Query or api.Mutation", v.Name, e.Name)
		}
		if !endpointNameRe.MatchString(e.Name) {
			return fmt.Errorf("api: version %s: endpoint name %q must be a lowerCamel identifier matching %s", v.Name, e.Name, endpointNameRe)
		}
		if _, dup := seen[e.Name]; dup {
			return fmt.Errorf("api: version %s: duplicate endpoint name %q", v.Name, e.Name)
		}
		seen[e.Name] = struct{}{}

		// A slice (not a map) so the first error reported is deterministic.
		for _, wt := range []struct {
			role string
			typ  reflect.Type
		}{{"request", e.Req}, {"response", e.Res}} {
			if err := checkWireType(wt.typ); err != nil {
				return fmt.Errorf("api: version %s: endpoint %q %s type: %w", v.Name, e.Name, wt.role, err)
			}
			if prev, ok := typeNames[wt.typ.Name()]; ok && prev != wt.typ {
				return fmt.Errorf("api: version %s: endpoint %q: type name %q is used by both %s and %s; rename one", v.Name, e.Name, wt.typ.Name(), prev, wt.typ)
			}
			typeNames[wt.typ.Name()] = wt.typ
		}
	}
	return nil
}

// checkWireType enforces "named struct, not generic".
func checkWireType(t reflect.Type) error {
	switch {
	case t == nil:
		return fmt.Errorf("type is nil")
	case t.Kind() != reflect.Struct:
		return fmt.Errorf("%s must be a struct (wrap it in a named struct)", t)
	case t.Name() == "":
		return fmt.Errorf("%s must be a named struct type", t)
	case strings.ContainsAny(t.Name(), "[]"):
		return fmt.Errorf("%s is a generic instantiation; declare a named struct instead", t)
	}
	return nil
}

// Mount registers v on r: a gin group at v.Prefix carrying v.Middleware,
// with one POST route per endpoint at <Prefix>/<Name> (endpoint middleware
// first, then the adapter). Each version is its own group so versions stay
// independent.
//
// Mount PANICS on any Validate failure. That is intentional: these are
// programming errors in a static, compiled-in list, and failing at boot
// (like gin does for conflicting routes) is far better than serving a
// half-registered API or generating a broken client.
func Mount(r gin.IRouter, v Version) {
	if err := v.Validate(); err != nil {
		panic(err)
	}

	group := r.Group(v.Prefix, v.Middleware...)
	for _, e := range v.Endpoints {
		handlers := make([]gin.HandlerFunc, 0, len(e.Middleware)+1)
		handlers = append(handlers, e.Middleware...)
		handlers = append(handlers, e.handle)
		group.POST("/"+e.Name, handlers...)
	}
}
