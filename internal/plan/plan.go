// Package plan parses a `terraform show -json` plan into grouped resource actions.
package plan

import (
	"encoding/json"
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
	NoOp    Action = "no-op"
	Read    Action = "read"
)

// Order is the display order for actions.
var Order = []Action{Create, Update, Replace, Destroy}

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

// HasRisk reports whether the plan destroys or replaces anything.
func (s Summary) HasRisk() bool { return s.Counts[Destroy] > 0 || s.Counts[Replace] > 0 }

// Total counts the changes that actually modify infrastructure (excludes
// no-op and read).
func (s Summary) Total() int {
	return s.Counts[Create] + s.Counts[Update] + s.Counts[Replace] + s.Counts[Destroy]
}

type planFile struct {
	ResourceChanges []struct {
		Address string `json:"address"`
		Type    string `json:"type"`
		Change  struct {
			Actions []string `json:"actions"`
		} `json:"change"`
	} `json:"resource_changes"`
}

// classify maps a Terraform actions array to a single effective Action.
// A create+delete (in either order) is a replacement.
func classify(actions []string) Action {
	create, del := false, false
	for _, a := range actions {
		switch a {
		case "create":
			create = true
		case "delete":
			del = true
		}
	}
	switch {
	case create && del:
		return Replace
	case create:
		return Create
	case del:
		return Destroy
	}
	if len(actions) == 1 {
		switch actions[0] {
		case "update":
			return Update
		case "read":
			return Read
		}
	}
	return NoOp
}

// Parse reads plan JSON produced by `terraform show -json plan.bin`.
func Parse(data []byte) (Summary, error) {
	var pf planFile
	if err := json.Unmarshal(data, &pf); err != nil {
		return Summary{}, fmt.Errorf("parse plan json: %w", err)
	}
	s := Summary{Counts: map[Action]int{}}
	for _, rc := range pf.ResourceChanges {
		act := classify(rc.Change.Actions)
		s.Counts[act]++
		if act == NoOp || act == Read {
			continue
		}
		s.Changes = append(s.Changes, ResourceChange{Address: rc.Address, Type: rc.Type, Action: act})
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
