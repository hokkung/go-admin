// admin-gen generates plain Go handler, store, and routing code from
// //admin:generate-annotated structs. Run from your module root:
//
//	admin-gen -in ./internal/models -out ./internal/admin
//
// The tool reads go.mod to resolve import paths, so you do not supply them
// manually.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/hokkung/go-admin/admin-gen/internal/generator"
	"github.com/hokkung/go-admin/admin-gen/internal/parser"
)

func main() {
	var (
		inDir         = flag.String("in", "", "directory containing annotated Go source (required)")
		outDir        = flag.String("out", "", "directory to write generated files into (required)")
		modulePath    = flag.String("module", "", "override go.mod module path (usually autodetected)")
		runtimeImport = flag.String("runtime", "", "override the runtime import path (optional)")
	)
	flag.Parse()

	if *inDir == "" || *outDir == "" {
		fmt.Fprintln(os.Stderr, "usage: admin-gen -in <models-dir> -out <generated-dir>")
		flag.PrintDefaults()
		os.Exit(2)
	}

	absIn, err := filepath.Abs(*inDir)
	if err != nil {
		fail("resolve -in: %v", err)
	}
	absOut, err := filepath.Abs(*outDir)
	if err != nil {
		fail("resolve -out: %v", err)
	}

	fmt.Println("absIn: ", absIn)
	fmt.Println("absOut: ", absOut)

	modRoot, modName := *modulePath, ""
	if modRoot == "" {
		root, name, err := findGoMod(absIn)
		if err != nil {
			fail("%v (pass -module to override)", err)
		}
		modRoot, modName = root, name
	} else {
		modName = *modulePath
		// If module was overridden, try to locate the go.mod root by walking up.
		if r, _, err := findGoMod(absIn); err == nil {
			modRoot = r
		}
	}

	modelsImport, err := importPathFor(modRoot, modName, absIn)
	if err != nil {
		fail("compute models import: %v", err)
	}

	entities, err := parser.Parse(parser.ParseOptions{
		Dir:         absIn,
		PackagePath: modelsImport,
	})
	if err != nil {
		fail("parse: %v", err)
	}
	if len(entities) == 0 {
		fmt.Fprintln(os.Stderr, "admin-gen: no //admin:generate structs found in", absIn)
		os.Exit(1)
	}

	opts := generator.Options{
		OutDir:        absOut,
		OutPackage:    filepath.Base(absOut),
		ModelsImport:  modelsImport,
		RuntimeImport: *runtimeImport,
	}
	if err := generator.Generate(entities, opts); err != nil {
		fail("generate: %v", err)
	}

	fmt.Printf("admin-gen: wrote %d entity handler file(s) + register.go to %s\n", len(entities), absOut)
	for _, e := range entities {
		fmt.Printf("  - %s (%s)\n", e.GoName, e.Name)
	}
}

// findGoMod walks up from start looking for go.mod. Returns the directory
// containing go.mod and the module path declared inside it.
func findGoMod(start string) (dir string, modulePath string, err error) {
	cur := start
	for {
		candidate := filepath.Join(cur, "go.mod")
		if data, err := os.ReadFile(candidate); err == nil {
			mod := parseModulePath(string(data))
			if mod == "" {
				return "", "", fmt.Errorf("go.mod at %s has no module directive", candidate)
			}
			return cur, mod, nil
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return "", "", fmt.Errorf("no go.mod found at or above %s", start)
		}
		cur = parent
	}
}

func parseModulePath(goMod string) string {
	for _, line := range strings.Split(goMod, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "module ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "module"))
		}
	}
	return ""
}

// importPathFor converts an absolute directory to the Go import path that
// references it, using the enclosing module's path as the prefix.
func importPathFor(modRoot, modulePath, absDir string) (string, error) {
	rel, err := filepath.Rel(modRoot, absDir)
	if err != nil {
		return "", err
	}
	if rel == "." {
		return modulePath, nil
	}
	// Use forward slashes regardless of host OS — import paths are not
	// filepaths.
	return modulePath + "/" + filepath.ToSlash(rel), nil
}

func fail(format string, args ...any) {
	fmt.Fprintln(os.Stderr, "admin-gen:", fmt.Sprintf(format, args...))
	os.Exit(1)
}
