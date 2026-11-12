package config

// Config is the root configuration. Each section is a NAMED field (not an
// embedded struct) so every use site spells out where a value comes from —
// cfg.App.Env, cfg.Server.Port, cfg.DB.Uri — instead of a mix of promoted
// (cfg.Env) and qualified (cfg.ServerConfig.Port) forms. The koanf tags are
// the config-file / env-var section names and are unchanged by this layout.
type Config struct {
	App    AppConfig     `koanf:"app"`
	Server ServerConfig  `koanf:"server"`
	DB     DbConfig      `koanf:"db"`
	OTel   OpenTelConfig `koanf:"otel"`
	Redis  RedisConfig   `koanf:"redis"`
	Auth   AuthConfig    `koanf:"auth"`
	Email  EmailConfig   `koanf:"email"`

	// sources records which files LoadConfig used (unexported: it is not
	// configuration, so koanf and the validator never see it).
	sources Sources
}

// Sources reports the config and env files this Config was loaded from,
// lowest priority first. The zero value (a Config not built by LoadConfig) has
// none.
func (c *Config) Sources() Sources { return c.sources }
