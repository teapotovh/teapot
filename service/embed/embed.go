package embed

import (
	"context"
	// "github.com/teapotovh/teapot/proto/embed"
)

type Embed struct {
	embed.UnimplementedEmbedderServer
}

// EmbedSingle implements embed.EmbedderServer
func (*Embed) EmbedSingle(context.Context, *embed.EmbedSingleRequest) (*embed.EmbedSingleReply, error) {
}

// Embed implements embed.EmbedderServer
func (*Embed) Embed(context.Context, *embed.EmbedRequest) (*embed.EmbedReply, error) {
}
