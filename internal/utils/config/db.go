package config

// DbConfig holds settings for the active database (MongoDB today; the pool
// fields keep their SQL-shaped names so the commented Postgres implementation
// in internal/db can reuse them unchanged).
//
// Field → Mongo client option mapping (internal/db/mongo.go):
//
//	Uri             → ApplyURI
//	Name            → Client().Database(Name)
//	MaxOpenConns    → SetMaxPoolSize
//	MaxIdleConns    → SetMinPoolSize (connections kept warm)
//	ConnMaxIdleTime → SetMaxConnIdleTime (seconds)
//	ConnMaxLifetime → no Mongo equivalent; reserved for the Postgres impl
type DbConfig struct {
	Uri             string `koanf:"uri"                validate:"required"`
	Name            string `koanf:"name"               validate:"required"`
	MaxOpenConns    int    `koanf:"max_open_conns"     validate:"required,min=1"`
	MaxIdleConns    int    `koanf:"max_idle_conns"     validate:"required,min=1,ltefield=MaxOpenConns"`
	ConnMaxLifetime int    `koanf:"conn_max_lifetime"  validate:"required,min=1"`
	ConnMaxIdleTime int    `koanf:"conn_max_idle_time" validate:"required,min=1"`
}
