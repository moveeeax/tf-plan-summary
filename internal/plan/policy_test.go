package plan

import (
	"os"
	"testing"
)

func TestGlob(t *testing.T) {
	cases := []struct {
		pattern, s string
		want       bool
	}{
		{"aws_db_instance.*", "aws_db_instance.main", true},
		{"aws_db_instance.*", "module.db.aws_db_instance.main", false},
		{"module.data.*", "module.data.aws_s3_bucket.b", true},
		{"module.data.*", "module.data.module.inner.aws_s3_bucket.b", true},
		{"module.data.*", "module.database.aws_s3_bucket.b", false},
		{"aws_instance.web[0]", "aws_instance.web[0]", true},
		{"aws_instance.web[?]", "aws_instance.web[7]", true},
		{"aws_instance.web[?]", "aws_instance.web[10]", false},
		{"*", "", true},
		{"", "", true},
		{"", "a", false},
		{"a*b*c", "aXXbYYc", true},
		{"a*b*c", "aXXbYY", false},
		{`*["eu|west"]`, `aws_s3_bucket.this["eu|west"]`, true},
	}
	for _, c := range cases {
		if got := Glob(c.pattern, c.s); got != c.want {
			t.Errorf("Glob(%q, %q) = %v, want %v", c.pattern, c.s, got, c.want)
		}
	}
}

func applied(t *testing.T, p Policy) Summary {
	t.Helper()
	data, err := os.ReadFile("../../testdata/plan_modules.json")
	if err != nil {
		t.Fatal(err)
	}
	s, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	s.Apply(p)
	return s
}

func TestApplyProtectByTypeAddressAndLocalAddress(t *testing.T) {
	for _, glob := range []string{
		"aws_db_instance",                   // type
		"module.db.aws_db_instance.primary", // full address
		"aws_db_instance.*",                 // address inside its module
	} {
		s := applied(t, Policy{Protect: []string{glob}})
		st := s.Stats()
		if st.ProtectedReplace != 1 || st.ProtectedDestroy != 0 || !st.HasCriticalRisk {
			t.Errorf("--protect %q: stats %+v", glob, st)
		}
	}
}

func TestApplyNoPolicyIsUnchanged(t *testing.T) {
	st := applied(t, Policy{}).Stats()
	if !st.HasRisk || st.HasCriticalRisk || st.ProtectedDestroy != 0 || st.ProtectedReplace != 0 {
		t.Errorf("empty policy changed stats: %+v", st)
	}
}

func TestApplyIgnoreRisk(t *testing.T) {
	// Exempt every risky change: the plan is no longer risky, but its counts
	// stay complete.
	s := applied(t, Policy{IgnoreRisk: []string{"module.*"}})
	if s.HasRisk() {
		t.Error("HasRisk should be false when every risky change is exempt")
	}
	if s.Counts[Destroy] != 2 || s.Counts[Replace] != 2 || s.RiskIgnored() != 5 {
		t.Errorf("counts changed: %v, ignored %d", s.Counts, s.RiskIgnored())
	}
	// Only creates and updates are never marked.
	for _, c := range s.Changes {
		if c.RiskIgnored != c.Action.risky() {
			t.Errorf("%s %s: RiskIgnored = %v", c.Action, c.Address, c.RiskIgnored)
		}
	}

	// Exempting one subnet leaves the rest risky.
	s = applied(t, Policy{IgnoreRisk: []string{"module.network.*"}})
	if !s.HasRisk() || s.RiskCount(Destroy) != 1 || s.RiskCount(Replace) != 1 {
		t.Errorf("partial exemption: destroy %d replace %d", s.RiskCount(Destroy), s.RiskCount(Replace))
	}
}

func TestApplyProtectWinsOverIgnore(t *testing.T) {
	s := applied(t, Policy{Protect: []string{"aws_db_instance"}, IgnoreRisk: []string{"*"}})
	if !s.HasCriticalRisk() || !s.HasRisk() || s.RiskCount(Replace) != 1 {
		t.Errorf("protected change was exempted: critical=%v risk=%v replace=%d",
			s.HasCriticalRisk(), s.HasRisk(), s.RiskCount(Replace))
	}
}

func TestPolicyValidate(t *testing.T) {
	if err := (Policy{Protect: []string{"a.*"}, IgnoreRisk: []string{"b"}}).Validate(); err != nil {
		t.Errorf("valid policy rejected: %v", err)
	}
	for _, p := range []Policy{{Protect: []string{""}}, {IgnoreRisk: []string{"  "}}} {
		if err := p.Validate(); err == nil {
			t.Errorf("empty pattern accepted: %+v", p)
		}
	}
}
