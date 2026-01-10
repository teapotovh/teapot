package alert

import (
	flag "github.com/spf13/pflag"
)

func AlertFlagSet() (*flag.FlagSet, func() AlertConfig) {
	fs := flag.NewFlagSet("alert", flag.ExitOnError)

	token := fs.String("alert-github-token", "", "The token to access the GitHub API. Needs permission to create issues")
	owner := fs.String("alert-github-owner", "teapotovh", "The owner of the github repository where issues will be opened")
	repo := fs.String("alert-github-repo", "dev", "The repository name where to open issues")

	return fs, func() AlertConfig {
		return AlertConfig{
			Token: *token,
			Owner: *owner,
			Repo:  *repo,
		}
	}
}
