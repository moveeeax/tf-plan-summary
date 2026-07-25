package render

import (
	"fmt"
	"strings"
	"testing"

	"github.com/moveeeax/tf-plan-summary/internal/plan"
)

func sample() plan.Summary {
	return plan.Summary{
		Changes: []plan.ResourceChange{
			{Address: "aws_s3_bucket.logs", Type: "aws_s3_bucket", Action: plan.Create},
			{Address: "aws_instance.web", Type: "aws_instance", Action: plan.Replace},
			{Address: "aws_db_instance.legacy", Type: "aws_db_instance", Action: plan.Destroy},
		},
		Counts: map[plan.Action]int{plan.Create: 1, plan.Replace: 1, plan.Destroy: 1},
	}
}

func TestMarkdownContent(t *testing.T) {
	md := Markdown(sample(), DefaultMarker)
	for _, sub := range []string{
		DefaultMarker,
		"## Terraform Plan Summary",
		"**1** to add",
		"**1** to replace",
		"**1** to destroy",
		"⚠️",
		"<details>",
		"| Action | Resource | Type |",
		"`aws_db_instance.legacy`",
	} {
		if !strings.Contains(md, sub) {
			t.Errorf("markdown missing %q\n---\n%s", sub, md)
		}
	}
}

func TestMarkdownNoChanges(t *testing.T) {
	empty := plan.Summary{Counts: map[plan.Action]int{}}
	md := Markdown(empty, DefaultMarker)
	if !strings.Contains(md, "No changes") {
		t.Errorf("expected no-changes message, got:\n%s", md)
	}
	if strings.Contains(md, "<details>") {
		t.Error("empty plan should not render a details table")
	}
}

// splitCells splits a table row on unescaped pipes.
func splitCells(row string) []string {
	var cells []string
	var cur strings.Builder
	for i := 0; i < len(row); i++ {
		if row[i] == '\\' && i+1 < len(row) {
			cur.WriteByte(row[i])
			i++
			cur.WriteByte(row[i])
			continue
		}
		if row[i] == '|' {
			cells = append(cells, cur.String())
			cur.Reset()
			continue
		}
		cur.WriteByte(row[i])
	}
	return append(cells, cur.String())
}

// for_each keys are arbitrary user strings, so a resource address can contain
// pipes, backticks or newlines. A crafted one must not be able to break the
// table apart or close the <details> block and hide the rows below it.
func TestMarkdownEscapesHostileAddresses(t *testing.T) {
	hostile := "aws_s3_bucket.this[\"x|y`\n</summary></details><!--\"]"
	s := plan.Summary{
		Changes: []plan.ResourceChange{
			{Address: hostile, Type: "aws_s3_bucket", Action: plan.Destroy},
			{Address: "aws_instance.after", Type: "aws_instance", Action: plan.Destroy},
		},
		Counts: map[plan.Action]int{plan.Destroy: 2},
	}
	md := Markdown(s, DefaultMarker)

	var row string
	for _, line := range strings.Split(md, "\n") {
		if strings.Contains(line, "aws_s3_bucket") {
			row = line
		}
	}
	if row == "" {
		t.Fatalf("hostile row missing:\n%s", md)
	}
	// A row must stay a row: leading empty cell, action, resource, type, trailing.
	if cells := splitCells(row); len(cells) != 5 {
		t.Errorf("row split into %d cells, want 5: %q", len(cells), row)
	}
	if !strings.Contains(row, `\|`) {
		t.Errorf("pipe in address not escaped: %q", row)
	}
	// The row that follows must still be rendered as its own row.
	if !strings.Contains(md, "aws_instance.after") {
		t.Errorf("row after the hostile one disappeared:\n%s", md)
	}
	if !strings.HasSuffix(md, "</details>\n") {
		t.Errorf("details block not closed by us:\n%s", md)
	}
}

func TestCodeSpanFencing(t *testing.T) {
	for _, in := range []string{"a`b", "a``b", "`lead", "trail`", "", "  ", "plain"} {
		got := code(in)
		fence := strings.Repeat("`", len(got)-len(strings.TrimLeft(got, "`")))
		if fence == "" {
			t.Fatalf("code(%q) = %q has no fence", in, got)
		}
		inner := strings.TrimSuffix(strings.TrimPrefix(got, fence), fence)
		if strings.Contains(inner, fence) {
			t.Errorf("code(%q) = %q: fence %q appears inside the span", in, got, fence)
		}
		if !strings.Contains(got, in) {
			t.Errorf("code(%q) = %q dropped the content", in, got)
		}
	}
}

// GitHub rejects comments over 65536 characters, so a huge plan is truncated —
// but never in a way that drops the destructive rows.
func TestMarkdownTruncationKeepsDestroys(t *testing.T) {
	s := plan.Summary{Counts: map[plan.Action]int{plan.Create: 4000, plan.Destroy: 3}}
	for i := 0; i < 4000; i++ {
		s.Changes = append(s.Changes, plan.ResourceChange{
			Address: fmt.Sprintf("module.big.aws_s3_bucket_object.file[%d]", i),
			Type:    "aws_s3_bucket_object",
			Action:  plan.Create,
		})
	}
	for i := 0; i < 3; i++ {
		s.Changes = append(s.Changes, plan.ResourceChange{
			Address: fmt.Sprintf("module.prod.aws_db_instance.critical[%d]", i),
			Type:    "aws_db_instance",
			Action:  plan.Destroy,
		})
	}
	md := Markdown(s, DefaultMarker)

	if len(md) > MaxBodyBytes {
		t.Fatalf("rendered %d bytes, want <= %d", len(md), MaxBodyBytes)
	}
	for i := 0; i < 3; i++ {
		want := fmt.Sprintf("module.prod.aws_db_instance.critical[%d]", i)
		if !strings.Contains(md, want) {
			t.Errorf("truncation dropped destroy row %q", want)
		}
	}
	// Counts stay exact even when the table does not.
	if !strings.Contains(md, "**3** to destroy") || !strings.Contains(md, "**4000** to add") {
		t.Errorf("counts line wrong after truncation:\n%s", md[:400])
	}
	if !strings.Contains(md, "omitted") || !strings.Contains(md, "showing") {
		t.Errorf("truncation not disclosed:\n%s", md[:400])
	}
}

func TestMarkdownForgetAndUnknown(t *testing.T) {
	s := plan.Summary{
		Changes: []plan.ResourceChange{
			{Address: "aws_instance.pet", Type: "aws_instance", Action: plan.Forget},
			{Address: "aws_instance.mystery", Type: "aws_instance", Action: plan.Unknown},
		},
		Counts: map[plan.Action]int{plan.Forget: 1, plan.Unknown: 1},
	}
	md := Markdown(s, DefaultMarker)
	for _, sub := range []string{"**1** to forget", "**1** unclassified", "⚠️", "1 to forget",
		"`aws_instance.pet`", "`aws_instance.mystery`"} {
		if !strings.Contains(md, sub) {
			t.Errorf("markdown missing %q\n---\n%s", sub, md)
		}
	}
}

func TestMarkdownRiskCalloutOmitsZeroes(t *testing.T) {
	s := plan.Summary{
		Changes: []plan.ResourceChange{{Address: "a.b", Type: "a", Action: plan.Destroy}},
		Counts:  map[plan.Action]int{plan.Destroy: 1},
	}
	md := Markdown(s, DefaultMarker)
	if !strings.Contains(md, "**Review carefully:** 1 to destroy.") {
		t.Errorf("unexpected callout:\n%s", md)
	}
}

func TestMarkdownNoRiskNoCallout(t *testing.T) {
	s := plan.Summary{
		Changes: []plan.ResourceChange{{Address: "a.b", Type: "a", Action: plan.Create}},
		Counts:  map[plan.Action]int{plan.Create: 1},
	}
	md := Markdown(s, DefaultMarker)
	if strings.Contains(md, "⚠️") {
		t.Errorf("create-only plan should not show a risk callout:\n%s", md)
	}
}
