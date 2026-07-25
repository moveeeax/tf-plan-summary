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

	if s.HasRisk() {
		fmt.Fprintf(&b, "> ⚠️ **Review carefully:** %s.\n\n", strings.Join(riskParts(s), ", "))
	}

	writeTable(&b, s.Changes)
	return b.String()
}

// riskParts lists the non-zero destructive counts for the callout.
func riskParts(s plan.Summary) []string {
	var out []string
	for _, a := range []plan.Action{plan.Replace, plan.Destroy, plan.Forget} {
		if n := s.Counts[a]; n > 0 {
			out = append(out, fmt.Sprintf("%d to %s", n, a))
		}
	}
	return out
}

func writeTable(b *strings.Builder, changes []plan.ResourceChange) {
	rows := make([]string, len(changes))
	for i, c := range changes {
		rows[i] = fmt.Sprintf("| %s %s | %s | %s |\n", symbol[c.Action], c.Action, code(c.Address), code(c.Type))
	}
	keep, shown := selectRows(changes, rows, MaxBodyBytes-b.Len()-truncationReserve)

	if shown == len(rows) {
		fmt.Fprintf(b, "<details>\n<summary>Details (%d resources)</summary>\n\n", len(rows))
	} else {
		fmt.Fprintf(b, "<details>\n<summary>Details (showing %d of %d resources)</summary>\n\n", shown, len(rows))
	}
	b.WriteString("| Action | Resource | Type |\n|--------|----------|------|\n")
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
		return riskRank(changes[order[a]].Action) < riskRank(changes[order[b]].Action)
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

func riskRank(a plan.Action) int {
	switch a {
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
