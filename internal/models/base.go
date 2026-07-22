// Package models holds the domain entities as pure structs — no DB or HTTP
// logic. Persistence lives in internal/repo; models must not import db,
// redis, or handler packages.
package models

import (
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

const (
	DefaultPage  = 1
	DefaultLimit = 20
	MaxLimit     = 100
)

// Base carries the fields shared by every persisted entity. Embed it with
// `bson:",inline"` so its fields flatten into the document root.
type Base struct {
	ID        bson.ObjectID `bson:"_id,omitempty" json:"id"`
	CreatedAt time.Time     `bson:"created_at"    json:"created_at"`
	UpdatedAt time.Time     `bson:"updated_at"    json:"updated_at"`
}

// NewBase returns a Base with a fresh ObjectID and matching UTC timestamps.
func NewBase() Base {
	now := time.Now().UTC()
	return Base{
		ID:        bson.NewObjectID(),
		CreatedAt: now,
		UpdatedAt: now,
	}
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
