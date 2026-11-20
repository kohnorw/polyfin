#!/usr/bin/env bash
# Merges upstream's main (moodiness/polyfin) into this fork's checked-out
# branch (kohnorw/polyfin), as .github/workflows/sync-upstream.yml does
# every quarter of an hour; it can be run by hand as well.
#
# The fork keeps its own changes apart from upstream's code wherever it can
# (files named *fork*, the database migrations in
# internal/database/migrations_fork), so that most of upstream's changes
# merge without conflicts. Two things are settled here every time:
#   - the fork runs its own workflows only (FORK_WORKFLOWS): upstream's,
#     added or changed, are left out, as they publish upstream's releases,
#     desktop apps and website;
#   - a conflict anywhere else stops the merge, for a person to settle.
#
# It prints, and writes to $GITHUB_OUTPUT when set, upstream=<commit> and
# state=up-to-date, merged (a merge commit made, not pushed) or conflict
# (nothing changed; the conflicting files are listed in conflicts.txt).
set -euo pipefail

UPSTREAM_URL=${UPSTREAM_URL:-https://github.com/moodiness/polyfin.git}
UPSTREAM_BRANCH=${UPSTREAM_BRANCH:-main}
FORK_WORKFLOWS="docker.yml sync-upstream.yml"

out() {
	echo "$1"
	if [ -n "${GITHUB_OUTPUT:-}" ]; then echo "$1" >>"$GITHUB_OUTPUT"; fi
}

if ! git remote get-url upstream >/dev/null 2>&1; then
	git remote add upstream "$UPSTREAM_URL"
fi
git fetch --no-tags --quiet upstream "$UPSTREAM_BRANCH"
target=$(git rev-parse "upstream/$UPSTREAM_BRANCH")
out "upstream=$target"

up_to_date=false
if git merge-base --is-ancestor "$target" HEAD; then
	up_to_date=true
else
	git merge --no-ff --no-commit "upstream/$UPSTREAM_BRANCH" >/dev/null 2>&1 || true
fi

# The fork's workflows only: upstream's are removed, whether they merged
# cleanly or conflict.
keep() {
	local name kept
	name=$(basename "$1")
	for kept in $FORK_WORKFLOWS; do
		[ "$name" = "$kept" ] && return 0
	done
	return 1
}
for file in $( (git ls-files .github/workflows; git diff --name-only --diff-filter=U -- .github/workflows) | sort -u); do
	if ! keep "$file"; then
		git rm -q -f --ignore-unmatch -- "$file" >/dev/null 2>&1 || true
		rm -f -- "$file"
	fi
done

if $up_to_date; then
	# A merge settled by hand (the upstream-sync pull request) may have
	# brought upstream's workflows back: they go.
	if git diff --cached --quiet; then
		out "state=up-to-date"
	else
		git commit --quiet -m "Keep the fork's own workflows only"
		out "state=merged"
	fi
	exit 0
fi

conflicts=$(git diff --name-only --diff-filter=U || true)
if [ -n "$conflicts" ]; then
	echo "$conflicts" >conflicts.txt
	git merge --abort
	out "state=conflict"
	echo "Conflicts with upstream:" >&2
	echo "$conflicts" >&2
	exit 0
fi

git commit --quiet --no-edit -m "Merge upstream moodiness/polyfin@${target:0:8} into the fork"
out "state=merged"
