package config

type RedisConfig struct {
	Addr     string `koanf:"addr"     validate:"required"`
	Password string `koanf:"password"`
	DB       int    `koanf:"db"       validate:"min=0,max=15"`
}
