package models

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewBase(t *testing.T) {
	b := NewBase()

	require.False(t, b.ID.IsZero(), "ID must be generated")
	assert.True(t, b.CreatedAt.Equal(b.UpdatedAt), "CreatedAt and UpdatedAt must match at creation")
	assert.Equal(t, time.UTC, b.CreatedAt.Location(), "timestamps must be UTC")
	assert.WithinDuration(t, time.Now().UTC(), b.CreatedAt, time.Second)
}

func TestBase_Touch(t *testing.T) {
	b := NewBase()
	past := time.Now().UTC().Add(-time.Hour)
	b.UpdatedAt = past

	b.Touch()

	assert.True(t, b.UpdatedAt.After(past), "Touch must advance UpdatedAt")
	assert.Equal(t, time.UTC, b.UpdatedAt.Location())
}

func TestPagination_Normalize(t *testing.T) {
	tests := []struct {
		name      string
		in        Pagination
		wantPage  int
		wantLimit int
	}{
		{"zero_values", Pagination{Page: 0, Limit: 0}, DefaultPage, DefaultLimit},
		{"negative_values", Pagination{Page: -3, Limit: -5}, DefaultPage, DefaultLimit},
		{"limit_above_max", Pagination{Page: 2, Limit: 500}, 2, MaxLimit},
		{"valid_passthrough", Pagination{Page: 3, Limit: 50}, 3, 50},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.in.Normalize()
			assert.Equal(t, tt.wantPage, tt.in.Page)
			assert.Equal(t, tt.wantLimit, tt.in.Limit)
		})
	}
}

func TestPagination_Skip(t *testing.T) {
	assert.EqualValues(t, 40, Pagination{Page: 3, Limit: 20}.Skip())
	assert.EqualValues(t, 0, Pagination{Page: 1, Limit: 20}.Skip())
}
