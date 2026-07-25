// Package plan parses a `terraform show -json` plan into grouped resource actions.
package plan

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
)

// Action is the effective change kind for a resource.
type Action string

const (
	Create  Action = "create"
	Update  Action = "update"
	Replace Action = "replace"
	Destroy Action = "destroy"
	// Forget is Terraform's `removed` block outcome: the object is dropped from
	// state but left running. Nothing is destroyed, yet the resource stops being
	// managed, so it is reported rather than hidden.
	Forget Action = "forget"
	// Unknown is any action combination this version does not recognise (an
	// empty array, or a verb from a newer Terraform). It is surfaced so the
	// summary can never silently under-report a change it did not understand.
	Unknown Action = "unknown"
	NoOp    Action = "no-op"
	Read    Action = "read"
)

// Order is the display order for actions.
var Order = []Action{Create, Update, Replace, Destroy, Forget, Unknown}

// ResourceChange is one planned change.
type ResourceChange struct {
	Address string
	Type    string
	Action  Action
}

// Summary is the parsed plan grouped by action.
type Summary struct {
	Changes []ResourceChange
	Counts  map[Action]int
}

// HasRisk reports whether the plan destroys, replaces, or forgets anything.
func (s Summary) HasRisk() bool {
	return s.Counts[Destroy] > 0 || s.Counts[Replace] > 0 || s.Counts[Forget] > 0
}

// Total counts the changes that actually modify infrastructure or state
// (excludes no-op and read).
func (s Summary) Total() int {
	return s.Counts[Create] + s.Counts[Update] + s.Counts[Replace] +
		s.Counts[Destroy] + s.Counts[Forget] + s.Counts[Unknown]
}

// Stats is the machine-readable form of a Summary, for `--stats-json` and for
// GitHub Actions step outputs.
type Stats struct {
	Create  int  `json:"create"`
	Update  int  `json:"update"`
	Replace int  `json:"replace"`
	Destroy int  `json:"destroy"`
	Forget  int  `json:"forget"`
	Unknown int  `json:"unknown"`
	NoOp    int  `json:"no_op"`
	Read    int  `json:"read"`
	Total   int  `json:"total"`
	HasRisk bool `json:"has_risk"`
}

// Stats projects the summary into counts a workflow can gate on.
func (s Summary) Stats() Stats {
	return Stats{
		Create:  s.Counts[Create],
		Update:  s.Counts[Update],
		Replace: s.Counts[Replace],
		Destroy: s.Counts[Destroy],
		Forget:  s.Counts[Forget],
		Unknown: s.Counts[Unknown],
		NoOp:    s.Counts[NoOp],
		Read:    s.Counts[Read],
		Total:   s.Total(),
		HasRisk: s.HasRisk(),
	}
}

// planFile mirrors the parts of `terraform show -json` output we need. The
// pointer fields distinguish "key absent" from "key present but empty", which
// is what tells a real plan document apart from some other JSON file.
type planFile struct {
	ResourceChanges *[]resourceChange `json:"resource_changes"`
	PlannedValues   *json.RawMessage  `json:"planned_values"`
}

type resourceChange struct {
	Address string `json:"address"`
	Type    string `json:"type"`
	Change  struct {
		Actions []string `json:"actions"`
	} `json:"change"`
}

// ErrNotAPlan is returned when the input parses as JSON but is not a Terraform
// plan document.
var ErrNotAPlan = errors.New("input is not a Terraform plan document: no " +
	`"resource_changes" or "planned_values" key found. Produce it with ` +
	"`terraform plan -out=plan.bin && terraform show -json plan.bin`, not " +
	"`terraform show -json` (which prints state)")

// classify maps a Terraform actions array to a single effective Action.
// A create paired with a delete (in either order, i.e. both the default and the
// create_before_destroy ordering) is a replacement; so is create+forget.
func classify(actions []string) Action {
	var create, del, forget bool
	for _, a := range actions {
		switch a {
		case "create":
			create = true
		case "delete":
			del = true
		case "forget":
			forget = true
		}
	}
	switch {
	case create && (del || forget):
		return Replace
	case del:
		return Destroy
	case forget:
		return Forget
	case create:
		return Create
	}
	if len(actions) == 1 {
		switch actions[0] {
		case "update":
			return Update
		case "read":
			return Read
		case "no-op":
			return NoOp
		}
	}
	return Unknown
}

// Parse reads plan JSON produced by `terraform show -json plan.bin`.
//
// Input that parses as JSON but carries neither "resource_changes" nor
// "planned_values" is rejected instead of being reported as an empty plan: a
// state file, a truncated download, or an unrelated JSON document must never
// render as "No changes".
func Parse(data []byte) (Summary, error) {
	var pf planFile
	if err := json.Unmarshal(data, &pf); err != nil {
		return Summary{}, fmt.Errorf("parse plan json: %w", err)
	}
	if pf.ResourceChanges == nil && pf.PlannedValues == nil {
		return Summary{}, ErrNotAPlan
	}
	s := Summary{Counts: map[Action]int{}}
	if pf.ResourceChanges != nil {
		for _, rc := range *pf.ResourceChanges {
			act := classify(rc.Change.Actions)
			s.Counts[act]++
			if act == NoOp || act == Read {
				continue
			}
			s.Changes = append(s.Changes, ResourceChange{Address: rc.Address, Type: rc.Type, Action: act})
		}
	}
	sort.SliceStable(s.Changes, func(i, j int) bool {
		if s.Changes[i].Action != s.Changes[j].Action {
			return actionRank(s.Changes[i].Action) < actionRank(s.Changes[j].Action)
		}
		return s.Changes[i].Address < s.Changes[j].Address
	})
	return s, nil
}

func actionRank(a Action) int {
	for i, o := range Order {
		if o == a {
			return i
		}
	}
	return len(Order)
}
