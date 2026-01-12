package alert

import (
	flag "github.com/spf13/pflag"
)

func AlertFlagSet() (*flag.FlagSet, func() AlertConfig) {
	fs := flag.NewFlagSet("alert", flag.ExitOnError)

	token := fs.String(
		"alert-github-token",
		"",
		"the token to access the GitHub API. Needs permission to create issues",
	)
	owner := fs.String(
		"alert-github-owner",
		"teapotovh",
		"the owner of the github repository where issues will be opened",
	)
	repo := fs.String("alert-github-repo", "dev", "the repository name where to open issues")
	assignees := fs.StringArray(
		"alert-github-assignees",
		[]string{"lucat1", "samuelemusiani"},
		"the GitHub users to which issues should be assigned",
	)

	maxRetries := fs.Uint64(
		"alert-github-max-retries",
		4,
		"maximum number of retries to open a GitHub issue",
	)

	return fs, func() AlertConfig {
		return AlertConfig{
			Token:     *token,
			Owner:     *owner,
			Repo:      *repo,
			Assignees: *assignees,

			MaxRetries: *maxRetries,
		}
	}
}
