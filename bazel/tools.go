//go:build bazel

package bazel

import (
	_ "github.com/planetscale/vtprotobuf/cmd/protoc-gen-go-vtproto"
	_ "golang.org/x/tools/gopls"
)
