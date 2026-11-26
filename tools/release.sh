#!/bin/sh
# Publishes a release from this machine: make release VERSION=1.2.3 (or 1.2.3-rc.1).
#
# Checks that main is clean, pushed and green on CI, that the image keeps its size, then builds
# devforth/exceptic for amd64 and arm64 and pushes :1.2.3, :1.2 and :latest (a pre-release only
# its own tag) with the docker login of this machine, and tags the commit v1.2.3 on GitHub.
# With DOCKERHUB_USERNAME and DOCKERHUB_TOKEN (a token with Read, Write & Delete) in the
# environment or .env.local, a release also replaces the repository's description on Docker Hub
# with docs/dockerhub.md.
set -eu

image=devforth/exceptic
version=${1:-}

case $version in
'' | *[!0-9a-z.-]*) echo "usage: make release VERSION=1.2.3" >&2 && exit 2 ;;
esac

if ! echo "$version" | grep -Eq '^[0-9]+\.[0-9]+\.[0-9]+(-[0-9a-z.]+)?$'; then
	echo "VERSION must be X.Y.Z or X.Y.Z-pre (got $version)" >&2
	exit 2
fi

cd "$(dirname "$0")/.."
tag=v$version
fail() { echo "release: $*" >&2 && exit 1; }

[ "$(git branch --show-current)" = main ] || fail "release from main"
[ -z "$(git status --porcelain)" ] || fail "commit or stash the changes first"
git fetch --quiet --tags origin
[ "$(git rev-parse HEAD)" = "$(git rev-parse origin/main)" ] || fail "main is not the pushed origin/main"
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
