package comment

import (
	"context"
	"net/http"

	"github.com/google/go-github/v93/github"
)

// GitHubCommenter implements Commenter against a real repository PR using the
// GitHub issue-comments API (PRs are issues for commenting purposes).
type GitHubCommenter struct {
	client *github.Client
	owner  string
	repo   string
	number int
}

// NewGitHubCommenter builds a Commenter for owner/repo#number authenticated with
// token (typically the Actions GITHUB_TOKEN). A nil hc selects a default
// http.Client.
func NewGitHubCommenter(ctx context.Context, hc *http.Client, token, owner, repo string, number int) (*GitHubCommenter, error) {
	opts := []github.ClientOptionsFunc{github.WithAuthToken(token)}
	if hc != nil {
		opts = append(opts, github.WithHTTPClient(hc))
	}
	client, err := github.NewClient(opts...)
	if err != nil {
		return nil, err
	}
	return &GitHubCommenter{client: client, owner: owner, repo: repo, number: number}, nil
}

func (g *GitHubCommenter) List(ctx context.Context) ([]Comment, error) {
	var out []Comment
	opts := &github.IssueListCommentsOptions{ListOptions: github.ListOptions{PerPage: 100}}
	for {
		comments, resp, err := g.client.Issues.ListComments(ctx, g.owner, g.repo, g.number, opts)
		if err != nil {
			return nil, err
		}
		for _, c := range comments {
			out = append(out, Comment{ID: c.GetID(), Body: c.GetBody()})
		}
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}
	return out, nil
}

func (g *GitHubCommenter) Create(ctx context.Context, body string) (Comment, error) {
	c, _, err := g.client.Issues.CreateComment(ctx, g.owner, g.repo, g.number, github.IssueCommentRequest{Body: body})
	if err != nil {
		return Comment{}, err
	}
	return Comment{ID: c.GetID(), Body: c.GetBody()}, nil
}

func (g *GitHubCommenter) Update(ctx context.Context, id int64, body string) (Comment, error) {
	c, _, err := g.client.Issues.UpdateComment(ctx, g.owner, g.repo, id, github.IssueCommentRequest{Body: body})
	if err != nil {
		return Comment{}, err
	}
	return Comment{ID: c.GetID(), Body: c.GetBody()}, nil
}
