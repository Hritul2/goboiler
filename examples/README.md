# examples

Compiled, tested examples that sit **outside** the application. Nothing here is
part of the server binary, and the root `go build ./...` ignores the overlay
(Go skips directories that start with `_`).

| Path | What |
|---|---|
| `user/` | One complete feature (User), as an overlay of files at their real destination paths. [`docs/EXAMPLE.md`](../docs/EXAMPLE.md) is generated from it. |
| `exampletool/` | The helper behind `make example-*` (standard library only): `materialize` and `docs`. |

## Commands

```sh
make example-try DIR=/tmp/try     # the skeleton + the example, to explore, edit, run `go test ./...` in
make example-check                # build, vet, lint, test and generate the clients in a scratch copy
make example-docs                 # regenerate docs/EXAMPLE.md (commit the result)
make example-docs-check           # fail if docs/EXAMPLE.md is stale
make check                        # includes example-check and example-docs-check
```

`make init` deletes this whole directory so a new project starts without the
User example. `make init MODULE=... KEEP_EXAMPLES=1` keeps it (and keeps the
`example-*` targets working under the new module path).
