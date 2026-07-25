// Package cmd wires the tf-plan-summary CLI.
package cmd

import (
	"context"
	"encoding/json"
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
		planPath  string
		output    string
		statsPath string
		ghOutput  string
		marker    string
		token     string
		repo      string
		prNumber  int
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
			summaryFile := ""
			if output != "" && output != "-" {
				if err := os.WriteFile(output, []byte(body), 0o644); err != nil {
					return fmt.Errorf("write output: %w", err)
				}
				summaryFile = output
			} else {
				fmt.Fprint(cmd.OutOrStdout(), body)
			}

			// Machine-readable counts, so a workflow or script can gate on the
			// plan instead of grepping the Markdown.
			stats := summary.Stats()
			if statsPath != "" {
				if err := writeStatsJSON(statsPath, stats); err != nil {
					return err
				}
			}
			if ghOutput != "" {
				if err := writeGitHubOutput(ghOutput, stats, summaryFile); err != nil {
					return err
				}
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
	root.Flags().StringVar(&statsPath, "stats-json", "", "write machine-readable counts to this JSON file")
	root.Flags().StringVar(&ghOutput, "github-output", os.Getenv("GITHUB_OUTPUT"),
		"append key=value step outputs to this file (defaults to $GITHUB_OUTPUT)")
	root.Flags().StringVar(&marker, "marker", render.DefaultMarker, "hidden marker identifying the sticky comment")
	root.Flags().StringVar(&token, "github-token", os.Getenv("GITHUB_TOKEN"), "GitHub token for posting the comment")
	root.Flags().StringVar(&repo, "repo", os.Getenv("GITHUB_REPOSITORY"), "owner/name of the repository")
	root.Flags().IntVar(&prNumber, "pr", envInt("PR_NUMBER"), "pull request number to comment on")
	_ = root.MarkFlagRequired("plan-json")
	return root
}

// writeStatsJSON writes the counts as a small JSON object.
func writeStatsJSON(path string, st plan.Stats) error {
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return fmt.Errorf("encode stats: %w", err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		return fmt.Errorf("write stats json: %w", err)
	}
	return nil
}

// writeGitHubOutput appends the counts in GitHub Actions step-output format, so
// a workflow can gate on them, e.g.
// `if: steps.summary.outputs.has-risk == 'true'`.
func writeGitHubOutput(path string, st plan.Stats, summaryFile string) error {
	var b strings.Builder
	fmt.Fprintf(&b, "create=%d\n", st.Create)
	fmt.Fprintf(&b, "update=%d\n", st.Update)
	fmt.Fprintf(&b, "replace=%d\n", st.Replace)
	fmt.Fprintf(&b, "destroy=%d\n", st.Destroy)
	fmt.Fprintf(&b, "forget=%d\n", st.Forget)
	fmt.Fprintf(&b, "unknown=%d\n", st.Unknown)
	fmt.Fprintf(&b, "total=%d\n", st.Total)
	fmt.Fprintf(&b, "has-risk=%t\n", st.HasRisk)
	if summaryFile != "" && !strings.ContainsAny(summaryFile, "\r\n") {
		fmt.Fprintf(&b, "summary-file=%s\n", summaryFile)
	}

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open github output: %w", err)
	}
	if _, err := f.WriteString(b.String()); err != nil {
		f.Close()
		return fmt.Errorf("write github output: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("write github output: %w", err)
	}
	return nil
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
