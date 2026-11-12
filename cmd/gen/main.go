// Command gen writes the typed TypeScript client for every registered API
// version. Run it via `make gen` (output: gen/ts/<version>/*.ts) after any
// change to an endpoint's name, request or response struct.
//
// Flags: -out <dir> sets the output directory (make gen OUT=...); -axios (or
// --axios; Go's flag package accepts both) generates an axios-based client.ts
// instead of the default fetch one (make gen GEN_FLAGS=--axios). Only
// client.ts differs between the two.
//
// It builds the handler tree with nil dependencies — the generator only
// reads endpoint metadata (names, kinds, reflect.Types) and never invokes a
// handler, so no database, Redis or config is needed.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/rs/zerolog"

	"github.com/hritul2/goboiler/internal/gen"
	"github.com/hritul2/goboiler/internal/handler"
	"github.com/hritul2/goboiler/internal/router"
	"github.com/hritul2/goboiler/internal/services"
)

func main() {
	out := flag.String("out", "gen/ts", "output directory; each API version is written to <out>/<version>/")
	useAxios := flag.Bool("axios", false, "generate client.ts on axios (peer dependency axios@^1) instead of fetch; all other files are identical")
	flag.Parse()

	opts := gen.Options{Client: gen.ClientFetch}
	if *useAxios {
		opts.Client = gen.ClientAxios
	}

	h := handler.New(nil, nil, &services.Services{}, zerolog.Nop())
	versions := router.APIVersions(h)

	if err := gen.Write(*out, versions, opts); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	for _, v := range versions {
		fmt.Printf("generated %s/%s (%d functions, %s client)\n", *out, v.Name, len(v.Endpoints), opts.Client)
	}
}
