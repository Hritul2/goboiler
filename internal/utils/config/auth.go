package config

type AuthConfig struct {
	AccessTokenSecret  string `koanf:"access_token_secret" validate:"required"`
	RefreshTokenSecret string `koanf:"refresh_token_secret" validate:"required"`
	AccessTokenExpiry  int    `koanf:"access_token_expiry" validate:"required"`
	RefreshTokenExpiry int    `koanf:"refresh_token_expiry" validate:"required"`
}
