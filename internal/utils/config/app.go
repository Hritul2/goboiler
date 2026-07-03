package config

type AppConfig struct {
	Name     string `koanf:"name"      validate:"required"`
	Env      string `koanf:"env"       validate:"required,oneof=dev development staging production"`
	Version  string `koanf:"version"   validate:"required"`
	LogLevel string `koanf:"log_level" validate:"omitempty,oneof=debug info warn error"`
}
