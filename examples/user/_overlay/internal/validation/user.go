package validation

// UserCreateInput is the payload for UserService.Create. Password is capped
// at 72 bytes to guard against bcrypt's silent truncation past that length.
type UserCreateInput struct {
	Email    string `json:"email" validate:"required,email"`
	Name     string `json:"name" validate:"required,min=2,max=100"`
	Password string `json:"password" validate:"required,min=8,max=72"`
}

// UserUpdateInput is the payload for UserService.Update.
type UserUpdateInput struct {
	Name string `json:"name" validate:"required,min=2,max=100"`
}
