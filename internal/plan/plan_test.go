package plan

import (
	"os"
	"testing"
)

func TestClassify(t *testing.T) {
	cases := []struct {
		in   []string
		want Action
	}{
		{[]string{"create"}, Create},
		{[]string{"update"}, Update},
		{[]string{"delete"}, Destroy},
		{[]string{"create", "delete"}, Replace},
		{[]string{"delete", "create"}, Replace},
		{[]string{"no-op"}, NoOp},
		{[]string{"read"}, Read},
	}
	for _, c := range cases {
		if got := classify(c.in); got != c.want {
			t.Errorf("classify(%v) = %s, want %s", c.in, got, c.want)
		}
	}
}

func TestParseCounts(t *testing.T) {
	data, err := os.ReadFile("../../testdata/plan.json")
	if err != nil {
		t.Fatal(err)
	}
	s, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if s.Counts[Create] != 2 {
		t.Errorf("create = %d, want 2", s.Counts[Create])
	}
	if s.Counts[Update] != 1 {
		t.Errorf("update = %d, want 1", s.Counts[Update])
	}
	if s.Counts[Replace] != 1 {
		t.Errorf("replace = %d, want 1", s.Counts[Replace])
	}
	if s.Counts[Destroy] != 1 {
		t.Errorf("destroy = %d, want 1", s.Counts[Destroy])
	}
	if s.Counts[NoOp] != 1 || s.Counts[Read] != 1 {
		t.Errorf("noop/read = %d/%d, want 1/1", s.Counts[NoOp], s.Counts[Read])
	}
	if s.Total() != 5 {
		t.Errorf("Total = %d, want 5", s.Total())
	}
	if !s.HasRisk() {
		t.Error("HasRisk should be true (destroy + replace present)")
	}
	// no-op and read must not appear in Changes.
	if len(s.Changes) != 5 {
		t.Fatalf("Changes = %d, want 5", len(s.Changes))
	}
	// Ordering: creates first, destroy last.
	if s.Changes[0].Action != Create {
		t.Errorf("first change = %s, want create", s.Changes[0].Action)
	}
	if s.Changes[len(s.Changes)-1].Action != Destroy {
		t.Errorf("last change = %s, want destroy", s.Changes[len(s.Changes)-1].Action)
	}
}

func TestParseNoChanges(t *testing.T) {
	s, err := Parse([]byte(`{"resource_changes":[{"address":"a.b","type":"a","change":{"actions":["no-op"]}}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if s.Total() != 0 || s.HasRisk() {
		t.Errorf("expected empty plan, got total=%d risk=%v", s.Total(), s.HasRisk())
	}
}

func TestParseInvalid(t *testing.T) {
	if _, err := Parse([]byte("not json")); err == nil {
		t.Fatal("expected error on invalid JSON")
	}
}
