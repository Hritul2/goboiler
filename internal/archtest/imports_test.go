package archtest

import (
	"bytes"
	"encoding/json"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// goPackage is the subset of `go list -json` output this test reads.
type goPackage struct {
	ImportPath   string
	Imports      []string
	TestImports  []string
	XTestImports []string
}

// importRule says: packages whose import path starts with ImportPrefix may
// only be imported (directly, including from test files) by the packages
// listed in Allowed. Allowed entries are module-relative package paths.
type importRule struct {
	Name         string
	ImportPrefix string
	Allowed      []string
	Why          string
}

// rules is the architecture. To isolate another database driver, add a rule
// (e.g. pgx -> internal/db + internal/repo/postgres).
var rules = []importRule{
	{
		Name:         "mongo driver stays in the database layer",
		ImportPrefix: "go.mongodb.org/mongo-driver",
		Allowed:      []string{"internal/db", "internal/repo/mongo"},
		Why: "models, services, handlers and the generator must be database-agnostic; " +
			"only the connection layer (internal/db) and the Mongo repository implementation " +
			"(internal/repo/mongo) may know the driver",
	},
	{
		Name:         "repository implementations are wired only by internal/repo",
		ImportPrefix: "{module}/internal/repo/mongo",
		Allowed:      []string{"internal/repo", "internal/repo/mongo"},
		Why: "services depend on the interfaces in internal/repo; " +
			"internal/repo.NewRepositories is the single place that picks a driver",
	},
}

func TestArchitecture_ImportRules(t *testing.T) {
	module, pkgs := listPackages(t)

	for _, rule := range rules {
		t.Run(rule.Name, func(t *testing.T) {
			prefix := strings.ReplaceAll(rule.ImportPrefix, "{module}", module)

			allowed := map[string]bool{}
			for _, a := range rule.Allowed {
				allowed[module+"/"+a] = true
			}

			var violations []string
			for _, p := range pkgs {
				if allowed[p.ImportPath] || allowed[strings.TrimSuffix(p.ImportPath, "_test")] {
					continue
				}
				for _, imp := range concat(p.Imports, p.TestImports, p.XTestImports) {
					if imp == prefix || strings.HasPrefix(imp, prefix+"/") {
						violations = append(violations, p.ImportPath+" imports "+imp)
					}
				}
			}
			sort.Strings(violations)

			require.Empty(t, violations, "%s.\nAllowed importers: %v\nViolations:\n  %s",
				rule.Why, rule.Allowed, strings.Join(violations, "\n  "))
		})
	}
}

// TestArchitecture_RulesMatchTheTree guards the rules themselves: an
// allow-listed package that no longer exists means a rename silently turned
// the rule into "nobody may import it" or hid a hole.
func TestArchitecture_RulesMatchTheTree(t *testing.T) {
	module, pkgs := listPackages(t)

	have := map[string]bool{}
	for _, p := range pkgs {
		have[p.ImportPath] = true
	}
	for _, rule := range rules {
		for _, a := range rule.Allowed {
			require.True(t, have[module+"/"+a], "rule %q allows %s, which does not exist", rule.Name, a)
		}
	}
}

// listPackages runs `go list -json ./...` from the module root and returns
// the module path plus every package's direct imports (non-test, in-package
// test and external test). `go list` — not a hand-rolled parser — so build
// constraints and the module graph are resolved exactly as the compiler does.
func listPackages(t *testing.T) (module string, pkgs []goPackage) {
	t.Helper()

	_, thisFile, _, ok := runtime.Caller(0)
	require.True(t, ok)
	root := filepath.Join(filepath.Dir(thisFile), "..", "..")

	run := func(args ...string) []byte {
		cmd := exec.Command("go", args...)
		cmd.Dir = root
		var stdout, stderr bytes.Buffer
		cmd.Stdout, cmd.Stderr = &stdout, &stderr
		require.NoError(t, cmd.Run(), "go %s failed: %s", strings.Join(args, " "), stderr.String())
		return stdout.Bytes()
	}

	module = strings.TrimSpace(string(run("list", "-m")))
	require.NotEmpty(t, module)

	dec := json.NewDecoder(bytes.NewReader(run("list", "-json=ImportPath,Imports,TestImports,XTestImports", "./...")))
	for dec.More() {
		var p goPackage
		require.NoError(t, dec.Decode(&p))
		pkgs = append(pkgs, p)
	}
	require.NotEmpty(t, pkgs)
	return module, pkgs
}

func concat(lists ...[]string) []string {
	var out []string
	for _, l := range lists {
		out = append(out, l...)
	}
	return out
}
