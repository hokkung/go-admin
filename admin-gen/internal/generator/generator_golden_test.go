package generator

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hokkung/go-admin/admin-gen/internal/parser"
)

// -update rewrites the golden files instead of diffing against them. Run via
// `go test ./admin-gen/internal/generator/... -run TestGenerate_ -update`
// whenever an intentional template change lands. Never the default — silent
// goldens defeat the point.
var updateGolden = flag.Bool("update", false, "rewrite golden fixtures instead of comparing")

// TestGenerate_GivenAnnotatedFixture_WhenGenerated_ThenOutputMatchesGolden runs
// parser + generator against every testdata/<case>/ fixture and compares the
// generated files to testdata/<case>/golden/. New cases only need input
// models added — the golden dir is created by running the test once with
// -update.
func TestGenerate_GivenAnnotatedFixture_WhenGenerated_ThenOutputMatchesGolden(t *testing.T) {
	root, err := filepath.Abs("testdata")
	if err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		t.Run("given "+name+" fixture when generated then output matches golden", func(t *testing.T) {
			runGoldenCase(t, root, name)
		})
	}
}

func runGoldenCase(t *testing.T, root, caseName string) {
	t.Helper()
	inputDir := filepath.Join(root, caseName, "input")
	goldenDir := filepath.Join(root, caseName, "golden")

	entities, err := parser.Parse(parser.ParseOptions{
		Dir:         inputDir,
		PackagePath: "example.com/fixture/" + caseName,
	})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	outDir := t.TempDir()
	err = Generate(entities, Options{
		OutDir:       outDir,
		OutPackage:   "admin",
		ModelsImport: "example.com/fixture/" + caseName,
	})
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	actualFiles := listGoFiles(t, outDir)

	if *updateGolden {
		// Blow away the existing golden dir before rewriting so stale files
		// (for actions / entities that no longer exist) don't linger.
		if err := os.RemoveAll(goldenDir); err != nil && !os.IsNotExist(err) {
			t.Fatal(err)
		}
		if err := os.MkdirAll(goldenDir, 0o755); err != nil {
			t.Fatal(err)
		}
		for name, content := range actualFiles {
			if err := os.WriteFile(filepath.Join(goldenDir, name), content, 0o644); err != nil {
				t.Fatal(err)
			}
		}
		return
	}

	expectedFiles := readAllGoFiles(t, goldenDir)

	for name, want := range expectedFiles {
		got, ok := actualFiles[name]
		if !ok {
			t.Errorf("missing generated file: %s", name)
			continue
		}
		if string(got) != string(want) {
			t.Errorf("file %s does not match golden.\nRerun with -update if the template change is intentional.\n--- diff (first mismatch around): ---\n%s", name, firstDiff(string(want), string(got)))
		}
	}
	for name := range actualFiles {
		if _, ok := expectedFiles[name]; !ok {
			t.Errorf("unexpected generated file (not in golden): %s", name)
		}
	}
}

func listGoFiles(t *testing.T, dir string) map[string][]byte {
	t.Helper()
	out := map[string][]byte{}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		out[e.Name()] = b
	}
	return out
}

func readAllGoFiles(t *testing.T, dir string) map[string][]byte {
	t.Helper()
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		t.Fatalf("golden dir missing: %s\nRun the test with -update to create it.", dir)
	}
	return listGoFiles(t, dir)
}

// firstDiff returns a small slice of the want/got strings around the first
// mismatching line so the error output stays readable. A full diff would
// just drown the console on any whitespace-only change.
func firstDiff(want, got string) string {
	wantLines := strings.Split(want, "\n")
	gotLines := strings.Split(got, "\n")
	for i := 0; i < len(wantLines) && i < len(gotLines); i++ {
		if wantLines[i] != gotLines[i] {
			return renderContext(wantLines, gotLines, i)
		}
	}
	if len(wantLines) != len(gotLines) {
		return "file lengths differ: want " +
			itoa(len(wantLines)) + " lines, got " + itoa(len(gotLines))
	}
	return "(no line-level difference found — likely trailing whitespace or BOM)"
}

func renderContext(want, got []string, line int) string {
	start := line - 2
	if start < 0 {
		start = 0
	}
	end := line + 3
	if end > len(want) {
		end = len(want)
	}
	var b strings.Builder
	b.WriteString("line " + itoa(line+1) + ":\n")
	b.WriteString("want: " + want[line] + "\n")
	if line < len(got) {
		b.WriteString("got:  " + got[line] + "\n")
	}
	b.WriteString("\ncontext (want):\n")
	for i := start; i < end && i < len(want); i++ {
		marker := "  "
		if i == line {
			marker = "> "
		}
		b.WriteString(marker + want[i] + "\n")
	}
	return b.String()
}

// itoa is a tiny reimplementation to avoid pulling in strconv just for a
// debug message. Go inlines this easily and there's no Atoi on the other
// side of the conversion.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := false
	if n < 0 {
		neg = true
		n = -n
	}
	buf := [20]byte{}
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
