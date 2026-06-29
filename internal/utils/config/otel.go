package config

type OpenTelConfig struct {
	Endpoint string `koanf:"endpoint" validate:"required"`
	Enabled  bool   `koanf:"enabled"`
}
