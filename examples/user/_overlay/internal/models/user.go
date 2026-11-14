package models

// User is the account entity. PasswordHash is never serialised to JSON.
// The ID is empty until UserRepository.Create assigns it.
type User struct {
	Base
	Email        string `json:"email"`
	Name         string `json:"name"`
	PasswordHash string `json:"-"`
}

// NewUser returns a User with timestamps set and no ID yet (the repository
// assigns it on Create). The caller is responsible for hashing the password
// before constructing the model.
func NewUser(email, name, passwordHash string) *User {
	return &User{
		Base:         NewBase(),
		Email:        email,
		Name:         name,
		PasswordHash: passwordHash,
	}
}
