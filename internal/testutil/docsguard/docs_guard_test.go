//go:build test

// Package docsguard checks the task documents under docs/tasks for defects that
// a Markdown renderer hides and a reviewer reads past. Prose has no compiler, so
// the few properties that can be checked mechanically are checked here.
package docsguard

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

// legacyTableFiles predate this guard and contain table rows whose cell counts
// disagree. They are exempt so the guard can be added without rewriting finished
// tasks. The list may only shrink: a file that has been repaired must be removed,
// which the test enforces.
var legacyTableFiles = map[string]struct{}{
	"0136_runtime_risk_evaluation_enforcement/03_implementation_plan.md":         {},
	"0137_package_manager_modification_detection/03_implementation_plan.md":      {},
	"0142_axis2_destination_zoning/03_implementation_plan.md":                    {},
	"0145_per_command_flag_accuracy/03_implementation_plan.md":                   {},
	"0153_failopen_error_handling_crosscut/03_implementation_plan.md":            {},
	"0158_dryrun_runas_ident_unification/03_implementation_plan.md":              {},
	"0161_sudo_uid_validation_and_logging/03_implementation_plan.md":             {},
	"0162_entrypoint_runid_privilege_toctou_hardening/03_implementation_plan.md": {},
	"0166_privilege_a1_low_remaining/03_implementation_plan.md":                  {},
	"0167_safefileio_b1_remaining/03_implementation_plan.md":                     {},
	"0170_excess_synchronization_removal/02_architecture.md":                     {},
}

// legacyStatusFiles predate this guard and have a Document Status header whose
// Review date or Reviewer disagrees with its Status. Same shrink-only rule.
var legacyStatusFiles = map[string]struct{}{
	"0156_env_denylist_consolidation/01_requirements.md": {},
	"0171_privilege_gap_narrowing/02_architecture.md":    {},
}

// statusCellRE matches one row of the Document Status table, e.g. "| Status | `draft` |".
var statusCellRE = regexp.MustCompile(`(?m)^\|\s*([^|]+?)\s*\|\s*(.*?)\s*\|\s*$`)

// statusHeading starts the section whose table this guard reads. Rows are taken
// only from there, so a "Status" row in some other table is not mistaken for it.
const statusHeading = "## Document Status"

func TestTaskDocs_TableRowsHaveMatchingCellCounts(t *testing.T) {
	t.Parallel()

	violated := make(map[string]struct{})
	for _, doc := range processDocs(t) {
		var offenders []int
		for _, block := range tableBlocks(doc.body) {
			want := block[0].pipes
			for _, row := range block[1:] {
				if row.pipes != want {
					offenders = append(offenders, row.line)
				}
			}
		}
		if len(offenders) == 0 {
			continue
		}
		violated[doc.rel] = struct{}{}
		if _, exempt := legacyTableFiles[doc.rel]; exempt {
			continue
		}
		// A pipe inside a cell splits it: GFM divides a row on "|" before it parses
		// inline code, so `rg a | rg b` in a table cell silently truncates the
		// command and adds a column.
		t.Errorf("%s: table rows at lines %v have a different cell count from their header; escape any \"|\" inside a cell as \"\\|\"", doc.rel, offenders)
	}
	reportRepaired(t, legacyTableFiles, violated, "legacyTableFiles")
}

func TestTaskDocs_StatusHeaderMatchesStatus(t *testing.T) {
	t.Parallel()

	violated := make(map[string]struct{})
	for _, doc := range processDocs(t) {
		status, reviewDate, reviewer := statusHeader(doc.body)
		if status == "" {
			continue
		}
		var problem string
		switch status {
		case "draft":
			if reviewDate != "-" || reviewer != "-" {
				problem = "status is draft, so Review date and Reviewer must both be \"-\""
			}
		case "approved", "completed":
			if reviewDate == "-" || reviewer == "-" || reviewDate == "" || reviewer == "" {
				problem = "status is " + status + ", so Review date and Reviewer must both be filled in"
			}
		default:
			problem = "status must be draft, approved or completed, got " + status
		}
		if problem == "" {
			continue
		}
		violated[doc.rel] = struct{}{}
		if _, exempt := legacyStatusFiles[doc.rel]; exempt {
			continue
		}
		t.Errorf("%s: %s (see docs/dev/developer_guide/requirements_process.md)", doc.rel, problem)
	}
	reportRepaired(t, legacyStatusFiles, violated, "legacyStatusFiles")
}

// reportRepaired fails when an exempt file no longer violates, so the exemption
// lists cannot outlive the defects they cover.
func reportRepaired(t *testing.T, exempt, violated map[string]struct{}, listName string) {
	t.Helper()
	for rel := range exempt {
		if _, still := violated[rel]; !still {
			t.Errorf("%s is listed in %s but no longer violates; remove it from the list", rel, listName)
		}
	}
}

type doc struct {
	rel  string
	body string
}

// processDocs returns the task documents that follow the current process, which
// is what having a Document Status table marks. Older documents predate it and
// are out of scope.
func processDocs(t *testing.T) []doc {
	t.Helper()

	tasksDir := filepath.Join(repoRoot(t), "docs", "tasks")
	var docs []doc
	err := filepath.WalkDir(tasksDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".md") {
			return nil
		}
		body, err := os.ReadFile(path) //nolint:gosec // path comes from WalkDir over the repository's own docs
		if err != nil {
			return err
		}
		if !strings.Contains(string(body), "## Document Status") {
			return nil
		}
		rel, err := filepath.Rel(tasksDir, path)
		if err != nil {
			return err
		}
		docs = append(docs, doc{rel: filepath.ToSlash(rel), body: string(body)})
		return nil
	})
	if err != nil {
		t.Fatalf("walking %s: %v", tasksDir, err)
	}
	if len(docs) == 0 {
		t.Fatalf("no task documents with a Document Status section found under %s", tasksDir)
	}
	return docs
}

// repoRoot walks up from this source file until it finds go.mod, so the guard
// does not depend on how deep its own package sits.
func repoRoot(t *testing.T) string {
	t.Helper()

	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot determine the path of this source file")
	}
	dir := filepath.Dir(file)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("no go.mod found above %s", filepath.Dir(file))
		}
		dir = parent
	}
}

type tableRow struct {
	line  int
	pipes int
}

// tableBlocks returns each run of consecutive table rows, skipping fenced code
// blocks so that Mermaid diagrams and shell samples are not read as tables.
func tableBlocks(body string) [][]tableRow {
	var blocks [][]tableRow
	var current []tableRow
	inFence := false

	flush := func() {
		if len(current) >= 2 {
			blocks = append(blocks, current)
		}
		current = nil
	}

	for i, line := range strings.Split(body, "\n") {
		trimmed := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(trimmed, "```"):
			inFence = !inFence
			flush()
		case inFence:
			// Nothing: fenced content is not Markdown table syntax.
		case strings.HasPrefix(trimmed, "|"):
			current = append(current, tableRow{line: i + 1, pipes: countCellSeparators(trimmed)})
		default:
			flush()
		}
	}
	flush()
	return blocks
}

// countCellSeparators counts the "|" characters that actually split cells; a
// backslash-escaped one does not.
func countCellSeparators(line string) int {
	count := 0
	for i := 0; i < len(line); i++ {
		if line[i] == '\\' {
			i++
			continue
		}
		if line[i] == '|' {
			count++
		}
	}
	return count
}

// statusHeader extracts Status, Review date and Reviewer from the Document
// Status table. It returns empty strings when the document has no such section.
func statusHeader(body string) (status, reviewDate, reviewer string) {
	_, section, found := strings.Cut(body, statusHeading)
	if !found {
		return "", "", ""
	}
	// The table is the first thing in the section; stop at the next heading so a
	// later table cannot contribute rows.
	if next, _, ok := strings.Cut(section, "\n## "); ok {
		section = next
	}

	for _, m := range statusCellRE.FindAllStringSubmatch(section, -1) {
		value := strings.Trim(m[2], "`")
		// A value may carry a parenthetical note; the status itself is what precedes it.
		if before, _, ok := strings.Cut(value, "（"); ok {
			value = strings.TrimSpace(before)
		}
		switch m[1] {
		case "Status":
			status = value
		case "Review date":
			reviewDate = value
		case "Reviewer":
			reviewer = value
		}
	}
	return status, reviewDate, reviewer
}
