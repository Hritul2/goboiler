package config

type Config struct {
	AppConfig     `koanf:"app"`
	ServerConfig  `koanf:"server"`
	DbConfig      `koanf:"db"`
	OpenTelConfig `koanf:"otel"`
	RedisConfig   `koanf:"redis"`
	AuthConfig    `koanf:"auth"`
	EmailConfig   `koanf:"email"`
}
