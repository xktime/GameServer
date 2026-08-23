---
name: prepare-pr
description: Prepare and open a pull request from local Git changes on the current branch. Use when asked to organize local modifications, stage and commit them, push the checked-out branch, or create a PR; do not use to review an existing PR.
---

# Prepare a Pull Request

Turn only the intended local changes on the checked-out branch into a reviewable commit and pull request. Keep the current branch; create, switch, rebase, or rewrite branches only when the user explicitly requests it.

## Preflight

1. Read the applicable repository instructions. Inspect the branch, upstream, remotes, status, staged diff, unstaged diff, and untracked files. Determine the repository's default base branch and whether it provides a pull-request template.
2. When a PR is requested, verify that push access and an authenticated PR-creation mechanism are available before changing Git state.
3. Define the exact inclusion set from the user's request and the diff. Leave unrelated changes untouched and call out anything intentionally excluded.
4. Stop before staging or committing when any of these is true:
   - `HEAD` is detached, a merge or rebase is unresolved, or the current branch is the PR base branch.
   - Intended and unrelated edits cannot be separated safely.
   - The diff contains credentials, secrets, large generated artifacts, or other suspicious files.
   - Completion would require a force push, history rewrite, branch change, or destructive cleanup that the user did not authorize.

## Review and Validate

1. Explain the change's purpose from the actual diff. Do not invent intent that the code and user request do not establish.
2. Follow repository-required impact analysis and pre-commit checks. Run the smallest relevant test, lint, type-check, or build commands for the included paths, preferring check-only commands before auto-fixing commands.
3. Reinspect the diff after any command that modifies files. Keep only modifications that belong to the requested change.
4. If a required check fails, distinguish failures caused by the included changes from pre-existing or environmental failures. Stop before commit and report the evidence unless the user explicitly accepts proceeding with the known failure.

## Commit the Current Branch

1. Derive a concise commit message from the included diff and the repository's recent commit style.
2. Present a mutation checkpoint containing the included paths, excluded paths, validation results, commit message, push target, PR base, and proposed PR title. An explicit request to commit, push, and create a PR authorizes those exact actions; otherwise obtain approval before the first mutation.
3. Stage only the included paths with path-specific Git arguments. Inspect the staged diff and status, then commit it on the current branch. Never amend an existing commit unless explicitly requested.
4. Push `HEAD` to the same-named remote branch, setting its upstream when necessary. Never force push.

## Open the Pull Request

1. Check whether the current head branch already has an open PR. Reuse and report it instead of creating a duplicate.
2. Create the PR against the verified base branch with the repository template when present. Derive the title from the committed change and make the body factual, including a short summary and the exact validation performed.
3. Verify the resulting head branch, base branch, commit, and PR URL.
4. Report the commit SHA, PR URL, validation results, and any local files deliberately left uncommitted. If a later step fails after a commit was created, clearly report the partial state and the safest next action.
