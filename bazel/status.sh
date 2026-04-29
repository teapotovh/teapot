#!/bin/sh
VERSION=${IMAGE_TAG:-$(git describe --tags --abbrev=0 2>/dev/null || echo 'dev')}
echo "STABLE_TEAPOT_VERSION $VERSION"
