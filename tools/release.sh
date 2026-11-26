#!/bin/sh
# Publishes a release from this machine: make release, or make release VERSION=1.2.3 (1.2.3-rc.1).
#
# Without VERSION the next one comes from the commits since the last tag, as semantic-release
# reads them (AdminForth): a breaking change ("feat!:", "BREAKING CHANGE") the major, a "feat:"
# the minor, a "fix:" or "perf:" the patch; with none of them there is nothing to release.
# Checks that main is clean, pushed and green on CI, that the image keeps its size, then builds
# devforth/exceptic for amd64 and arm64 and pushes :1.2.3, :1.2 and :latest (a pre-release only
# its own tag) with the docker login of this machine, tags the commit v1.2.3 and makes its GitHub
# release (the commits since the previous tag).
# With DOCKERHUB_USERNAME and DOCKERHUB_TOKEN (a token with Read, Write & Delete) in the
# environment or .env.local, a release also replaces the repository's description on Docker Hub
# with docs/dockerhub.md.
set -eu

image=devforth/exceptic
version=${1:-}
cd "$(dirname "$0")/.."
fail() { echo "release: $*" >&2 && exit 1; }

[ "$(git branch --show-current)" = main ] || fail "release from main"
[ -z "$(git status --porcelain)" ] || fail "commit or stash the changes first"
git fetch --quiet --tags origin
[ "$(git rev-parse HEAD)" = "$(git rev-parse origin/main)" ] || fail "main is not the pushed origin/main"

# The last release (pre-releases are not a base).
last=$(git tag --list 'v[0-9]*.[0-9]*.[0-9]*' --merged HEAD | grep -v '[-]' | sort -V | tail -1)

if [ -z "$version" ]; then
	[ -n "$last" ] || fail "no release yet: give the first one, make release VERSION=0.1.0"

	log=$(git log --format='%s%n%b' "$last..HEAD")
	set -- $(echo "${last#v}" | tr . ' ')
	major=$1 minor=$2 patch=$3

	if echo "$log" | grep -Eq '^[a-z]+(\([^)]*\))?!:|^BREAKING[ -]CHANGE'; then
		version=$((major + 1)).0.0
	elif echo "$log" | grep -Eq '^feat(\([^)]*\))?:'; then
		version=$major.$((minor + 1)).0
	elif echo "$log" | grep -Eq '^(fix|perf)(\([^)]*\))?:'; then
		version=$major.$minor.$((patch + 1))
	else
		fail "no feat, fix or perf commit since $last: nothing to release"
	fi

	echo "since $last:"
	git log --format='  %s' "$last..HEAD"
	printf 'release %s? [y/N] ' "$version"
	read -r answer </dev/tty
	[ "$answer" = y ] || fail "not released"
fi

if ! echo "$version" | grep -Eq '^[0-9]+\.[0-9]+\.[0-9]+(-[0-9a-z.]+)?$'; then
	echo "VERSION must be X.Y.Z or X.Y.Z-pre (got $version)" >&2
	exit 2
fi

tag=v$version
! git rev-parse --quiet --verify "refs/tags/$tag" >/dev/null || fail "$tag exists"

# The checks of this very commit already ran on CI (tests, lint, vulnerabilities, both images).
commit=$(git rev-parse HEAD)
ci=$(gh run list --workflow CI --commit "$commit" --json conclusion,status --jq '.[0] | "\(.status) \(.conclusion)"' 2>/dev/null || true)
[ "$ci" = "completed success" ] || fail "CI of $(git rev-parse --short HEAD) is '${ci:-not found}': wait for it to pass"

make docker image-size VERSION="$version"

tags="--tag $image:$version"

case $version in
*-*) ;; # a pre-release: no :X.Y, no :latest
*) tags="$tags --tag $image:${version%.*} --tag $image:latest" ;;
esac

# No QEMU needed: Go cross-compiles on the build platform and the final stage has no RUN steps.
docker buildx inspect exceptic-release >/dev/null 2>&1 || docker buildx create --name exceptic-release >/dev/null
# shellcheck disable=SC2086 # tags are several words on purpose
docker buildx build --builder exceptic-release --platform linux/amd64,linux/arm64 \
	--build-arg VERSION="$version" $tags --push .

git tag -a "$tag" -m "$tag"
git push --quiet origin "$tag"
echo "published $image:$version and tagged $tag"

# The GitHub release: the commits since the previous tag, without the task reference.
previous=$(git describe --tags --abbrev=0 "$tag^" 2>/dev/null || true)
notes=$(mktemp)

if [ -n "$previous" ]; then
	git log --format='- %s' "$previous..$tag" | sed 's/ DevFo-OSS\/17$//' >"$notes"
else
	echo "The first release. Run it with Docker: https://github.com/devforth/exceptic/blob/main/docs/deployment.md" >"$notes"
fi

printf '\nImage: `docker pull %s:%s`\n' "$image" "$version" >>"$notes"
prerelease=""

case $version in *-*) prerelease="--prerelease" ;; esac

# shellcheck disable=SC2086 # empty or one flag
gh release create "$tag" --title "$tag" --notes-file "$notes" $prerelease
rm -f "$notes"

case $version in *-*) exit 0 ;; esac

[ -f .env.local ] && set -a && . ./.env.local && set +a

if [ -z "${DOCKERHUB_USERNAME:-}" ] || [ -z "${DOCKERHUB_TOKEN:-}" ]; then
	echo "Docker Hub description not updated: set DOCKERHUB_USERNAME and DOCKERHUB_TOKEN (.env.local) for it"
	exit 0
fi

# The description is docs/dockerhub.md without the markers of its copied blocks (TestDocsCopies).
python3 - "$image" <<'EOF'
import json, os, re, sys, urllib.request

def call(url, body, token=None, method="POST"):
    request = urllib.request.Request(url, json.dumps(body).encode(), method=method, headers={"Content-Type": "application/json"})
    if token:
        request.add_header("Authorization", "Bearer " + token)
    with urllib.request.urlopen(request, timeout=30) as response:
        return json.load(response)

login = call("https://hub.docker.com/v2/users/login", {"username": os.environ["DOCKERHUB_USERNAME"], "password": os.environ["DOCKERHUB_TOKEN"]})
readme = re.sub(r"(?m)^[ \t]*<!-- .* -->\n", "", open("docs/dockerhub.md").read())
call(f"https://hub.docker.com/v2/repositories/{sys.argv[1]}/", {
    "description": "Self-hosted error and log monitoring for the official Sentry SDKs",
    "full_description": readme,
}, login["token"], "PATCH")
print("Docker Hub description updated")
EOF
