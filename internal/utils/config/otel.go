package config

type OpenTelConfig struct {
	Endpoint   string  `koanf:"endpoint"    validate:"required_if=Enabled true"`
	Enabled    bool    `koanf:"enabled"`
	SampleRate float64 `koanf:"sample_rate"`
}
