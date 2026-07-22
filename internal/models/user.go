package models

// User is the account entity. PasswordHash is never serialised to JSON.
type User struct {
	Base         `bson:",inline"`
	Email        string `bson:"email"         json:"email"`
	Name         string `bson:"name"          json:"name"`
	PasswordHash string `bson:"password_hash" json:"-"`
}

// NewUser returns a User with a populated Base. The caller is responsible
// for hashing the password before constructing the model.
func NewUser(email, name, passwordHash string) *User {
	return &User{
		Base:         NewBase(),
		Email:        email,
		Name:         name,
		PasswordHash: passwordHash,
	}
}
