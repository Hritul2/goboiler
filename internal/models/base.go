// Package models holds the domain entities as pure structs — no DB or HTTP
// logic and no storage-driver types. Persistence lives behind the interfaces
// in internal/repo, whose per-database implementations (internal/repo/mongo,
// and later postgres/sqlite) own the storage representation, tags and ID
// format. models must not import db, redis, handler or any database driver
// package, and its structs carry no bson/gorm/db tags (an architecture test
// in internal/archtest enforces the driver rule).
package models

import "time"

const (
	DefaultPage  = 1
	DefaultLimit = 20
	MaxLimit     = 100
)

// ID is an entity identifier. It is an opaque string to everything outside
// the repository implementations: a Mongo repo stores the hex form of an
// ObjectID, a Postgres repo might store a UUID or a bigint rendered as text,
// a Clerk-backed user might use Clerk's "user_..." string. Only the repo
// that stores the entity knows how to interpret it. The zero value ("")
// means "not persisted yet": the store assigns the ID on Create.
type ID string

// String returns the ID as a plain string (logging, DTO mapping).
func (id ID) String() string { return string(id) }

// IsZero reports whether the ID is unset (entity not yet persisted).
func (id ID) IsZero() bool { return id == "" }

// Base carries the fields shared by every persisted entity. Embed it in a
// domain model; each repo implementation maps it onto its own storage
// layout.
type Base struct {
	ID        ID        `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ADD YOUR DOMAIN: one file per entity in this package (e.g. user.go), each
// embedding Base, with no storage tags — see docs/EXAMPLE.md step 1.

// NewBase returns a Base with matching UTC timestamps and an EMPTY ID: the
// repository assigns the ID when the entity is created, because the ID
// format is a storage decision (ObjectID, UUID, serial, ...).
func NewBase() Base {
	now := time.Now().UTC()
	return Base{CreatedAt: now, UpdatedAt: now}
}

// Touch advances UpdatedAt to now. Call before persisting a mutation.
func (b *Base) Touch() {
	b.UpdatedAt = time.Now().UTC()
}

// Pagination carries list-endpoint paging params. Call Normalize before use.
type Pagination struct {
	Page  int `json:"page"  query:"page"`
	Limit int `json:"limit" query:"limit"`
}

// Normalize clamps out-of-range values to safe defaults.
func (p *Pagination) Normalize() {
	if p.Page < 1 {
		p.Page = DefaultPage
	}
	if p.Limit < 1 {
		p.Limit = DefaultLimit
	}
	if p.Limit > MaxLimit {
		p.Limit = MaxLimit
	}
}

// Skip returns the number of documents to skip for the current page.
func (p Pagination) Skip() int64 {
	return int64((p.Page - 1) * p.Limit)
}
