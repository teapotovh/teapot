---
name: Implement a GitHub issue
description: Implement a feature following the task description provided in a GitHub issue.
metadata:
  opencode/autoinvoke: false
---

# Implement a feature to solve a GitHub issue

You're going to follow the workflow outlined in the following sections to
implement a feature/bug fix as outlined in the provided GitHub issue.

The GitHub issue shall be referenced when starting this skill, if not, stop
immediately and notify the user.

## Worktree

Create a new branch based off the latest `origin/main` (fetch), unless the user
specified the work should be based on another feature branch. Then, create a
git worktree where all the implementation will be carried out.

## Implementation

If strictly necessary, you can ask questions to the user before proceeding. Only
do this if the plan has some clear gaps that must be addressed before continuing.

Read the style guidelines in .agents/STYLE.md (from project root) for how to write code.

### Checks

Ensure all tests pertaining to the touched code pass. Ensure all lint checks
succeed and all the Bazel files are up-to-date.

## Create the PR

Create a PR with the `gh` tool targeting the current worktree. For the PR's
textual formatting, follow these rules:

- The title should follow this pattern: "<component>: <what was done":
  it should not be longer than 80 characters. Good examples:
  a. httpsrv: add graceful shutdown
  b. calendar: implement object update
  c. bazel: add golangci-lint support
- The body should contain no sections, just a few short paragraphs outlining
  what was done. Stick to at most 4 sentences. Most changes should be simple
  and require less. Do not use highly techincal terminology or code references
  unless strictly required.

- All text should follow the simple English rules ASD-STE100.

PRs MUST always be created as drafts. Unless explicitly asked by the user, the
PR should contain one single commit. If changes are made during the iteration
on the PR, that same commit should be amended. The commit name should be the
same as the PR's. The commit description can be more in-depth compared to the PR's.

Always mention 'Closes #<issue>' at the end of the PR.

### Iteration

The user may prompt you for changes to the PR. When prompted, first apply the
change as requested, by amending the commit, and push immediately. Then, re-run
the checks (only those which it makes sense to run based on what you touched).

The objective is to push the code to Git as quickly as possible, so the user
can further steer the conversation. Ensuring checks pass (and thus that the CI
is green) is a secondary concern: it's only relevant when the PR is ready to be merged.
