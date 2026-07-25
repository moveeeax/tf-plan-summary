// Package cmd wires the tf-plan-summary CLI.
package cmd

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/moveeeax/tf-plan-summary/internal/comment"
	"github.com/moveeeax/tf-plan-summary/internal/plan"
	"github.com/moveeeax/tf-plan-summary/internal/render"
)

func newRootCmd() *cobra.Command {
	var (
		planPath string
		output   string
		marker   string
		token    string
		repo     string
		prNumber int
	)

	root := &cobra.Command{
		Use:   "tf-plan-summary",
		Short: "Summarize a terraform plan JSON as a sticky PR comment",
		Long: "tf-plan-summary parses `terraform show -json plan.bin`, groups resource\n" +
			"actions (create/update/replace/destroy), and renders a Markdown summary. With\n" +
			"a token, repo, and PR number it upserts a single sticky comment.",
		Example: "  tf-plan-summary --plan-json plan.json\n" +
			"  tf-plan-summary --plan-json plan.json --repo owner/name --pr 42 --github-token $GITHUB_TOKEN",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			data, err := os.ReadFile(planPath)
			if err != nil {
				return fmt.Errorf("read plan json: %w", err)
			}
			summary, err := plan.Parse(data)
			if err != nil {
				return err
			}
			body := render.Markdown(summary, marker)

			// Write/print the rendered Markdown.
			if output != "" && output != "-" {
				if err := os.WriteFile(output, []byte(body), 0o644); err != nil {
					return fmt.Errorf("write output: %w", err)
				}
			} else {
				fmt.Fprint(cmd.OutOrStdout(), body)
			}

			// Post the sticky comment only when fully configured.
			if token == "" || repo == "" || prNumber == 0 {
				return nil
			}
			owner, name, ok := splitRepo(repo)
			if !ok {
				return fmt.Errorf("invalid --repo %q, want owner/name", repo)
			}
			ctx := context.Background()
			c := comment.NewGitHubCommenter(ctx, nil, token, owner, name, prNumber)
			res, err := comment.Upsert(ctx, c, marker, body)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.ErrOrStderr(), "sticky comment %s (id %d)\n", res.Action, res.ID)
			return nil
		},
	}

	root.Flags().StringVar(&planPath, "plan-json", "", "path to `terraform show -json` output (required)")
	root.Flags().StringVarP(&output, "output", "o", "-", "write Markdown to a file (default: stdout)")
	root.Flags().StringVar(&marker, "marker", render.DefaultMarker, "hidden marker identifying the sticky comment")
	root.Flags().StringVar(&token, "github-token", os.Getenv("GITHUB_TOKEN"), "GitHub token for posting the comment")
	root.Flags().StringVar(&repo, "repo", os.Getenv("GITHUB_REPOSITORY"), "owner/name of the repository")
	root.Flags().IntVar(&prNumber, "pr", envInt("PR_NUMBER"), "pull request number to comment on")
	_ = root.MarkFlagRequired("plan-json")
	return root
}

func splitRepo(s string) (owner, name string, ok bool) {
	parts := strings.SplitN(s, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", false
	}
	return parts[0], parts[1], true
}

func envInt(key string) int {
	n, _ := strconv.Atoi(os.Getenv(key))
	return n
}

// Execute runs the root command.
func Execute() error {
	return newRootCmd().Execute()
}
