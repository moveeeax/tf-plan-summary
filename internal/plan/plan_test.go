package plan

import (
	"errors"
	"fmt"
	"os"
	"strings"
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
		{[]string{"forget"}, Forget},
		{[]string{"create", "forget"}, Replace},
		// Never silently swallow something we do not recognise.
		{[]string{}, Unknown},
		{nil, Unknown},
		{[]string{"teleport"}, Unknown},
		{[]string{"update", "read"}, Unknown},
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

// A plan full of nested modules, count/for_each indices, both replacement
// orderings, a deposed object and a `removed` block must be counted in full —
// under-reporting a destroy is the worst thing this tool can do.
func TestParseNestedModulesAndIndices(t *testing.T) {
	data, err := os.ReadFile("../../testdata/plan_modules.json")
	if err != nil {
		t.Fatal(err)
	}
	s, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	want := map[Action]int{
		Create: 1, Update: 1, Replace: 2, Destroy: 2, Forget: 1, Unknown: 0, NoOp: 1, Read: 1,
	}
	for act, n := range want {
		if s.Counts[act] != n {
			t.Errorf("%s = %d, want %d", act, s.Counts[act], n)
		}
	}
	if s.Total() != 7 {
		t.Errorf("Total = %d, want 7", s.Total())
	}
	if !s.HasRisk() {
		t.Error("HasRisk should be true")
	}
	// Addresses are reported verbatim, including module nesting and indices.
	got := map[string]Action{}
	for _, c := range s.Changes {
		got[c.Address+"/"+string(c.Action)] = c.Action
	}
	for _, key := range []string{
		"module.network.module.subnets.aws_subnet.this[0]/destroy",
		"module.network.module.subnets.aws_subnet.this[1]/replace",
		"module.db.aws_db_instance.primary/replace",
		`module.storage.aws_s3_bucket.this["eu-west-1|primary"]/update`,
		"module.legacy.aws_instance.pet/forget",
		"module.app.aws_instance.worker[0]/destroy",
		"module.app.aws_instance.worker[0]/create",
	} {
		if _, ok := got[key]; !ok {
			t.Errorf("missing change %q in %v", key, s.Changes)
		}
	}
}

// The parser must not carry plan values around at all: everything it keeps is
// address, type and action, so no secret can reach a PR comment.
func TestParseDropsSensitiveValues(t *testing.T) {
	data, err := os.ReadFile("../../testdata/plan_modules.json")
	if err != nil {
		t.Fatal(err)
	}
	s, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	blob := fmt.Sprintf("%#v", s)
	for _, secret := range []string{"hunter2", "sensitive_values", "master_user_secret", "password"} {
		if strings.Contains(blob, secret) {
			t.Errorf("summary leaks %q:\n%s", secret, blob)
		}
	}
}

// A JSON document that is not a plan must never be reported as "no changes".
func TestParseRejectsNonPlanDocuments(t *testing.T) {
	cases := map[string]string{
		"empty object": `{}`,
		// `terraform show -json` without a plan file prints state.
		"state document": `{"format_version":"1.0","terraform_version":"1.9.0",
			"values":{"root_module":{"resources":[{"address":"aws_s3_bucket.b","type":"aws_s3_bucket"}]}}}`,
		"unrelated json": `{"hello":"world"}`,
	}
	for name, in := range cases {
		if _, err := Parse([]byte(in)); !errors.Is(err, ErrNotAPlan) {
			t.Errorf("%s: err = %v, want ErrNotAPlan", name, err)
		}
	}
}

// A genuine plan with nothing to do omits resource_changes entirely; that one
// really is "no changes".
func TestParseEmptyPlanIsAccepted(t *testing.T) {
	s, err := Parse([]byte(`{"format_version":"1.2","planned_values":{"root_module":{}}}`))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if s.Total() != 0 || s.HasRisk() {
		t.Errorf("expected empty plan, got total=%d risk=%v", s.Total(), s.HasRisk())
	}
}

func TestStats(t *testing.T) {
	data, err := os.ReadFile("../../testdata/plan_modules.json")
	if err != nil {
		t.Fatal(err)
	}
	s, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	st := s.Stats()
	if st.Destroy != 2 || st.Replace != 2 || st.Forget != 1 || st.Total != 7 || !st.HasRisk {
		t.Errorf("unexpected stats: %+v", st)
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
