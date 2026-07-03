package config

type AuthConfig struct {
	AccessTokenSecret  string `koanf:"access_token_secret"  validate:"required,min=32"`
	RefreshTokenSecret string `koanf:"refresh_token_secret" validate:"required,min=32"`
	AccessTokenExpiry  int    `koanf:"access_token_expiry"  validate:"required,min=60"`
	RefreshTokenExpiry int    `koanf:"refresh_token_expiry" validate:"required,min=300"`
}
