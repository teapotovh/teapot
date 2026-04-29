#!/bin/sh
VERSION=${GITHUB_REF_NAME:-$(git describe --tags --abbrev=0 2>/dev/null || echo 'dev')}
echo "STABLE_TEAPOT_VERSION $VERSION"
