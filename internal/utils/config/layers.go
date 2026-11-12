package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
)

// Sources records which files LoadConfig actually used, lowest priority first.
// PrintConfig reports it so "why does the server see this value?" is
// answerable from the boot log.
type Sources struct {
	// ConfigFiles are the JSON files merged into the config, lowest priority
	// first: the base file, then its .local override if one exists.
	ConfigFiles []string

	// EnvFiles are the env files named by the merged config, lowest priority
	// first. Loaded reports, per file, whether it existed. Real OS
	// environment variables beat every one of them.
	EnvFiles []EnvFile
}

// EnvFile is one env file LoadConfig looked for.
type EnvFile struct {
	Path   string
	Loaded bool
}

// localPathFor returns where the optional override for base lives:
// <dir>/<name>.local.json next to <dir>/<name>.json. The base path is used
// as given (it may start with "~/"); only the file name is rewritten.
//
//	configs/config.json  -> configs/config.local.json
//	/etc/app/prod.json   -> /etc/app/prod.local.json
//	settings             -> settings.local.json   (no .json suffix)
func localPathFor(base string) string {
	dir, file := filepath.Split(base)
	return dir + strings.TrimSuffix(file, ".json") + ".local.json"
}

// readOptionalConfigFile reads the override file at p. A missing file is not
// an error (the override is optional) and returns (nil, false, nil); any
// other failure — unreadable, or not valid JSON — is, and names the file.
func readOptionalConfigFile(p string) (raw map[string]any, found bool, err error) {
	b, err := readConfigFile(p)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, false, nil
		}
		return nil, false, err
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return nil, false, fmt.Errorf("parse %s: %w", p, err)
	}
	return raw, true, nil
}

// mergeMaps deep-merges override onto base and returns a new map (neither
// input is modified):
//
//   - both values are JSON objects  -> merged recursively, so an override
//     sets only the keys it mentions and leaves their siblings alone;
//   - anything else (string, number, bool, null, ARRAY) -> the override value
//     replaces the base value outright. Arrays are never concatenated or
//     merged element-wise: an override that lists CORS origins is the list.
func mergeMaps(base, override map[string]any) map[string]any {
	out := make(map[string]any, len(base)+len(override))
	for k, v := range base {
		out[k] = v
	}
	for k, ov := range override {
		bm, bIsMap := out[k].(map[string]any)
		om, oIsMap := ov.(map[string]any)
		if bIsMap && oIsMap {
			out[k] = mergeMaps(bm, om)
			continue
		}
		out[k] = ov
	}
	return out
}

// report renders the sources for PrintConfig, lowest priority first.
func (s Sources) report() string {
	var b strings.Builder
	b.WriteString("=== Config sources (lowest -> highest priority) ===\n")
	for i, f := range s.ConfigFiles {
		kind := "base"
		if i > 0 {
			kind = "override"
		}
		fmt.Fprintf(&b, "  config file: %s (%s)\n", f, kind)
	}
	for _, f := range s.EnvFiles {
		status := "loaded"
		if !f.Loaded {
			status = "not found, skipped"
		}
		fmt.Fprintf(&b, "  env file:    %s (%s)\n", f.Path, status)
	}
	b.WriteString("  OS environment variables win over every file above\n")
	return b.String()
}
