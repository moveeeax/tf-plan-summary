package comment

import (
	"context"
	"errors"
	"testing"
)

type fakeCommenter struct {
	comments []Comment
	created  []string
	updated  map[int64]string
	listErr  error
	nextID   int64
}

func newFake(existing ...Comment) *fakeCommenter {
	return &fakeCommenter{comments: existing, updated: map[int64]string{}, nextID: 1000}
}

func (f *fakeCommenter) List(context.Context) ([]Comment, error) {
	return f.comments, f.listErr
}

func (f *fakeCommenter) Create(_ context.Context, body string) (Comment, error) {
	f.created = append(f.created, body)
	f.nextID++
	c := Comment{ID: f.nextID, Body: body}
	f.comments = append(f.comments, c)
	return c, nil
}

func (f *fakeCommenter) Update(_ context.Context, id int64, body string) (Comment, error) {
	f.updated[id] = body
	return Comment{ID: id, Body: body}, nil
}

const marker = "<!-- tf-plan-summary -->"

func TestUpsertCreatesWhenAbsent(t *testing.T) {
	f := newFake(Comment{ID: 1, Body: "unrelated comment"})
	res, err := Upsert(context.Background(), f, marker, marker+"\nhello")
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	if res.Action != "created" {
		t.Errorf("action = %s, want created", res.Action)
	}
	if len(f.created) != 1 {
		t.Errorf("expected 1 create, got %d", len(f.created))
	}
}

func TestUpsertUpdatesExisting(t *testing.T) {
	f := newFake(
		Comment{ID: 1, Body: "chatter"},
		Comment{ID: 42, Body: marker + "\nold body"},
	)
	res, err := Upsert(context.Background(), f, marker, marker+"\nnew body")
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	if res.Action != "updated" || res.ID != 42 {
		t.Errorf("got %+v, want updated id 42", res)
	}
	if f.updated[42] != marker+"\nnew body" {
		t.Errorf("comment 42 not updated: %q", f.updated[42])
	}
	if len(f.created) != 0 {
		t.Error("should not create when an existing sticky comment is found")
	}
}

func TestUpsertRejectsBodyWithoutMarker(t *testing.T) {
	f := newFake()
	if _, err := Upsert(context.Background(), f, marker, "no marker here"); err == nil {
		t.Fatal("expected error when body lacks the marker")
	}
}

func TestUpsertListError(t *testing.T) {
	f := newFake()
	f.listErr = errors.New("boom")
	if _, err := Upsert(context.Background(), f, marker, marker+"\nx"); err == nil {
		t.Fatal("expected list error to propagate")
	}
}
