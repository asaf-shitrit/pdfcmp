// Command crap is a CRAP (Change Risk Anti-Patterns) gate for Go.
//
//	CRAP(f) = comp(f)^2 * (1 - cov(f))^3 + comp(f)
//
// comp is cyclomatic complexity (from go/ast); cov is statement coverage from
// a coverprofile produced by:
//
//	go test -coverpkg=./... -coverprofile=coverage.out ./...
//
// Usage: go run ./scripts/crap [-profile coverage.out] [-threshold 30] [-top 10]
// Config: .crap.json at the module root ({ "threshold", "exclude", "profile" }).
// Exits 1 when any function scores above the threshold.
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

type config struct {
	Threshold float64  `json:"threshold"`
	Exclude   []string `json:"exclude"`
	Profile   string   `json:"profile"`
}

type block struct {
	startLine, endLine int
	stmts              int
	hit                bool
}

type result struct {
	file       string
	line       int
	name       string
	complexity int
	coverage   float64
	crap       float64
}

var defaultExclude = []string{"vendor", "testdata", ".git", "node_modules", "scripts/crap"}

func main() {
	cfg := loadConfig()
	profile := flag.String("profile", orDefault(cfg.Profile, "coverage.out"), "coverprofile path")
	threshold := flag.Float64("threshold", orDefaultF(cfg.Threshold, 30), "maximum allowed CRAP score")
	top := flag.Int("top", 10, "how many of the worst functions to print")
	flag.Parse()

	module, err := modulePath()
	if err != nil {
		fail(2, "crap: %v", err)
	}
	blocks, err := readProfile(*profile, module)
	if err != nil {
		fail(2, "crap: %v (run: go test -coverpkg=./... -coverprofile=%s ./...)", err, *profile)
	}
	exclude := append(defaultExclude, cfg.Exclude...)

	var results []result
	fset := token.NewFileSet()
	err = filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel := filepath.ToSlash(path)
		if excluded(rel, exclude) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() || !strings.HasSuffix(rel, ".go") || strings.HasSuffix(rel, "_test.go") {
			return nil
		}
		file, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
		if err != nil {
			return err
		}
		if ast.IsGenerated(file) {
			return nil
		}
		results = append(results, analyzeFile(fset, file, rel, blocks[rel])...)
		return nil
	})
	if err != nil {
		fail(2, "crap: %v", err)
	}
	os.Exit(report(results, *threshold, *top))
}

func loadConfig() config {
	var cfg config
	data, err := os.ReadFile(".crap.json")
	if err == nil {
		if err := json.Unmarshal(data, &cfg); err != nil {
			fail(2, "crap: invalid .crap.json: %v", err)
		}
	}
	return cfg
}

func modulePath() (string, error) {
	data, err := os.ReadFile("go.mod")
	if err != nil {
		return "", fmt.Errorf("go.mod not found; run from the module root")
	}
	for _, line := range strings.Split(string(data), "\n") {
		if fields := strings.Fields(line); len(fields) == 2 && fields[0] == "module" {
			return strings.Trim(fields[1], `"`), nil
		}
	}
	return "", fmt.Errorf("no module directive in go.mod")
}

// readProfile returns coverage blocks keyed by file path relative to the module root.
// Blocks repeated across test binaries (-coverpkg) are merged: hit if any run hit them.
func readProfile(path, module string) (map[string][]block, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	type key struct {
		file string
		pos  string
	}
	merged := map[key]*block{}
	var order []key
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "mode:") || line == "" {
			continue
		}
		// name.go:startLine.startCol,endLine.endCol numStmts count
		colon := strings.LastIndex(line, ":")
		fields := strings.Fields(line[colon+1:])
		if colon < 0 || len(fields) != 3 {
			return nil, fmt.Errorf("malformed profile line: %q", line)
		}
		file := strings.TrimPrefix(line[:colon], module+"/")
		span := strings.Split(fields[0], ",")
		stmts, _ := strconv.Atoi(fields[1])
		count, _ := strconv.Atoi(fields[2])
		k := key{file, fields[0]}
		if b, ok := merged[k]; ok {
			b.hit = b.hit || count > 0
			continue
		}
		merged[k] = &block{startLine: lineOf(span[0]), endLine: lineOf(span[1]), stmts: stmts, hit: count > 0}
		order = append(order, k)
	}
	byFile := map[string][]block{}
	for _, k := range order {
		byFile[k.file] = append(byFile[k.file], *merged[k])
	}
	return byFile, scanner.Err()
}

func lineOf(pos string) int {
	n, _ := strconv.Atoi(strings.SplitN(pos, ".", 2)[0])
	return n
}

func excluded(rel string, exclude []string) bool {
	for _, e := range exclude {
		e = strings.TrimSuffix(strings.TrimPrefix(e, "./"), "/")
		if rel == e || strings.HasPrefix(rel, e+"/") {
			return true
		}
	}
	return false
}

func analyzeFile(fset *token.FileSet, file *ast.File, rel string, blocks []block) []result {
	var out []result
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		start := fset.Position(fn.Pos()).Line
		end := fset.Position(fn.End()).Line
		comp := complexity(fn)
		cov := coverage(blocks, start, end)
		out = append(out, result{
			file: rel, line: start, name: funcName(fn),
			complexity: comp, coverage: cov, crap: crapScore(comp, cov),
		})
	}
	return out
}

func funcName(fn *ast.FuncDecl) string {
	if fn.Recv == nil || len(fn.Recv.List) == 0 {
		return fn.Name.Name
	}
	t := fn.Recv.List[0].Type
	if star, ok := t.(*ast.StarExpr); ok {
		t = star.X
	}
	if idx, ok := t.(*ast.IndexExpr); ok {
		t = idx.X
	}
	if idx, ok := t.(*ast.IndexListExpr); ok {
		t = idx.X
	}
	if ident, ok := t.(*ast.Ident); ok {
		return ident.Name + "." + fn.Name.Name
	}
	return fn.Name.Name
}

// complexity follows gocyclo: 1 + if, for, range, non-default case/comm clause, &&, ||.
// Function literals count toward the enclosing function.
func complexity(fn *ast.FuncDecl) int {
	c := 1
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.IfStmt, *ast.ForStmt, *ast.RangeStmt:
			c++
		case *ast.CaseClause:
			if n.List != nil {
				c++
			}
		case *ast.CommClause:
			if n.Comm != nil {
				c++
			}
		case *ast.BinaryExpr:
			if n.Op == token.LAND || n.Op == token.LOR {
				c++
			}
		}
		return true
	})
	return c
}

func coverage(blocks []block, start, end int) float64 {
	total, covered := 0, 0
	for _, b := range blocks {
		if b.startLine >= start && b.endLine <= end {
			total += b.stmts
			if b.hit {
				covered += b.stmts
			}
		}
	}
	if total == 0 {
		return 0
	}
	return float64(covered) / float64(total)
}

func crapScore(comp int, cov float64) float64 {
	c := float64(comp)
	return c*c*math.Pow(1-cov, 3) + c
}

func report(rows []result, threshold float64, top int) int {
	sort.Slice(rows, func(i, j int) bool { return rows[i].crap > rows[j].crap })
	var failing int
	for _, r := range rows {
		if r.crap > threshold {
			failing++
		}
	}
	shown := rows[:min(len(rows), max(top, failing))]
	fmt.Printf("CRAP report: %d functions, threshold %g\n", len(rows), threshold)
	if len(shown) > 0 {
		fmt.Println("  CRAP   comp  cov    function")
	}
	for _, r := range shown {
		mark := " "
		if r.crap > threshold {
			mark = "✗"
		}
		fmt.Printf("%s %5.1f  %4d  %3.0f%%  %s:%d %s\n", mark, r.crap, r.complexity, r.coverage*100, r.file, r.line, r.name)
	}
	if failing > 0 {
		fmt.Fprintf(os.Stderr, "\ncrap: %d function(s) above %g. Add tests for them or split them into simpler functions.\n", failing, threshold)
		return 1
	}
	fmt.Println("\ncrap: OK")
	return 0
}

func orDefault(s, d string) string {
	if s == "" {
		return d
	}
	return s
}

func orDefaultF(f, d float64) float64 {
	if f == 0 {
		return d
	}
	return f
}

func fail(code int, format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(code)
}
