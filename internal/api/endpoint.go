// Package api is this project's small typed-RPC framework. A backend function is
// declared once, as a Go function with a request struct and a response
// struct, and that single declaration drives three things:
//
//  1. the HTTP route (POST <version prefix>/<function name>) via Mount,
//  2. request decoding + validation + error mapping via the gin adapter in
//     handle.go (the only code in this package that touches *gin.Context),
//  3. the generated TypeScript client (internal/gen reads the reflect.Types
//     captured here — it never sees the Go function itself).
//
// Everything is a POST with a JSON body: no GET/PATCH/DELETE at the HTTP
// level, no path or query parameters. That is a deliberate RPC-style choice —
// one calling convention means one adapter, one generated client shape and
// one place to attach auth/rate-limit middleware.
//
// Endpoint is deliberately NOT generic. The generic constructors (Query,
// Mutation) erase Req/Res into reflect.Types and a plain gin.HandlerFunc at
// construction time, so a single []Endpoint can hold functions with
// completely different request/response pairs (the same trick Huma and
// tygor use). A generic Endpoint[Req, Res] could not live in one slice.
//
// This package imports errs (internal/errors) and validation, never a
// concrete service or handler, so it stays reusable across domains.
package api

import (
	"context"
	"reflect"

	"github.com/gin-gonic/gin"
)

// Kind says how the generated React hooks treat an endpoint. It has no effect
// on the HTTP side — both kinds are POST.
type Kind int

const (
	// KindQuery marks a read: the generated hook is a TanStack useQuery
	// keyed by [version, name, input], so results are cached and refetched.
	// Queries must be safe to repeat and must not change state.
	KindQuery Kind = iota + 1

	// KindMutation marks a write: the generated hook is a TanStack
	// useMutation, called imperatively and never cached or auto-refetched.
	KindMutation
)

// String returns the lowercase kind name, used in panic messages and by the
// generator's golden fixtures.
func (k Kind) String() string {
	switch k {
	case KindQuery:
		return "query"
	case KindMutation:
		return "mutation"
	default:
		return "invalid"
	}
}

// Empty is the Req/Res type for a function with no input or no output. It is
// a named struct (not struct{}) because the TypeScript generator needs a
// name to emit a schema for; it renders as z.object({}).
type Empty struct{}

// Endpoint is one registered RPC function, type-erased. Build it with Query
// or Mutation, never by hand — the unexported handle field can only be set
// by the constructors, which is what guarantees Req/Res match the handler.
type Endpoint struct {
	// Name is the lowerCamel function name. It is both the URL suffix
	// (POST <prefix>/<Name>) and the generated TypeScript function name.
	Name string

	// Kind selects useQuery vs useMutation in the generated hooks.
	Kind Kind

	// Req and Res are the request/response struct types. The generator feeds
	// them to zen to produce zod schemas.
	Req, Res reflect.Type

	// Middleware runs before the handler, in order, for this endpoint only.
	// This is the hook for future per-function auth or rate limits.
	Middleware []gin.HandlerFunc

	// Description becomes a JSDoc comment on the generated client function
	// and hook.
	Description string

	// handle is the gin adapter closing over the typed function.
	handle gin.HandlerFunc
}

// Option customises an Endpoint at construction time.
type Option func(*Endpoint)

// WithMiddleware appends per-endpoint gin middleware (e.g. an auth check or
// a stricter rate limit). Order of arguments is order of execution.
func WithMiddleware(mw ...gin.HandlerFunc) Option {
	return func(e *Endpoint) { e.Middleware = append(e.Middleware, mw...) }
}

// WithDescription sets the text emitted as JSDoc on the generated TypeScript
// client function and hook. Keep it to a sentence: it is documentation for
// the frontend developer reading their editor tooltip.
func WithDescription(desc string) Option {
	return func(e *Endpoint) { e.Description = desc }
}

// Query declares a read-only function. fn receives the request context and
// the decoded + validated request, and returns the response or an error.
// Errors should be *errs.AppError values (errs.NotFound(...), etc.); any
// other error becomes a 500 via errs.FromError in middleware.ErrorHandler.
func Query[Req, Res any](name string, fn func(context.Context, Req) (Res, error), opts ...Option) Endpoint {
	return newEndpoint(KindQuery, name, fn, opts)
}

// Mutation declares a state-changing function. Same shape as Query; only the
// generated React hook differs (useMutation instead of useQuery).
func Mutation[Req, Res any](name string, fn func(context.Context, Req) (Res, error), opts ...Option) Endpoint {
	return newEndpoint(KindMutation, name, fn, opts)
}

// newEndpoint is the shared constructor body. It captures the type
// parameters as reflect.Types (the only place Req/Res are known statically)
// and wraps fn in the gin adapter. Structural checks (valid name, named
// struct types, duplicates) are NOT done here — they run in Version.Validate
// at Mount time, so a mistake fails at boot with the whole version in view.
func newEndpoint[Req, Res any](kind Kind, name string, fn func(context.Context, Req) (Res, error), opts []Option) Endpoint {
	e := Endpoint{
		Name:   name,
		Kind:   kind,
		Req:    reflect.TypeFor[Req](),
		Res:    reflect.TypeFor[Res](),
		handle: adapt(fn),
	}
	for _, opt := range opts {
		opt(&e)
	}
	return e
}
