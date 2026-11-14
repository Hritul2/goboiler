// Command exampletool builds and documents the compiled example under
// examples/user. It is standard library only and is not part of the
// application: `make init` deletes examples/ unless KEEP_EXAMPLES=1.
//
//	exampletool materialize -dst DIR [-src .] [-overlay examples/user/_overlay] [-clean]
//	exampletool docs -tree DIR [-tmpl examples/user/EXAMPLE.md.tmpl] [-out docs/EXAMPLE.md]
//
// materialize copies the project (tracked files plus untracked files that are
// not ignored, minus examples/) into DIR and lays the example overlay on top.
// docs renders the template, whose directives pull code from the overlay and
// from the generated tree, into the markdown page.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
)

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "exampletool:", err)
		os.Exit(1)
	}
}

func run(args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: exampletool materialize|docs [flags]")
	}
	switch args[0] {
	case "materialize":
		fs := flag.NewFlagSet("materialize", flag.ContinueOnError)
		fs.SetOutput(stderr)
		src := fs.String("src", ".", "project root to copy")
		overlay := fs.String("overlay", "examples/user/_overlay", "overlay directory laid on top of the copy")
		dst := fs.String("dst", "", "destination directory (required)")
		clean := fs.Bool("clean", false, "empty a non-empty destination first")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if *dst == "" {
			return fmt.Errorf("materialize: -dst is required")
		}
		n, err := materialize(*src, *overlay, *dst, *clean)
		if err != nil {
			return err
		}
		_, _ = fmt.Fprintf(stdout, "materialized %d files into %s\n", n, *dst)
		return nil
	case "docs":
		fs := flag.NewFlagSet("docs", flag.ContinueOnError)
		fs.SetOutput(stderr)
		tree := fs.String("tree", "", "materialized tree, with gen/ts generated (required)")
		tmpl := fs.String("tmpl", "examples/user/EXAMPLE.md.tmpl", "template")
		out := fs.String("out", "docs/EXAMPLE.md", "output markdown file")
		module := fs.String("module", "", "current Go module path (default: read from ./go.mod)")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if *tree == "" {
			return fmt.Errorf("docs: -tree is required")
		}
		return writeDocs(*tree, *tmpl, *out, *module)
	default:
		return fmt.Errorf("unknown subcommand %q (want materialize or docs)", args[0])
	}
}
