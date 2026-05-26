package archtest

import (
	"bufio"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

type capability struct {
	ID               string
	AllowedPackages  []string
	ForbiddenImports []string
	ForbiddenSymbols []string
}

type acknowledgmentRow struct {
	Section  string
	Consumed string
	Decision string
	Becomes  string
}

type acknowledgmentCoverage struct {
	acknowledgmentRow
	TestID         string
	Implementation string
}

type implementedAcknowledgmentTest struct {
	TestID string
	File   string
	Test   string
}

func TestCapabilityIDsAreUnique(t *testing.T) {
	capabilities := loadCapabilities(t)

	seen := make(map[string]bool, len(capabilities))
	for _, capability := range capabilities {
		if capability.ID == "" {
			t.Fatalf("capability id is empty")
		}
		if seen[capability.ID] {
			t.Fatalf("duplicate capability id %q", capability.ID)
		}
		seen[capability.ID] = true
	}
}

func TestCommandSurfaceRejectsRunRegistrationForms(t *testing.T) {
	capabilities := loadCapabilities(t)
	commandSurface := findCapability(t, capabilities, "command-surface")

	for _, symbol := range []string{`zerobased run`, `Use: "run"`, `Use: 'run'`, `case "run":`, `case 'run':`} {
		if !contains(commandSurface.ForbiddenSymbols, symbol) {
			t.Fatalf("command-surface forbidden symbols missing %q", symbol)
		}
	}
}

func TestCommandSourceDoesNotRegisterRunCommand(t *testing.T) {
	for _, file := range goSourceFilesUnder(t, "cmd") {
		for _, literal := range stringLiterals(t, file) {
			firstWord := strings.Fields(literal)
			if len(firstWord) > 0 && firstWord[0] == "run" {
				t.Fatalf("%s contains forbidden run command string %q", file, literal)
			}
			if strings.Contains(literal, "zerobased run") {
				t.Fatalf("%s contains forbidden run command string %q", file, literal)
			}
		}
	}
}

func TestForbiddenSymbolsAreAbsentFromRuntimeSource(t *testing.T) {
	capabilities := loadCapabilities(t)
	files := runtimeSourceFiles(t)

	for _, capability := range capabilities {
		scannedFiles := filesOutsideAllowedPackages(t, files, capability.AllowedPackages)
		for _, symbol := range capability.ForbiddenSymbols {
			assertAbsent(t, scannedFiles, symbol)
		}
	}
}

func TestForbiddenImportsAreAbsentFromRuntimeSource(t *testing.T) {
	capabilities := loadCapabilities(t)
	files := runtimeSourceFiles(t)

	for _, capability := range capabilities {
		if capability.ID == "repo-up-cannot-import-route-runtime" {
			continue
		}
		scannedFiles := filesOutsideAllowedPackages(t, files, capability.AllowedPackages)
		for _, importPath := range capability.ForbiddenImports {
			assertAbsent(t, scannedFiles, importPrefix(importPath))
		}
	}
}

func TestAcknowledgmentCoverageMatchesTracedTDDRFC(t *testing.T) {
	rfcRows := loadAcknowledgmentRowsFromRFC(t)
	coverageRows := loadAcknowledgmentCoverage(t)

	coverageByKey := make(map[string]acknowledgmentCoverage, len(coverageRows))
	for _, row := range coverageRows {
		if row.TestID == "" {
			t.Fatalf("acknowledgment coverage row has empty test_id: %#v", row.acknowledgmentRow)
		}
		if row.Implementation != "planned" && row.Implementation != "implemented" {
			t.Fatalf("acknowledgment coverage row %s has invalid implementation status %q", row.TestID, row.Implementation)
		}
		coverageByKey[acknowledgmentKey(row.acknowledgmentRow)] = row
	}

	rfcByKey := make(map[string]acknowledgmentRow, len(rfcRows))
	for _, row := range rfcRows {
		key := acknowledgmentKey(row)
		rfcByKey[key] = row
		if _, ok := coverageByKey[key]; !ok {
			t.Fatalf("missing acknowledgment coverage for %s / %s", row.Section, row.Consumed)
		}
	}

	for _, row := range coverageRows {
		if _, ok := rfcByKey[acknowledgmentKey(row.acknowledgmentRow)]; !ok {
			t.Fatalf("stale acknowledgment coverage row not found in RFC: %#v", row.acknowledgmentRow)
		}
	}
}

func TestImplementedAcknowledgmentTestsExist(t *testing.T) {
	coverageRows := loadAcknowledgmentCoverage(t)
	implementedTests := loadImplementedAcknowledgmentTests(t)

	coverageByTestID := make(map[string]acknowledgmentCoverage, len(coverageRows))
	for _, row := range coverageRows {
		coverageByTestID[row.TestID] = row
	}
	implementedByTestID := make(map[string]implementedAcknowledgmentTest, len(implementedTests))
	for _, implemented := range implementedTests {
		implementedByTestID[implemented.TestID] = implemented
	}

	for _, row := range coverageRows {
		_, implemented := implementedByTestID[row.TestID]
		switch row.Implementation {
		case "implemented":
			if !implemented {
				t.Fatalf("coverage row %q is marked implemented but missing from implemented_tests", row.TestID)
			}
		case "planned":
			if implemented {
				t.Fatalf("coverage row %q is planned but present in implemented_tests", row.TestID)
			}
		default:
			t.Fatalf("coverage row %q has invalid implementation status %q", row.TestID, row.Implementation)
		}
	}

	for _, implemented := range implementedTests {
		if implemented.TestID == "" || implemented.File == "" || implemented.Test == "" {
			t.Fatalf("implemented acknowledgment test has empty field: %#v", implemented)
		}
		row, ok := coverageByTestID[implemented.TestID]
		if !ok {
			t.Fatalf("implemented test id %q is not present in acknowledgment coverage rows", implemented.TestID)
		}
		if row.Implementation != "implemented" {
			t.Fatalf("implemented test id %q maps to coverage row with status %q", implemented.TestID, row.Implementation)
		}

		content, err := os.ReadFile(filepath.Join(repoRoot(t), implemented.File))
		if err != nil {
			t.Fatalf("read implemented test file %s: %v", implemented.File, err)
		}
		if !strings.Contains(string(content), "func "+implemented.Test+"(") {
			t.Fatalf("implemented test %q not found in %s", implemented.Test, implemented.File)
		}
	}
}

func TestRepoLocalUpCannotImportRouteRuntime(t *testing.T) {
	capabilities := loadCapabilities(t)
	files := repoLocalUpSourceFiles(t, capabilities)

	for _, file := range files {
		assertFileDoesNotContain(t, file, "internal/route_runtime")
	}
}

func loadCapabilities(t *testing.T) []capability {
	t.Helper()

	file, err := os.Open(filepath.Join(repoRoot(t), "docs", "superpowers", "specs", "zerobased-capabilities.yaml"))
	if err != nil {
		t.Fatalf("open capability allowlist: %v", err)
	}
	defer file.Close()

	var capabilities []capability
	var current *capability
	var list string

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		if strings.HasPrefix(line, "- id:") {
			capabilities = append(capabilities, capability{
				ID: strings.TrimSpace(strings.TrimPrefix(line, "- id:")),
			})
			current = &capabilities[len(capabilities)-1]
			list = ""
			continue
		}
		if current == nil {
			continue
		}

		if strings.HasSuffix(line, ":") {
			list = strings.TrimSuffix(line, ":")
			continue
		}
		if strings.Contains(line, ":") && !strings.HasPrefix(line, "- ") {
			list = ""
			continue
		}
		if !strings.HasPrefix(line, "- ") {
			continue
		}

		value := unquoteScalar(strings.TrimSpace(strings.TrimPrefix(line, "- ")))
		switch list {
		case "allowed_packages":
			current.AllowedPackages = append(current.AllowedPackages, value)
		case "forbidden_imports":
			current.ForbiddenImports = append(current.ForbiddenImports, value)
		case "forbidden_symbols":
			current.ForbiddenSymbols = append(current.ForbiddenSymbols, value)
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan capability allowlist: %v", err)
	}
	if len(capabilities) == 0 {
		t.Fatalf("no capabilities parsed")
	}

	return capabilities
}

func loadAcknowledgmentRowsFromRFC(t *testing.T) []acknowledgmentRow {
	t.Helper()

	path := filepath.Join(repoRoot(t), "docs", "superpowers", "specs", "2026-05-13-zerobased-traced-tdd-rfc.md")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read traced-TDD RFC: %v", err)
	}

	var rows []acknowledgmentRow
	var section string
	inAcknowledgmentTable := false
	for _, line := range strings.Split(string(content), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "### ") {
			section = strings.TrimSpace(strings.TrimPrefix(line, "### "))
			inAcknowledgmentTable = false
			continue
		}
		if strings.HasPrefix(line, "| Consumed ") {
			inAcknowledgmentTable = true
			continue
		}
		if !inAcknowledgmentTable {
			continue
		}
		if !strings.HasPrefix(line, "|") {
			inAcknowledgmentTable = false
			continue
		}
		if strings.HasPrefix(line, "| ---") {
			continue
		}

		cells := splitMarkdownRow(line)
		if len(cells) < 3 {
			t.Fatalf("invalid acknowledgment table row in %s: %q", section, line)
		}
		rows = append(rows, acknowledgmentRow{
			Section:  section,
			Consumed: cells[0],
			Decision: cells[1],
			Becomes:  cells[2],
		})
	}
	if len(rows) == 0 {
		t.Fatalf("no acknowledgment rows parsed from RFC")
	}
	return rows
}

func loadAcknowledgmentCoverage(t *testing.T) []acknowledgmentCoverage {
	t.Helper()

	path := filepath.Join(repoRoot(t), "docs", "superpowers", "specs", "zerobased-acknowledgment-coverage.yaml")
	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("open acknowledgment coverage: %v", err)
	}
	defer file.Close()

	var rows []acknowledgmentCoverage
	var current *acknowledgmentCoverage
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "- section:") {
			rows = append(rows, acknowledgmentCoverage{
				acknowledgmentRow: acknowledgmentRow{
					Section: yamlValue(line, "- section:"),
				},
			})
			current = &rows[len(rows)-1]
			continue
		}
		if current == nil {
			continue
		}
		switch {
		case strings.HasPrefix(line, "consumed:"):
			current.Consumed = yamlValue(line, "consumed:")
		case strings.HasPrefix(line, "decision:"):
			current.Decision = yamlValue(line, "decision:")
		case strings.HasPrefix(line, "becomes:"):
			current.Becomes = yamlValue(line, "becomes:")
		case strings.HasPrefix(line, "test_id:"):
			current.TestID = yamlValue(line, "test_id:")
		case strings.HasPrefix(line, "implementation:"):
			current.Implementation = yamlValue(line, "implementation:")
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan acknowledgment coverage: %v", err)
	}
	if len(rows) == 0 {
		t.Fatalf("no acknowledgment coverage rows parsed")
	}
	return rows
}

func loadImplementedAcknowledgmentTests(t *testing.T) []implementedAcknowledgmentTest {
	t.Helper()

	path := filepath.Join(repoRoot(t), "docs", "superpowers", "specs", "zerobased-acknowledgment-coverage.yaml")
	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("open acknowledgment coverage: %v", err)
	}
	defer file.Close()

	var tests []implementedAcknowledgmentTest
	var current *implementedAcknowledgmentTest
	inImplementedTests := false
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		switch line {
		case "implemented_tests:":
			inImplementedTests = true
			continue
		case "rows:":
			inImplementedTests = false
			continue
		}
		if !inImplementedTests {
			continue
		}
		if strings.HasPrefix(line, "- test_id:") {
			tests = append(tests, implementedAcknowledgmentTest{
				TestID: yamlValue(line, "- test_id:"),
			})
			current = &tests[len(tests)-1]
			continue
		}
		if current == nil {
			continue
		}
		switch {
		case strings.HasPrefix(line, "file:"):
			current.File = yamlValue(line, "file:")
		case strings.HasPrefix(line, "test:"):
			current.Test = yamlValue(line, "test:")
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan implemented acknowledgment tests: %v", err)
	}
	if len(tests) == 0 {
		t.Fatalf("no implemented acknowledgment tests parsed")
	}
	return tests
}

func splitMarkdownRow(line string) []string {
	line = strings.TrimSpace(line)
	line = strings.TrimPrefix(line, "|")
	line = strings.TrimSuffix(line, "|")

	rawCells := strings.Split(line, "|")
	cells := make([]string, 0, len(rawCells))
	for _, cell := range rawCells {
		cells = append(cells, strings.TrimSpace(cell))
	}
	return cells
}

func acknowledgmentKey(row acknowledgmentRow) string {
	return row.Section + "\x00" + row.Consumed + "\x00" + row.Decision + "\x00" + row.Becomes
}

func yamlValue(line string, prefix string) string {
	return unquoteScalar(strings.TrimSpace(strings.TrimPrefix(line, prefix)))
}

func runtimeSourceFiles(t *testing.T) []string {
	t.Helper()

	root := repoRoot(t)
	var files []string
	for _, dir := range []string{"cmd", "internal"} {
		base := filepath.Join(root, dir)
		if _, err := os.Stat(base); os.IsNotExist(err) {
			continue
		}
		err := filepath.WalkDir(base, func(path string, entry os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() {
				if filepath.ToSlash(path) == filepath.ToSlash(filepath.Join(root, "internal", "archtest")) {
					return filepath.SkipDir
				}
				return nil
			}
			if strings.HasSuffix(path, ".go") && !strings.HasSuffix(path, "_test.go") {
				files = append(files, path)
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", dir, err)
		}
	}
	return files
}

func repoLocalUpSourceFiles(t *testing.T, capabilities []capability) []string {
	t.Helper()

	root := repoRoot(t)
	var files []string
	for _, cap := range capabilities {
		if cap.ID != "repo-up-cannot-import-route-runtime" {
			continue
		}
		for _, pattern := range cap.AllowedPackages {
			dir := filepath.Join(root, strings.TrimSuffix(pattern, "/**"))
			if _, err := os.Stat(dir); os.IsNotExist(err) {
				continue
			}
			files = append(files, goSourceFilesInDir(t, dir)...)
		}
	}
	return files
}

func findCapability(t *testing.T, capabilities []capability, id string) capability {
	t.Helper()
	for _, capability := range capabilities {
		if capability.ID == id {
			return capability
		}
	}
	t.Fatalf("capability %q not found", id)
	return capability{}
}

func contains(values []string, needle string) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
}

func filesOutsideAllowedPackages(t *testing.T, files []string, allowedPackages []string) []string {
	t.Helper()

	if len(allowedPackages) == 0 {
		return files
	}

	root := repoRoot(t)
	var filtered []string
	for _, file := range files {
		allowed := false
		for _, pattern := range allowedPackages {
			if pathMatchesPackagePattern(root, file, pattern) {
				allowed = true
				break
			}
		}
		if !allowed {
			filtered = append(filtered, file)
		}
	}
	return filtered
}

func pathMatchesPackagePattern(root string, file string, pattern string) bool {
	trimmed := strings.TrimSuffix(pattern, "/**")
	allowedRoot := filepath.Join(root, filepath.FromSlash(trimmed))
	rel, err := filepath.Rel(allowedRoot, file)
	if err != nil {
		return false
	}
	return rel == "." || (!strings.HasPrefix(rel, "..") && !filepath.IsAbs(rel))
}

func goSourceFilesUnder(t *testing.T, dir string) []string {
	t.Helper()

	root := repoRoot(t)
	base := filepath.Join(root, filepath.FromSlash(dir))
	if _, err := os.Stat(base); os.IsNotExist(err) {
		return nil
	}
	return goSourceFilesInDir(t, base)
}

func goSourceFilesInDir(t *testing.T, dir string) []string {
	t.Helper()

	var files []string
	err := filepath.WalkDir(dir, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		if strings.HasSuffix(path, ".go") && !strings.HasSuffix(path, "_test.go") {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", dir, err)
	}
	return files
}

func stringLiterals(t *testing.T, file string) []string {
	t.Helper()

	parsed, err := parser.ParseFile(token.NewFileSet(), file, nil, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", file, err)
	}

	var literals []string
	ast.Inspect(parsed, func(node ast.Node) bool {
		lit, ok := node.(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return true
		}
		value, err := strconv.Unquote(lit.Value)
		if err != nil {
			t.Fatalf("unquote %s in %s: %v", lit.Value, file, err)
		}
		literals = append(literals, value)
		return true
	})
	return literals
}

func assertAbsent(t *testing.T, files []string, needle string) {
	t.Helper()
	if needle == "" {
		return
	}
	for _, file := range files {
		assertFileDoesNotContain(t, file, needle)
	}
}

func assertFileDoesNotContain(t *testing.T, file string, needle string) {
	t.Helper()

	content, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("read %s: %v", file, err)
	}
	if strings.Contains(string(content), needle) {
		t.Fatalf("%s contains forbidden capability token %q", file, needle)
	}
}

func importPrefix(pattern string) string {
	return strings.TrimSuffix(pattern, "/**")
}

func unquoteScalar(value string) string {
	if len(value) < 2 {
		return value
	}
	if (value[0] == '\'' && value[len(value)-1] == '\'') || (value[0] == '"' && value[len(value)-1] == '"') {
		return value[1 : len(value)-1]
	}
	return value
}

func repoRoot(t *testing.T) string {
	t.Helper()

	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("get cwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("repo root not found from cwd")
		}
		dir = parent
	}
}
