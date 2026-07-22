package config

// EmailConfig holds SMTP settings for the outbound email client
// (internal/lib/email). Works with any standard SMTP provider (Zoho, Gmail
// app password, AWS SES SMTP). When Enabled is false the app boots without
// SMTP credentials and sends become logged no-ops.
type EmailConfig struct {
	Enabled  bool   `koanf:"enabled"`
	Host     string `koanf:"host"      validate:"required_if=Enabled true"`
	Port     int    `koanf:"port"      validate:"required_if=Enabled true,omitempty,min=1,max=65535"`
	Username string `koanf:"username"`
	Password string `koanf:"password"`
	From     string `koanf:"from"      validate:"required_if=Enabled true,omitempty,email"`
	FromName string `koanf:"from_name"`
}
