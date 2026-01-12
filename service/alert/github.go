package alert

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/cenkalti/backoff/v5"
	"github.com/google/go-github/v81/github"
)

const GitHubTimeout = 10 * time.Second

var (
	ErrRateLimitExceeded = errors.New("rate limit exceeded")
	ErrNoIssueNumber     = errors.New("received no issue number")
)

func (a *Alert) createGitHubIssue(ctx context.Context, alert AlertData) (int, error) {
	ctx, _ = context.WithTimeout(ctx, GitHubTimeout)
	rlb := NewRateLimitBackOff(a.logger)

	f := func() (int, error) {
		if a.owner == "" || a.repo == "" {
			return 0, fmt.Errorf("you must set owner and repository via flags")
		}

		body := fmt.Sprintf("%s\nFired at: %s\n\n<details>%s</details>", alert.Description, alert.Time, alert.Details)

		// Build the issue request
		issueRequest := &github.IssueRequest{
			Title: github.Ptr(alert.Title),
			Body:  github.Ptr(body),
		}

		if len(alert.Labels) > 0 {
			issueRequest.Labels = github.Ptr(alert.Labels)
		}

		if len(a.assignees) > 0 {
			issueRequest.Assignees = github.Ptr(a.assignees)
		}

		issue, resp, err := a.client.Issues.Create(ctx, a.owner, a.repo, issueRequest)
		if err != nil {
			if resp != nil && resp.Rate.Used >= resp.Rate.Limit {
				rlb.ReleaseAt(resp.Rate.Reset.Time)
				return 0, fmt.Errorf("error while creating issue: %w", errors.Join(err, ErrRateLimitExceeded))
			}
			return 0, fmt.Errorf("failed to create issue: %w", err)
		}

		if issue == nil || issue.Number == nil {
			return 0, ErrNoIssueNumber
		}

		return *issue.Number, nil
	}

	return backoff.Retry(ctx, f, backoff.WithMaxTries(uint(a.maxRetries)), backoff.WithBackOff(rlb))
}
