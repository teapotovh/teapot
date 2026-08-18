package embed

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"google.golang.org/grpc/codes"

	"github.com/teapotovh/teapot/lib/grpcerror"
	"github.com/teapotovh/teapot/proto/embed"
)

var (
	ErrEmbedEmpty        = errors.New("cannot generate embedding for empty text")
	ErrTooManyEmbeddings = errors.New("too many embeddings")
)

type embedServer struct {
	embed.UnimplementedEmbedderServer

	logger *slog.Logger

	embed *Embed
}

func newEmbedServer(embed *Embed, logger *slog.Logger) *embedServer {
	es := embedServer{
		logger: logger,
		embed:  embed,
	}

	return &es
}

func toGRPCEmbedding(e *Embedding) *embed.Embedding {
	return &embed.Embedding{
		Vector: e.Vector,
		Text:   e.Text,
	}
}

// EmbedPassage implements embed.EmbedderServer.
func (es *embedServer) EmbedPassage(
	ctx context.Context,
	req *embed.EmbedPassageRequest,
) (*embed.EmbedPassageResponse, error) {
	text := req.GetText()
	if err := es.validate(text); err != nil {
		return nil, grpcerror.Wrap(codes.InvalidArgument, err)
	}

	es.logger.DebugContext(ctx, "received passage embed request", "length", len(text))

	embeddings, err := es.embed.Embed(ctx, EmbeddingKindPassage, text)
	if err != nil {
		return nil, grpcerror.Wrap(codes.Internal, err)
	}

	es.logger.InfoContext(ctx, "embedded into multiple vectors", "embeddings", len(embeddings))

	embs := make([]*embed.Embedding, 0, len(embeddings))
	for _, embedding := range embeddings {
		emb := toGRPCEmbedding(&embedding)
		embs = append(embs, emb)
	}

	res := embed.EmbedPassageResponse{
		Embeddings: embs,
	}

	return &res, nil
}

// EmbedQuery implements embed.EmbedderServer.
func (es *embedServer) EmbedQuery(
	ctx context.Context,
	req *embed.EmbedQueryRequest,
) (*embed.EmbedQueryResponse, error) {
	text := req.GetText()
	if err := es.validate(text); err != nil {
		return nil, grpcerror.Wrap(codes.InvalidArgument, err)
	}

	es.logger.DebugContext(ctx, "received query embed request", "length", len(text))

	embeddings, err := es.embed.Embed(ctx, EmbeddingKindQuery, text)
	if err != nil {
		return nil, grpcerror.Wrap(codes.Internal, err)
	}

	if len(embeddings) > 1 {
		return nil, fmt.Errorf("%w: query input returned more than one embedding", ErrTooManyEmbeddings)
	}

	embedding := embeddings[0]
	es.logger.InfoContext(
		ctx,
		"embedded into single vector",
		"text_length",
		len(embedding.Text),
		"vector_length",
		len(embedding.Vector),
	)
	emb := toGRPCEmbedding(&embedding)

	res := embed.EmbedQueryResponse{
		Embedding: emb,
	}

	return &res, nil
}

func (es *embedServer) validate(text string) error {
	if len(text) <= 0 {
		return ErrEmbedEmpty
	}

	return nil
}
