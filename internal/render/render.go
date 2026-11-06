// Package render turns a plan Summary into a Markdown PR comment.
package render

import (
	"fmt"
	"sort"
	"strings"

	"github.com/moveeeax/tf-plan-summary/internal/plan"
)

// DefaultMarker is the hidden HTML comment used to find the sticky comment on
// re-runs. Keep it stable across versions.
const DefaultMarker = "<!-- tf-plan-summary -->"

// MaxBodyBytes bounds the rendered comment. GitHub rejects issue comments over
// 65536 characters outright, so a large plan would otherwise fail to post at
// all; the details table is truncated instead, riskiest rows kept first.
const MaxBodyBytes = 60000

// truncationReserve is the room held back from the budget for the truncation
// notice and the closing tags.
const truncationReserve = 256

var symbol = map[plan.Action]string{
	plan.Create:  "+",
	plan.Update:  "~",
	plan.Replace: "±",
	plan.Destroy: "-",
	plan.Forget:  "⊘",
	plan.Unknown: "?",
}

// Markdown renders the summary. The marker is embedded (hidden) so the poster
// can locate and update the same comment next time.
func Markdown(s plan.Summary, marker string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s\n", marker)
	b.WriteString("## Terraform Plan Summary\n\n")

	if s.Total() == 0 {
		b.WriteString("No changes. Infrastructure matches the configuration.\n")
		return b.String()
	}

	parts := []string{
		fmt.Sprintf("**%d** to add", s.Counts[plan.Create]),
		fmt.Sprintf("**%d** to change", s.Counts[plan.Update]),
		fmt.Sprintf("**%d** to replace", s.Counts[plan.Replace]),
		fmt.Sprintf("**%d** to destroy", s.Counts[plan.Destroy]),
	}
	if n := s.Counts[plan.Forget]; n > 0 {
		parts = append(parts, fmt.Sprintf("**%d** to forget", n))
	}
	if n := s.Counts[plan.Unknown]; n > 0 {
		parts = append(parts, fmt.Sprintf("**%d** unclassified", n))
	}
	fmt.Fprintf(&b, "%s.\n\n", strings.Join(parts, ", "))

	if s.HasCriticalRisk() {
		writeCriticalCallout(&b, s)
	}
	if s.HasRisk() {
		fmt.Fprintf(&b, "> ⚠️ **Review carefully:** %s.\n\n", strings.Join(riskParts(s), ", "))
	}
	if n := s.RiskIgnored(); n > 0 {
		fmt.Fprintf(&b, "_%d risky change(s) exempted by `--ignore-risk`; they are still listed below._\n\n", n)
	}

	writeTable(&b, s.Changes)
	return b.String()
}

// maxCalloutAddresses caps the protected addresses named in the callout; the
// table lists them all.
const maxCalloutAddresses = 10

// writeCriticalCallout names the protected resources a plan destroys or
// replaces, above the ordinary risk callout.
func writeCriticalCallout(b *strings.Builder, s plan.Summary) {
	var parts, addrs []string
	for _, a := range []plan.Action{plan.Replace, plan.Destroy} {
		if n := s.ProtectedCount(a); n > 0 {
			parts = append(parts, fmt.Sprintf("%d to %s", n, a))
		}
	}
	for _, c := range s.Changes {
		if c.Protected && (c.Action == plan.Destroy || c.Action == plan.Replace) {
			addrs = append(addrs, code(c.Address))
		}
	}
	more := ""
	if len(addrs) > maxCalloutAddresses {
		more = fmt.Sprintf(" and %d more", len(addrs)-maxCalloutAddresses)
		addrs = addrs[:maxCalloutAddresses]
	}
	fmt.Fprintf(b, "> [!CAUTION]\n> **Protected resources at risk:** %s: %s%s.\n\n",
		strings.Join(parts, ", "), strings.Join(addrs, ", "), more)
}

// riskParts lists the non-zero destructive counts for the callout, leaving out
// changes a policy exempted from the risk gate.
func riskParts(s plan.Summary) []string {
	var out []string
	for _, a := range []plan.Action{plan.Replace, plan.Destroy, plan.Forget} {
		if n := s.RiskCount(a); n > 0 {
			out = append(out, fmt.Sprintf("%d to %s", n, a))
		}
	}
	return out
}

// maxWhyPaths caps the attribute paths listed in one "Why" cell; a resource
// replaced because of dozens of nested blocks would otherwise swamp the table.
const maxWhyPaths = 5

// reasonText describes Terraform's action_reason codes. Unknown codes (newer
// Terraform versions add them) are shown verbatim.
var reasonText = map[string]string{
	"replace_because_tainted":           "tainted",
	"replace_by_request":                "replace requested (`-replace`)",
	"replace_by_triggers":               "`replace_triggered_by`",
	"replace_because_cannot_update":     "cannot update in place",
	"delete_because_no_resource_config": "removed from configuration",
	"delete_because_no_module":          "module instance removed",
	"delete_because_wrong_repetition":   "count/for_each mode changed",
	"delete_because_count_index":        "count index out of range",
	"delete_because_each_key":           "for_each key removed",
}

// why explains a replace or destroy row from attribute paths and the reason
// code only, never from values. Other actions get no explanation.
func why(c plan.ResourceChange) string {
	if c.Action != plan.Replace && c.Action != plan.Destroy {
		return ""
	}
	if c.Action == plan.Replace && len(c.ReplacePaths) > 0 {
		paths := c.ReplacePaths
		more := ""
		if len(paths) > maxWhyPaths {
			more = fmt.Sprintf(" and %d more", len(paths)-maxWhyPaths)
			paths = paths[:maxWhyPaths]
		}
		spans := make([]string, len(paths))
		for i, p := range paths {
			spans[i] = code(p)
		}
		return "forces replacement: " + strings.Join(spans, ", ") + more
	}
	if c.Action == plan.Destroy && c.Deposed {
		return "deposed object"
	}
	if c.Reason == "" {
		return ""
	}
	if t, ok := reasonText[c.Reason]; ok {
		return t
	}
	return code(c.Reason)
}

func writeTable(b *strings.Builder, changes []plan.ResourceChange) {
	// Protected rows are pinned to the top; the order within each group is
	// kept.
	changes = append([]plan.ResourceChange(nil), changes...)
	sort.SliceStable(changes, func(i, j int) bool {
		return changes[i].Protected && !changes[j].Protected
	})
	whys := make([]string, len(changes))
	withWhy := false
	for i, c := range changes {
		whys[i] = why(c)
		withWhy = withWhy || whys[i] != ""
	}
	rows := make([]string, len(changes))
	for i, c := range changes {
		lock := ""
		if c.Protected {
			lock = "🔒 "
		}
		exempt := ""
		if c.RiskIgnored {
			exempt = " (exempt)"
		}
		rows[i] = fmt.Sprintf("| %s%s %s%s | %s | %s |", lock, symbol[c.Action], c.Action, exempt, code(c.Address), code(c.Type))
		if withWhy {
			rows[i] += " " + whys[i] + " |"
		}
		rows[i] += "\n"
	}
	keep, shown := selectRows(changes, rows, MaxBodyBytes-b.Len()-truncationReserve)

	if shown == len(rows) {
		fmt.Fprintf(b, "<details>\n<summary>Details (%d resources)</summary>\n\n", len(rows))
	} else {
		fmt.Fprintf(b, "<details>\n<summary>Details (showing %d of %d resources)</summary>\n\n", shown, len(rows))
	}
	if withWhy {
		b.WriteString("| Action | Resource | Type | Why |\n|--------|----------|------|-----|\n")
	} else {
		b.WriteString("| Action | Resource | Type |\n|--------|----------|------|\n")
	}
	for i, row := range rows {
		if keep[i] {
			b.WriteString(row)
		}
	}
	if shown != len(rows) {
		fmt.Fprintf(b, "\n_%d more resource(s) omitted to stay within GitHub's comment size limit; "+
			"destroys and replacements are listed first._\n", len(rows)-shown)
	}
	b.WriteString("\n</details>\n")
}

// selectRows picks which table rows fit in budget. Rows are chosen in
// risk-first order so that when a huge plan has to be truncated the destroys
// and replacements are the last things dropped — a truncated table must never
// make a plan look milder than it is.
func selectRows(changes []plan.ResourceChange, rows []string, budget int) (keep []bool, shown int) {
	keep = make([]bool, len(rows))
	order := make([]int, len(rows))
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(a, b int) bool {
		return riskRank(changes[order[a]]) < riskRank(changes[order[b]])
	})
	used := 0
	for _, i := range order {
		if used+len(rows[i]) > budget {
			continue
		}
		keep[i] = true
		used += len(rows[i])
		shown++
	}
	return keep, shown
}

// riskRank orders rows for truncation: protected resources are dropped last.
func riskRank(c plan.ResourceChange) int {
	if c.Protected {
		return -1
	}
	switch c.Action {
	case plan.Destroy:
		return 0
	case plan.Replace:
		return 1
	case plan.Forget:
		return 2
	case plan.Unknown:
		return 3
	case plan.Update:
		return 4
	default:
		return 5
	}
}

// code renders s as a Markdown code span that is safe inside a table cell.
// Resource addresses come straight from the plan and can contain arbitrary
// text — `for_each` keys are user-controlled strings, so an address may hold
// pipes, backticks, or newlines. Unescaped, those corrupt the table or let a
// crafted address close the <details> block and hide the rows below it.
func code(s string) string {
	s = strings.NewReplacer("\r", " ", "\n", " ", "|", `\|`).Replace(s)
	// CommonMark: a code span may be fenced with any run of backticks longer
	// than the longest run it contains, which makes the content inert.
	longest, run := 0, 0
	for _, r := range s {
		if r == '`' {
			run++
			if run > longest {
				longest = run
			}
		} else {
			run = 0
		}
	}
	fence := strings.Repeat("`", longest+1)
	pad := ""
	if strings.HasPrefix(s, "`") || strings.HasSuffix(s, "`") || strings.TrimSpace(s) == "" {
		pad = " "
	}
	return fence + pad + s + pad + fence
}
