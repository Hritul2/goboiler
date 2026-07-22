package validation

import zog "github.com/Oudwins/zog"

// UserCreateInput is the payload for UserService.Create.
type UserCreateInput struct {
	Email    string
	Name     string
	Password string
}

// UserCreateSchema validates UserCreateInput. Password is capped at 72 bytes
// to guard against bcrypt's silent truncation past that length.
var UserCreateSchema = zog.Struct(zog.Shape{
	"email":    zog.String().Email().Required(),
	"name":     zog.String().Min(2).Max(100).Required(),
	"password": zog.String().Min(8).Max(72).Required(),
})

// UserUpdateInput is the payload for UserService.Update.
type UserUpdateInput struct {
	Name string
}

// UserUpdateSchema validates UserUpdateInput.
var UserUpdateSchema = zog.Struct(zog.Shape{
	"name": zog.String().Min(2).Max(100).Required(),
})
