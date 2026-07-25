// Package render turns a plan Summary into a Markdown PR comment.
package render

import (
	"fmt"
	"strings"

	"github.com/moveeeax/tf-plan-summary/internal/plan"
)

// DefaultMarker is the hidden HTML comment used to find the sticky comment on
// re-runs. Keep it stable across versions.
const DefaultMarker = "<!-- tf-plan-summary -->"

var symbol = map[plan.Action]string{
	plan.Create:  "+",
	plan.Update:  "~",
	plan.Replace: "±",
	plan.Destroy: "-",
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
	fmt.Fprintf(&b, "%s.\n\n", strings.Join(parts, ", "))

	if s.HasRisk() {
		fmt.Fprintf(&b, "> ⚠️ **Review carefully:** %d to replace, %d to destroy.\n\n",
			s.Counts[plan.Replace], s.Counts[plan.Destroy])
	}

	fmt.Fprintf(&b, "<details>\n<summary>Details (%d resources)</summary>\n\n", len(s.Changes))
	b.WriteString("| Action | Resource | Type |\n|--------|----------|------|\n")
	for _, c := range s.Changes {
		fmt.Fprintf(&b, "| %s %s | `%s` | `%s` |\n", symbol[c.Action], c.Action, c.Address, c.Type)
	}
	b.WriteString("\n</details>\n")
	return b.String()
}
