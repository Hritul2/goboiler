package config

type ServerConfig struct {
	Port               int      `koanf:"port"                validate:"required,min=1,max=65535"`
	ReadTimeout        int      `koanf:"read_timeout"        validate:"required,min=1"`
	WriteTimeout       int      `koanf:"write_timeout"       validate:"required,min=1"`
	IdleTimeout        int      `koanf:"idle_timeout"        validate:"required,min=1"`
	ShutdownTimeout    int      `koanf:"shutdown_timeout"    validate:"required,min=5,max=300"`
	CORSAllowedOrigins []string `koanf:"cors_allowed_origins" validate:"required,min=1"`

	// CORS — methods/headers/max-age were previously hardcoded in cors.go,
	// now config-driven. CORSAllowCredentials deliberately has no default
	// applied in setDefaults (see loader.go) — its Go zero value (false) is
	// the secure-by-default posture (no cross-origin credential sharing
	// unless explicitly opted into).
	CORSAllowedMethods   []string `koanf:"cors_allowed_methods"`
	CORSAllowedHeaders   []string `koanf:"cors_allowed_headers"`
	CORSAllowCredentials bool     `koanf:"cors_allow_credentials"`
	CORSMaxAgeSeconds    int      `koanf:"cors_max_age_seconds"`

	// Body size limits, in bytes, enforced by middleware.MaxReqResSize.
	MaxRequestBodyBytes  int64 `koanf:"max_request_body_bytes"`
	MaxResponseBodyBytes int64 `koanf:"max_response_body_bytes"`

	// Rate limiting — per-IP token bucket, enforced by middleware.RateLimit.
	RateLimitPerMinute int `koanf:"rate_limit_per_minute"`
	RateLimitBurst     int `koanf:"rate_limit_burst"`
}
