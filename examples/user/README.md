# examples/user

The User feature from [`docs/EXAMPLE.md`](../../docs/EXAMPLE.md), as code.

- `_overlay/` holds the files at their **real destination paths**: copy a file
  to the same path in a goboiler checkout and you have the step. Some files
  replace a skeleton file of the same name (`internal/repo/repositories.go`,
  `internal/services/services.go`, `internal/handler/handlers.go`,
  `internal/router/versions.go`, `internal/services/registerasynctasks.go`,
  `internal/services/services_test.go`, `internal/lib/email/client_test.go`,
  `internal/repo/repositories_test.go`). When the skeleton changes, diff these
  against it: drift in their comments does not fail the build.
- `EXAMPLE.md.tmpl` is the page's prose. Its directives pull the code out of
  the overlay (and the generated TypeScript out of the scratch tree):
  - `<!-- example:file path="..." [label="..."] -->` a whole file
  - `<!-- example:region path="..." name="..." label="..." -->` the part
    between `// example:region NAME` and `// example:endregion NAME` (a name
    may be used several times in a file; the pieces are joined)
  - `<!-- example:excerpt path="..." from="..." to="..." label="..." -->`
    the lines from the first containing `from` through the next containing `to`
  - `<!-- example:sketch path="sketches/..." -->` a block that is **not** compiled
  - Every `// example:` marker line is removed from the output. An unknown
    directive, a missing file or a missing marker is an error, which is how
    drift is caught.
- `sketches/` holds the one block that is not compiled (Postgres; it needs pgx).
  The `.txt` extension keeps Go tools away.

Edit the overlay or the template, then `make example-docs` and commit both.
`docs/EXAMPLE.md` records a hash of everything under this directory, so
`make example-docs-check` also fails if you changed a file here and did not
regenerate.
