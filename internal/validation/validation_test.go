package validation

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestErrors_Error(t *testing.T) {
	e := Errors{
		"password": {"is required"},
		"email":    {"must be a valid email address"},
	}
	// Deterministic (sorted-key) join, regardless of map iteration order.
	assert.Equal(t, "email: must be a valid email address; password: is required", e.Error())
}

func TestErrors_HasErrors(t *testing.T) {
	assert.False(t, Errors(nil).HasErrors())
	assert.False(t, Errors{}.HasErrors())
	assert.True(t, Errors{"name": {"is required"}}.HasErrors())
}

func TestValidate(t *testing.T) {
	t.Run("valid_returns_nil", func(t *testing.T) {
		data := struct {
			Name string `validate:"required,min=2"`
		}{Name: "Ada"}
		errs := Validate(&data)
		assert.False(t, errs.HasErrors())
	})

	t.Run("invalid_returns_field_errors", func(t *testing.T) {
		data := struct {
			Name string `validate:"required,min=2"`
		}{Name: ""}
		errs := Validate(&data)
		require.True(t, errs.HasErrors())
		assert.Contains(t, errs, "name")
	})
}

func TestValidate_KeysByJSONName(t *testing.T) {
	type req struct {
		UserID   string `json:"userId"          validate:"required"`
		Omit     string `json:"nick,omitempty"  validate:"required"`
		NoTag    string `validate:"required"`
		Dashed   string `json:"-"               validate:"required"`
		ID       string `validate:"required"`
		UserKey  string `validate:"required"`
		HTTPCode string `validate:"required"`
	}
	errs := Validate(&req{})
	require.True(t, errs.HasErrors())

	assert.ElementsMatch(t,
		[]string{"userId", "nick", "noTag", "dashed", "id", "userKey", "httpCode"},
		keysOf(errs))
	assert.Equal(t, []string{"is required"}, errs["userId"])
}

func TestLowerCamel(t *testing.T) {
	for in, want := range map[string]string{
		"Email":    "email",
		"UserID":   "userID",
		"ID":       "id",
		"URLPath":  "urlPath",
		"HTTPCode": "httpCode",
		"already":  "already",
		"X":        "x",
		"":         "",
	} {
		assert.Equal(t, want, lowerCamel(in), in)
	}
}

func TestValidate_NestedPathsUseDottedJSONNames(t *testing.T) {
	type address struct {
		City string `json:"city" validate:"required"`
		Zip  string `json:"zip"  validate:"required,len=5"`
	}
	type line struct {
		SKU string `json:"sku" validate:"required"`
	}
	type req struct {
		Name    string  `json:"name"    validate:"required"`
		Address address `json:"address"`
		Lines   []line  `json:"lines"   validate:"dive"`
	}

	errs := Validate(&req{Address: address{Zip: "123"}, Lines: []line{{SKU: "ok"}, {}}})
	require.True(t, errs.HasErrors())

	assert.Equal(t, []string{"is required"}, errs["name"])
	assert.Equal(t, []string{"is required"}, errs["address.city"])
	assert.Equal(t, []string{"must be exactly 5 characters"}, errs["address.zip"])
	assert.Equal(t, []string{"is required"}, errs["lines[1].sku"])
	assert.NotContains(t, errs, "req.name", "struct type name must not leak into keys")
}

func TestValidate_NamedAndAnonymousStructsKeyTheSame(t *testing.T) {
	anon := struct {
		Email string `json:"email" validate:"required"`
	}{}
	assert.Contains(t, Validate(&anon), "email")
}

func TestTagMessage_DependsOnKind(t *testing.T) {
	type req struct {
		Str   string         `json:"str"   validate:"min=3"`
		StrOK string         `json:"strOk" validate:"max=1"`
		One   string         `json:"one"   validate:"min=1"`
		Int   int            `json:"int"   validate:"min=18"`
		Float float64        `json:"float" validate:"gte=0.5"`
		Uint  uint           `json:"uint"  validate:"max=10"`
		Lte   int            `json:"lte"   validate:"lte=100"`
		Gt    int            `json:"gt"    validate:"gt=0"`
		Lt    int            `json:"lt"    validate:"lt=5"`
		Items []string       `json:"items" validate:"min=2"`
		Many  []string       `json:"many"  validate:"max=1"`
		Map   map[string]int `json:"map"   validate:"min=1"`
		Len   []string       `json:"len"   validate:"len=2"`
		Eq    int            `json:"eq"    validate:"len=3"`
		Kind  string         `json:"kind"  validate:"oneof=red green blue"`
		Link  string         `json:"link"  validate:"url"`
		ID    string         `json:"id"    validate:"uuid"`
		Other string         `json:"other" validate:"alpha"`
	}
	errs := Validate(&req{
		Str: "ab", StrOK: "toolong", One: "",
		Int: 3, Float: 0.1, Uint: 11, Lte: 101, Gt: 0, Lt: 9,
		Items: []string{"a"}, Many: []string{"a", "b"}, Map: map[string]int{},
		Len: []string{"a"}, Eq: 4,
		Kind: "pink", Link: "nope", ID: "nope", Other: "123",
	})

	want := map[string]string{
		"str":   "must be at least 3 characters",
		"strOk": "must be at most 1 character",
		"one":   "must be at least 1 character",
		"int":   "must be at least 18",
		"float": "must be at least 0.5",
		"uint":  "must be at most 10",
		"lte":   "must be at most 100",
		"gt":    "must be greater than 0",
		"lt":    "must be less than 5",
		"items": "must be at least 2 items",
		"many":  "must be at most 1 item",
		"map":   "must be at least 1 item",
		"len":   "must be exactly 2 items",
		"eq":    "must equal 3",
		"kind":  "must be one of: red, green, blue",
		"link":  "must be a valid URL",
		"id":    "must be a valid UUID",
		"other": "failed 'alpha' validation",
	}
	for field, msg := range want {
		assert.Equal(t, []string{msg}, errs[field], field)
	}
	assert.Len(t, errs, len(want))
}

func TestValidate_PointerFieldUsesDereferencedKind(t *testing.T) {
	n := 3
	type req struct {
		Age *int `json:"age" validate:"min=18"`
	}
	assert.Equal(t, []string{"must be at least 18"}, Validate(&req{Age: &n})["age"])
}

func keysOf(e Errors) []string {
	out := make([]string, 0, len(e))
	for k := range e {
		out = append(out, k)
	}
	return out
}
