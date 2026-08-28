# Backports

## Overview

Merged pull requests can be backported to release branches using
[`korthout/backport-action`](https://github.com/korthout/backport-action),
which opens a cherry-pick PR against the target branch. Same action as
`crossplane/crossplane` and `upbound/upjet`.

The workflow lives in `.github/workflows/backport.yaml`.

## How to backport a PR

Two ways, and both work before or after merge:

- **Label:** add a `backport <branch>` label (e.g. `backport release/v1_10_x`).
  Before merge it runs on merge; after merge it runs right away.
- **Comment:** comment `/backport <branch>` on the PR. After merge it runs
  right away; on an open PR it adds the label so the backport runs on merge.

The backport PR links back to the original. For multiple branches, add one
label (or `/backport` comment) per branch.

The `/backport` comment works only for repo members and collaborators.

## Feedback

The action comments the result on the original PR - a link to the backport PR
on success, cherry-pick instructions on conflict, or the error on failure.

- **Conflict:** the backport PR still opens, with conflict markers. Resolve it
  there.
- **Unknown branch:** if the target branch does not exist, that target fails
  with a comment naming the branch, and the workflow run is marked failed.
  Other valid targets in the same request still proceed.
