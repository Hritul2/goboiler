package config

type RedisConfig struct {
	Addr     string `koanf:"addr" validate:"required"`
	Password string `koanf:"password"`
	DB       int    `koanf:"db"`
}
