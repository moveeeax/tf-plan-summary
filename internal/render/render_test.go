package render

import (
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
