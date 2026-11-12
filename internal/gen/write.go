package gen

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/hritul2/goboiler/internal/api"
)

// Write generates the client for versions and writes it under outDir as
// <outDir>/<version>/<file>.ts. It also removes stale .ts files — files a
// previous run wrote that the current run no longer produces (e.g. a renamed
// output) — but ONLY inside the version folders it owns, and only *.ts
// files, so anything else in outDir (a hand-written package.json, another
// version's folder from a different tool) is never touched.
func Write(outDir string, versions []api.Version, opts Options) error {
	files, err := Generate(versions, opts)
	if err != nil {
		return err
	}

	for _, v := range versions {
		dir := filepath.Join(outDir, v.Name)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("gen: create %s: %w", dir, err)
		}

		entries, err := os.ReadDir(dir)
		if err != nil {
			return fmt.Errorf("gen: read %s: %w", dir, err)
		}
		for _, ent := range entries {
			if ent.IsDir() || !strings.HasSuffix(ent.Name(), ".ts") {
				continue
			}
			if _, keep := files[v.Name+"/"+ent.Name()]; !keep {
				if err := os.Remove(filepath.Join(dir, ent.Name())); err != nil {
					return fmt.Errorf("gen: remove stale %s: %w", ent.Name(), err)
				}
			}
		}
	}

	for rel, content := range files {
		path := filepath.Join(outDir, filepath.FromSlash(rel))
		if err := os.WriteFile(path, content, 0o644); err != nil {
			return fmt.Errorf("gen: write %s: %w", path, err)
		}
	}
	return nil
}
