#!/usr/bin/env bash
# Moves Polyfin to a newer Jellyfin release: the server version it reports,
# the jellyfin-web it serves (the official image's files, pinned by digest,
# and the source tarball releases attach, pinned by checksum), and the
# image the fixture scripts record from.
#
# It changes pins only. Whether Polyfin still answers as the new Jellyfin
# does is for the tests to say, and for scripts/jellyfin-fixtures.sh, run
# against the new image, to show in the diff of the recorded fixtures.
#
# Usage: scripts/update-jellyfin.sh [VERSION]
#   VERSION is a Jellyfin release such as 12.3.0; without it, the latest
#   stable release on GitHub. Prints the new version on stdout when it
#   changed something, nothing when Polyfin is already on it or newer.
#
# Requirements: curl, jq, sha256sum, docker (buildx, for the digest).
set -euo pipefail

cd "$(dirname "$0")/.."

for tool in curl jq sha256sum docker; do
	command -v "$tool" >/dev/null || { echo "$tool is required" >&2; exit 1; }
done

api() {
	local auth=()
	[ -n "${GITHUB_TOKEN:-}" ] && auth=(-H "Authorization: Bearer $GITHUB_TOKEN")
	curl -fsSL "${auth[@]}" -H 'Accept: application/vnd.github+json' "https://api.github.com/$1"
}

new_full="${1:-}"
if [ -z "$new_full" ]; then
	new_full="$(api repos/jellyfin/jellyfin/releases/latest | jq -r .tag_name)"
fi
new_full="${new_full#v}"
# Jellyfin tags some releases with two numbers (v12.4): those are x.y.0.
if [[ "$new_full" =~ ^[0-9]+\.[0-9]+$ ]]; then
	new_full="$new_full.0"
fi
if ! [[ "$new_full" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
	# A prerelease or an odd tag: nothing to follow, which is no failure.
	echo "not a stable Jellyfin release: '$new_full', nothing to do" >&2
	exit 0
fi
new_minor="${new_full%.*}"

old_full="$(sed -nE 's/^[[:space:]]*Version = "([0-9.]+)"$/\1/p' internal/jellyfin/jellyfin.go)"
old_minor="$(sed -nE 's#^FROM .*jellyfin/jellyfin:([0-9.]+)@sha256:.* AS jellyfin-web$#\1#p' Dockerfile)"
old_digest="$(sed -nE 's#^FROM .*jellyfin/jellyfin:[0-9.]+@(sha256:[0-9a-f]+) AS jellyfin-web$#\1#p' Dockerfile)"
old_web_sum="$(grep -A1 -E '^ADD --checksum=sha256:[0-9a-f]+ \\$' Dockerfile | grep -B1 'jellyfin-web/archive' | sed -nE 's/^ADD --checksum=(sha256:[0-9a-f]+).*/\1/p')"
old_web_url="$(grep -oE 'https://github.com/jellyfin/jellyfin-web/archive/refs/tags/[^ ]+\.tar\.gz' Dockerfile)"
for value in old_full old_minor old_digest old_web_sum old_web_url; do
	[ -n "${!value}" ] || { echo "could not read $value from the repository" >&2; exit 1; }
done

newest="$(printf '%s\n%s\n' "$old_full" "$new_full" | sort -V | tail -1)"
if [ "$new_full" = "$old_full" ] || [ "$newest" = "$old_full" ]; then
	echo "Polyfin is on Jellyfin $old_full, as new as $new_full" >&2
	exit 0
fi
echo "Jellyfin $old_full -> $new_full" >&2

# The image's tag: Polyfin pins the minor version's tag, as before, unless
# only the full version's exists.
tag="$new_minor"
if ! digest="$(docker buildx imagetools inspect "jellyfin/jellyfin:$tag" --format '{{json .Manifest}}' 2>/dev/null | jq -r .digest)" || [ -z "$digest" ] || [ "$digest" = null ]; then
	tag="$new_full"
	digest="$(docker buildx imagetools inspect "jellyfin/jellyfin:$tag" --format '{{json .Manifest}}' | jq -r .digest)"
fi
[[ "$digest" =~ ^sha256:[0-9a-f]{64}$ ]] || { echo "no image digest for jellyfin/jellyfin:$tag" >&2; exit 1; }

# jellyfin-web's source, tagged like the server, with or without the patch.
web_tag=""
for candidate in "v$new_full" "v$new_minor"; do
	if curl -fsSLI -o /dev/null "https://github.com/jellyfin/jellyfin-web/archive/refs/tags/$candidate.tar.gz"; then
		web_tag="$candidate"
		break
	fi
done
[ -n "$web_tag" ] || { echo "no jellyfin-web source tagged v$new_full or v$new_minor" >&2; exit 1; }
web_url="https://github.com/jellyfin/jellyfin-web/archive/refs/tags/$web_tag.tar.gz"
web_sum="sha256:$(curl -fsSL "$web_url" | sha256sum | cut -d' ' -f1)"
web_version="${web_tag#v}"
old_web_version="$(basename "$old_web_url" .tar.gz)"
old_web_version="${old_web_version#v}"

esc() { printf '%s' "$1" | sed 's/[.[\*^$/]/\\&/g'; }

# The image and source pins, and the versions named beside them.
sed -i \
	-e "s#jellyfin/jellyfin:$(esc "$old_minor")@$old_digest#jellyfin/jellyfin:$tag@$digest#" \
	-e "s#$old_web_sum#$web_sum#" \
	-e "s#$(esc "$old_web_url")#$web_url#" \
	-e "s#jellyfin-web-$(esc "$old_web_version")-source#jellyfin-web-$web_version-source#g" \
	-e "s#jellyfin-web $(esc "$old_minor")\b#jellyfin-web $new_minor#g" \
	-e "s#Jellyfin $(esc "$old_minor")\b#Jellyfin $new_minor#g" \
	Dockerfile
sed -i \
	-e "s#jellyfin/jellyfin:$(esc "$old_minor")@$old_digest#jellyfin/jellyfin:$tag@$digest#" \
	scripts/jellyfin-fixtures.sh scripts/jellyfin-fixtures-music.sh
sed -i \
	-e "s#jellyfin-web-$(esc "$old_web_version")-source#jellyfin-web-$web_version-source#g" \
	-e "s#jellyfin/jellyfin:$(esc "$old_minor")\b#jellyfin/jellyfin:$tag#g" \
	-e "s#tree/v$(esc "$old_web_version")\b#tree/$web_tag#g" \
	-e "s#\b$(esc "$old_minor")\b#$new_minor#g" \
	third_party/jellyfin-web/NOTICE

# The version Polyfin reports, and the one its own sign-in to jellyfin-web
# sends.
sed -i -E "s/^([[:space:]]*Version = )\"$(esc "$old_full")\"$/\1\"$new_full\"/" internal/jellyfin/jellyfin.go
sed -i "s/Version=\"$(esc "$old_full")\"/Version=\"$new_full\"/" web/src/webClientSession.ts

# What the documentation says Polyfin targets.
sed -i "s/Jellyfin $(esc "$old_minor") API/Jellyfin $new_minor API/g" README.md docs/jellyfin-compatibility.md

echo "$new_full"
