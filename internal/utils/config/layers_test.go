package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- helpers ---------------------------------------------------------------

// writeJSON writes v as JSON to dir/name and returns the path.
func writeJSON(t *testing.T, dir, name string, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err)
	p := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(p, b, 0o644))
	return p
}

func writeEnvFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(p, []byte(content), 0o644))
	return p
}

// freshEnv makes sure the named variables are unset for the test and are put
// back to their original state afterwards. LoadConfig loads env files with
// godotenv, which sets real process variables; without this a variable set by
// one test would look like a "real OS variable" to the next.
func freshEnv(t *testing.T, names ...string) {
	t.Helper()
	for _, n := range names {
		t.Setenv(n, "") // registers the restore
		require.NoError(t, os.Unsetenv(n))
	}
}

// layeredBase is a valid base config whose app name and version come from the
// environment, so env-file priority is observable.
func layeredBase(baseEnvFile string) map[string]any {
	raw := validConfigJSON()
	raw["env_file_name"] = baseEnvFile
	app := raw["config"].(map[string]any)["app"].(map[string]any)
	app["name"] = ".env:LAYER_NAME"
	app["version"] = ".env:LAYER_VERSION"
	return raw
}

// --- config file layering --------------------------------------------------

func TestLoadConfig_NoLocalOverride(t *testing.T) {
	dir := t.TempDir()
	base := writeJSON(t, dir, "config.json", validConfigJSON())

	cfg, err := LoadConfig(&base)
	require.NoError(t, err)

	assert.Equal(t, 8080, cfg.Server.Port)
	assert.Equal(t, []string{base}, cfg.Sources().ConfigFiles, "only the base file was used")
}

func TestLoadConfig_LocalOverridesOneNestedKeyAndLeavesSiblings(t *testing.T) {
	dir := t.TempDir()
	base := writeJSON(t, dir, "config.json", validConfigJSON())
	local := writeJSON(t, dir, "config.local.json", map[string]any{
		"config": map[string]any{
			"server": map[string]any{
				"port":                   9090,
				"cors_allow_credentials": true,
			},
		},
	})

	cfg, err := LoadConfig(&base)
	require.NoError(t, err)

	// Overridden:
	assert.Equal(t, 9090, cfg.Server.Port)
	assert.True(t, cfg.Server.CORSAllowCredentials)
	// Siblings in the same object, and other sections, keep the base values:
	assert.Equal(t, 30, cfg.Server.ReadTimeout)
	assert.Equal(t, 120, cfg.Server.IdleTimeout)
	assert.Equal(t, []string{"http://localhost:3000"}, cfg.Server.CORSAllowedOrigins)
	assert.Equal(t, "test-app", cfg.App.Name)
	assert.Equal(t, "mongodb://localhost:27017", cfg.DB.Uri)

	assert.Equal(t, []string{base, local}, cfg.Sources().ConfigFiles, "lowest priority first")
}

func TestLoadConfig_LocalArraysReplaceNotMerge(t *testing.T) {
	dir := t.TempDir()
	raw := validConfigJSON()
	raw["config"].(map[string]any)["server"].(map[string]any)["cors_allowed_origins"] = []string{"https://a.example", "https://b.example"}
	base := writeJSON(t, dir, "config.json", raw)
	writeJSON(t, dir, "config.local.json", map[string]any{
		"config": map[string]any{"server": map[string]any{"cors_allowed_origins": []string{"http://localhost:5173"}}},
	})

	cfg, err := LoadConfig(&base)
	require.NoError(t, err)

	assert.Equal(t, []string{"http://localhost:5173"}, cfg.Server.CORSAllowedOrigins)
}

func TestLoadConfig_LocalIsFoundNextToAnExplicitPath(t *testing.T) {
	dir := t.TempDir()
	base := writeJSON(t, dir, "prod.json", validConfigJSON())
	writeJSON(t, dir, "prod.local.json", map[string]any{
		"config": map[string]any{"server": map[string]any{"port": 7070}},
	})
	// A config.local.json for a DIFFERENT base name must not leak in.
	writeJSON(t, dir, "config.local.json", map[string]any{
		"config": map[string]any{"server": map[string]any{"port": 1111}},
	})

	cfg, err := LoadConfig(&base)
	require.NoError(t, err)

	assert.Equal(t, 7070, cfg.Server.Port)
}

func TestLoadConfig_InvalidLocalFileIsAnErrorNamingIt(t *testing.T) {
	dir := t.TempDir()
	base := writeJSON(t, dir, "config.json", validConfigJSON())
	local := filepath.Join(dir, "config.local.json")
	require.NoError(t, os.WriteFile(local, []byte("{not json"), 0o644))

	_, err := LoadConfig(&base)

	require.Error(t, err)
	assert.Contains(t, err.Error(), local)
}

func TestLoadConfig_OverrideCanMakeABaseThatFailsValidationValid(t *testing.T) {
	// The merged result is what gets validated, not each file on its own.
	dir := t.TempDir()
	raw := validConfigJSON()
	delete(raw["config"].(map[string]any)["server"].(map[string]any), "port")
	base := writeJSON(t, dir, "config.json", raw)

	_, err := LoadConfig(&base)
	require.Error(t, err, "base alone has no port")

	writeJSON(t, dir, "config.local.json", map[string]any{
		"config": map[string]any{"server": map[string]any{"port": 8081}},
	})
	cfg, err := LoadConfig(&base)
	require.NoError(t, err)
	assert.Equal(t, 8081, cfg.Server.Port)
}

func TestLocalPathFor(t *testing.T) {
	for in, want := range map[string]string{
		"configs/config.json": "configs/config.local.json",
		"/etc/app/prod.json":  "/etc/app/prod.local.json",
		"config.json":         "config.local.json",
		"~/cfg/app.json":      "~/cfg/app.local.json",
		"settings":            "settings.local.json",
	} {
		assert.Equal(t, want, localPathFor(in), in)
	}
}

func TestMergeMaps(t *testing.T) {
	base := map[string]any{
		"a":    map[string]any{"x": 1.0, "y": 2.0, "list": []any{"b1", "b2"}},
		"s":    "base",
		"keep": map[string]any{"k": true},
	}
	override := map[string]any{
		"a": map[string]any{"y": 20.0, "list": []any{"o1"}, "z": 3.0},
		"s": map[string]any{"now": "an object"}, // type change: override wins
	}

	got := mergeMaps(base, override)

	assert.Equal(t, map[string]any{
		"a":    map[string]any{"x": 1.0, "y": 20.0, "z": 3.0, "list": []any{"o1"}},
		"s":    map[string]any{"now": "an object"},
		"keep": map[string]any{"k": true},
	}, got)
	// Inputs are untouched.
	assert.Equal(t, 2.0, base["a"].(map[string]any)["y"])
	assert.Equal(t, "base", base["s"])
}

// --- env file layering -----------------------------------------------------

func TestLoadConfig_LocalEnvFileBeatsBaseEnvFile(t *testing.T) {
	freshEnv(t, "LAYER_NAME", "LAYER_VERSION")
	dir := t.TempDir()
	baseEnv := writeEnvFile(t, dir, "base.env", "LAYER_NAME=from-base-env\nLAYER_VERSION=only-in-base-env\n")
	localEnv := writeEnvFile(t, dir, "local.env", "LAYER_NAME=from-local-env\n")

	base := writeJSON(t, dir, "config.json", layeredBase(baseEnv))
	writeJSON(t, dir, "config.local.json", map[string]any{"env_file_name": localEnv})

	cfg, err := LoadConfig(&base)
	require.NoError(t, err)

	assert.Equal(t, "from-local-env", cfg.App.Name, "the override's env file wins")
	assert.Equal(t, "only-in-base-env", cfg.App.Version, "the base env file still supplies what the local one does not")
	assert.Equal(t, []EnvFile{{Path: baseEnv, Loaded: true}, {Path: localEnv, Loaded: true}},
		cfg.Sources().EnvFiles, "lowest priority first")
}

func TestLoadConfig_OSEnvBeatsBothEnvFiles(t *testing.T) {
	freshEnv(t, "LAYER_VERSION")
	t.Setenv("LAYER_NAME", "from-os-env")
	dir := t.TempDir()
	baseEnv := writeEnvFile(t, dir, "base.env", "LAYER_NAME=from-base-env\nLAYER_VERSION=v1\n")
	localEnv := writeEnvFile(t, dir, "local.env", "LAYER_NAME=from-local-env\n")

	base := writeJSON(t, dir, "config.json", layeredBase(baseEnv))
	writeJSON(t, dir, "config.local.json", map[string]any{"env_file_name": localEnv})

	cfg, err := LoadConfig(&base)
	require.NoError(t, err)

	assert.Equal(t, "from-os-env", cfg.App.Name)
	assert.Equal(t, "v1", cfg.App.Version)
}

func TestLoadConfig_OSEnvBeatsBaseEnvFileWithoutAnyLocal(t *testing.T) {
	t.Setenv("LAYER_NAME", "from-os-env")
	freshEnv(t, "LAYER_VERSION")
	dir := t.TempDir()
	baseEnv := writeEnvFile(t, dir, "base.env", "LAYER_NAME=from-base-env\nLAYER_VERSION=v1\n")
	base := writeJSON(t, dir, "config.json", layeredBase(baseEnv))

	cfg, err := LoadConfig(&base)
	require.NoError(t, err)

	assert.Equal(t, "from-os-env", cfg.App.Name)
}

func TestLoadConfig_MissingEnvFileIsReportedAndSkipped(t *testing.T) {
	freshEnv(t, "LAYER_NAME", "LAYER_VERSION")
	dir := t.TempDir()
	baseEnv := writeEnvFile(t, dir, "base.env", "LAYER_NAME=n\nLAYER_VERSION=v\n")
	missing := filepath.Join(dir, "does-not-exist.env")

	base := writeJSON(t, dir, "config.json", layeredBase(baseEnv))
	writeJSON(t, dir, "config.local.json", map[string]any{"env_file_name": missing})

	cfg, err := LoadConfig(&base)
	require.NoError(t, err)

	assert.Equal(t, "n", cfg.App.Name)
	assert.Equal(t, []EnvFile{{Path: baseEnv, Loaded: true}, {Path: missing, Loaded: false}}, cfg.Sources().EnvFiles)
}

func TestLoadConfig_SameEnvFileInBothIsLoadedOnce(t *testing.T) {
	freshEnv(t, "LAYER_NAME", "LAYER_VERSION")
	dir := t.TempDir()
	env := writeEnvFile(t, dir, "shared.env", "LAYER_NAME=n\nLAYER_VERSION=v\n")
	base := writeJSON(t, dir, "config.json", layeredBase(env))
	writeJSON(t, dir, "config.local.json", map[string]any{"env_file_name": env})

	cfg, err := LoadConfig(&base)
	require.NoError(t, err)

	assert.Equal(t, []EnvFile{{Path: env, Loaded: true}}, cfg.Sources().EnvFiles)
}

func TestLoadConfig_InterpolationRunsOnTheMergedResult(t *testing.T) {
	freshEnv(t, "LAYER_NAME", "LAYER_VERSION", "LAYER_PORT", "LAYER_ORIGINS")
	dir := t.TempDir()
	baseEnv := writeEnvFile(t, dir, "base.env", "LAYER_NAME=n\nLAYER_VERSION=v\n")
	localEnv := writeEnvFile(t, dir, "local.env", "LAYER_PORT=9191\nLAYER_ORIGINS=http://a.example, http://b.example\n")

	base := writeJSON(t, dir, "config.json", layeredBase(baseEnv))
	// The OVERRIDE introduces typed references (.env:int:, .env:array:); they
	// must be resolved after the merge, from the layered environment.
	writeJSON(t, dir, "config.local.json", map[string]any{
		"env_file_name": localEnv,
		"config": map[string]any{"server": map[string]any{
			"port":                 ".env:int:LAYER_PORT",
			"cors_allowed_origins": ".env:array:LAYER_ORIGINS",
		}},
	})

	cfg, err := LoadConfig(&base)
	require.NoError(t, err)

	assert.Equal(t, 9191, cfg.Server.Port)
	assert.Equal(t, []string{"http://a.example", "http://b.example"}, cfg.Server.CORSAllowedOrigins)
}

func TestSourcesReport(t *testing.T) {
	got := Sources{
		ConfigFiles: []string{"configs/config.json", "configs/config.local.json"},
		EnvFiles:    []EnvFile{{Path: ".env", Loaded: true}, {Path: ".env.local", Loaded: false}},
	}.report()

	assert.Equal(t, `=== Config sources (lowest -> highest priority) ===
  config file: configs/config.json (base)
  config file: configs/config.local.json (override)
  env file:    .env (loaded)
  env file:    .env.local (not found, skipped)
  OS environment variables win over every file above
`, got)
}

// TestShippedConfigsLoadAndMerge loads the repository's real configs/ files
// with a complete environment, so a typo in config.json or config.local.json
// (or a key the override sets that the base lacks) fails here, not at boot.
func TestShippedConfigsLoadAndMerge(t *testing.T) {
	for k, v := range map[string]string{
		"APP_ENV": "dev", "APP_VERSION": "0.0.0-test", "LOG_LEVEL": "info",
		"SERVER_PORT": "8080", "CORS_ALLOWED_ORIGINS": "http://localhost:3000",
		"DATABASE_URL": "mongodb://localhost:27017", "DATABASE_NAME": "test",
		"OTEL_ENABLED": "false", "OTEL_ENDPOINT": "", "OTEL_SAMPLE_RATE": "1.0",
		"REDIS_ADDR": "localhost:6379", "REDIS_PASSWORD": "",
		"EMAIL_ENABLED": "false", "EMAIL_HOST": "", "EMAIL_PORT": "587",
		"EMAIL_USERNAME": "", "EMAIL_PASSWORD": "", "EMAIL_FROM": "", "EMAIL_FROM_NAME": "",
		"ACCESS_TOKEN_SECRET":  "test-access-token-secret-min-32-chars",
		"REFRESH_TOKEN_SECRET": "test-refresh-token-secret-min-32-chars",
	} {
		t.Setenv(k, v)
	}
	base := filepath.Join("..", "..", "..", "configs", "config.json")

	cfg, err := LoadConfig(&base)
	require.NoError(t, err)

	assert.Equal(t, 8080, cfg.Server.Port)
	assert.Equal(t, 30, cfg.Server.ReadTimeout, "base value that the override does not touch")

	// The override is optional (a project may gitignore it); when present it
	// must be listed after the base and apply on top of it.
	files := cfg.Sources().ConfigFiles
	require.NotEmpty(t, files)
	assert.Equal(t, base, files[0])
	if len(files) == 2 {
		local := localPathFor(base)
		assert.Equal(t, local, files[1])
		raw, _, err := readOptionalConfigFile(local)
		require.NoError(t, err)
		// Whatever the override file sets for this key is what wins.
		section, _ := raw["config"].(map[string]any)
		server, _ := section["server"].(map[string]any)
		if v, ok := server["cors_allow_credentials"].(bool); ok {
			assert.Equal(t, v, cfg.Server.CORSAllowCredentials)
		}
	}
}
