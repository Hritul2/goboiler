package config

type AppConfig struct {
	Name    string `koanf:"name" validate:"required"`
	Env     string `koanf:"env" validate:"required"`
	Version string `koanf:"version" validate:"required"`
}
