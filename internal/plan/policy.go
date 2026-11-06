package plan

import (
	"errors"
	"strings"
)

// Policy marks some resources as protected and lets others out of the risk
// gate. Patterns are globs: `*` matches any run of characters (dots and
// brackets included) and `?` matches one character; everything else is
// literal, so `aws_instance.web[0]` needs no escaping.
//
// A pattern applies to a change when it matches the full address
// (`module.db.aws_db_instance.main`), the resource type (`aws_db_instance`),
// or the address inside its module (`aws_db_instance.main`).
type Policy struct {
	Protect    []string
	IgnoreRisk []string
}

// Validate rejects empty patterns, which would otherwise match nothing and
// silently leave a protected resource unprotected.
func (p Policy) Validate() error {
	for _, g := range append(append([]string{}, p.Protect...), p.IgnoreRisk...) {
		if strings.TrimSpace(g) == "" {
			return errors.New("empty --protect/--ignore-risk pattern")
		}
	}
	return nil
}

// Apply marks the summary's changes according to the policy. Protection wins
// over --ignore-risk: a resource matched by both stays protected and risky.
func (s *Summary) Apply(p Policy) {
	for i := range s.Changes {
		c := &s.Changes[i]
		c.Protected = matchAny(p.Protect, *c)
		c.RiskIgnored = !c.Protected && c.Action.risky() && matchAny(p.IgnoreRisk, *c)
	}
}

func (a Action) risky() bool {
	return a == Destroy || a == Replace || a == Forget
}

func matchAny(globs []string, c ResourceChange) bool {
	local := c.Address
	if c.ModuleAddress != "" {
		local = strings.TrimPrefix(c.Address, c.ModuleAddress+".")
	}
	for _, g := range globs {
		if Glob(g, c.Address) || Glob(g, c.Type) || Glob(g, local) {
			return true
		}
	}
	return false
}

// Glob reports whether s matches pattern, where `*` matches any sequence of
// characters and `?` exactly one.
func Glob(pattern, s string) bool {
	p, t := []rune(pattern), []rune(s)
	// Classic iterative wildcard match with backtracking to the last star.
	pi, ti, star, mark := 0, 0, -1, 0
	for ti < len(t) {
		switch {
		case pi < len(p) && (p[pi] == '?' || p[pi] == t[ti]) && p[pi] != '*':
			pi++
			ti++
		case pi < len(p) && p[pi] == '*':
			star, mark = pi, ti
			pi++
		case star >= 0:
			pi = star + 1
			mark++
			ti = mark
		default:
			return false
		}
	}
	for pi < len(p) && p[pi] == '*' {
		pi++
	}
	return pi == len(p)
}
