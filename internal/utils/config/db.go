package config

type DbConfig struct {
	Uri             string `koanf:"uri"               validate:"required"`
	MaxOpenConns    int    `koanf:"max_open_conns"    validate:"required,min=1"`
	MaxIdleConns    int    `koanf:"max_idle_conns"    validate:"required,min=1,ltefield=MaxOpenConns"`
	ConnMaxLifetime int    `koanf:"conn_max_lifetime" validate:"required,min=1"`
	ConnMaxIdleTime int    `koanf:"conn_max_idle_time" validate:"required,min=1"`
}
