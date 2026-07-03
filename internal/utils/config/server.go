package config

type ServerConfig struct {
	Port               int      `koanf:"port"                validate:"required,min=1,max=65535"`
	ReadTimeout        int      `koanf:"read_timeout"        validate:"required,min=1"`
	WriteTimeout       int      `koanf:"write_timeout"       validate:"required,min=1"`
	IdleTimeout        int      `koanf:"idle_timeout"        validate:"required,min=1"`
	ShutdownTimeout    int      `koanf:"shutdown_timeout"    validate:"required,min=5,max=300"`
	CORSAllowedOrigins []string `koanf:"cors_allowed_origins" validate:"required,min=1"`
}
