// Package comment upserts a single sticky PR comment identified by a marker.
package comment

import (
	"context"
	"fmt"
	"strings"
)

// Comment is a minimal issue/PR comment.
type Comment struct {
	ID   int64
	Body string
}

// Commenter is the seam over the GitHub issue-comments API, so the sticky logic
// is testable without network access.
type Commenter interface {
	List(ctx context.Context) ([]Comment, error)
	Create(ctx context.Context, body string) (Comment, error)
	Update(ctx context.Context, id int64, body string) (Comment, error)
}

// Result reports what Upsert did.
type Result struct {
	Action string // "created" or "updated"
	ID     int64
}

// Upsert posts body as a new comment, or edits the existing comment whose body
// contains marker. This keeps a single sticky comment that refreshes in place
// on every re-run instead of piling up.
func Upsert(ctx context.Context, c Commenter, marker, body string) (Result, error) {
	if !strings.Contains(body, marker) {
		return Result{}, fmt.Errorf("body does not contain marker %q", marker)
	}
	existing, err := c.List(ctx)
	if err != nil {
		return Result{}, fmt.Errorf("list comments: %w", err)
	}
	for _, cm := range existing {
		if strings.Contains(cm.Body, marker) {
			updated, err := c.Update(ctx, cm.ID, body)
			if err != nil {
				return Result{}, fmt.Errorf("update comment %d: %w", cm.ID, err)
			}
			return Result{Action: "updated", ID: updated.ID}, nil
		}
	}
	created, err := c.Create(ctx, body)
	if err != nil {
		return Result{}, fmt.Errorf("create comment: %w", err)
	}
	return Result{Action: "created", ID: created.ID}, nil
}
