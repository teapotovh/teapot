#!/bin/sh
VERSION=${GITHUB_REF_NAME:-$(git describe --tags --abbrev=0 2>/dev/null || echo 'dev')}
echo "STABLE_TEAPOT_VERSION $VERSION"

# Normalized origin: git@github.com:teapotovh/teapot.git or
# https://github.com/teapotovh/teapot.git both become github.com/teapotovh/teapot.
# Non-GitHub remotes are left empty: consumers must fail loudly on that.
REPO=$(git config --get remote.origin.url |
  sed -e 's|^git@github\.com:|github.com/|' \
      -e 's|^ssh://git@github\.com/|github.com/|' \
      -e 's|^https://github\.com/|github.com/|' \
      -e 's|\.git$||')
case "$REPO" in
  github.com/*) ;;
  *) REPO= ;;
esac
echo "STABLE_TEAPOT_REPOSITORY $REPO"
