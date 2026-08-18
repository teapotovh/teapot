package embed

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"slices"
	"time"

	"github.com/daulet/tokenizers"
	"github.com/teapotovh/teapot/lib/run"
	"github.com/teapotovh/teapot/proto/embed"
	ort "github.com/yalue/onnxruntime_go"
	"google.golang.org/grpc"
)

const (
	PassagePrefix = "passage: "
	QueryPrefix   = "query: "
)

var (
	ErrUnexpectedEmbeddingKind = errors.New("unexpected embedding kind")

	ErrInvalidChunkSize = errors.New("invalid chunk size")
	ErrInvalidOverlap   = errors.New("invalid overlap")
	ErrQueryTooLong     = errors.New("query too long")

	ErrUnexpectedOutputTensorType = errors.New("unexpected output tensor type")
	ErrMismatchedBatchSize        = errors.New("mismatched batch size")
	ErrMismatchedEmbeddingsLength = errors.New("mismatched embeddings length")
)

type EmbedConfig struct {
	TokenizerPath   string
	ONNXRuntimePath string
	ModelPath       string
	ChunkSize       uint32
	Overlap         float32
	ShutdownDelay   time.Duration
}

type Embed struct {
	logger *slog.Logger

	tokenizer     *tokenizers.Tokenizer
	passageTokens []uint32
	queryTokens   []uint32
	session       *ort.DynamicAdvancedSession

	chunkSize     int
	overlap       int
	shutdownDelay time.Duration
}

func NewEmbed(config EmbedConfig, logger *slog.Logger) (*Embed, error) {
	tokenizer, err := tokenizers.FromFile(config.TokenizerPath)
	if err != nil {
		return nil, fmt.Errorf("could not load tokenizer from %q: %w", config.TokenizerPath, err)
	}

	passageTokens, _, err := tokenizer.EncodeErr(PassagePrefix, false)
	if err != nil {
		return nil, fmt.Errorf("error tokenizing passage prefix %q: %w", PassagePrefix, err)
	}

	queryTokens, _, err := tokenizer.EncodeErr(QueryPrefix, false)
	if err != nil {
		return nil, fmt.Errorf("error tokenizing query prefix %q: %w", QueryPrefix, err)
	}

	chunkSize := int(config.ChunkSize)
	overlap := int(float32(config.ChunkSize) * config.Overlap)
	if overlap < 0 || overlap > chunkSize {
		return nil, fmt.Errorf("computed overlap must be between 0 and %d, but got %d", chunkSize, overlap)
	}

	ort.SetSharedLibraryPath(config.ONNXRuntimePath)
	err = ort.InitializeEnvironment()
	if err != nil {
		return nil, fmt.Errorf("could not load ONNX runtime: %w", err)
	}

	opts, err := ort.NewSessionOptions()
	if err != nil {
		return nil, fmt.Errorf("creating session options: %w", err)
	}
	defer opts.Destroy()

	if err := opts.SetExecutionMode(ort.ExecutionModeParallel); err != nil {
		return nil, fmt.Errorf("enabling parallel execution: %w", err)
	}

	session, err := ort.NewDynamicAdvancedSession(
		config.ModelPath,
		[]string{"input_ids", "attention_mask", "token_type_ids"},
		[]string{"last_hidden_state"},
		opts,
	)
	if err != nil {
		return nil, fmt.Errorf("creating ONNX session: %w", err)
	}

	e := Embed{
		logger: logger,

		tokenizer:     tokenizer,
		passageTokens: passageTokens,
		queryTokens:   queryTokens,
		session:       session,

		chunkSize: chunkSize,
		overlap:   overlap,
	}

	return &e, nil
}

type Embedding struct {
	Vector []float32
	Text   string
}

type EmbeddingKind uint8

const (
	EmbeddingKindPassage EmbeddingKind = iota
	EmbeddingKindQuery
)

type rng struct {
	start int
	end   int
}

func chunk(prefix []uint32, tokens []uint32, chunkSize, overlap int) ([]rng, [][]uint32, error) {
	maxTokTokens := chunkSize - len(prefix)
	step := maxTokTokens - overlap

	if maxTokTokens <= 0 {
		return nil, nil, fmt.Errorf("%w: must be larger than prefix (passage:/query:) length", ErrInvalidChunkSize)
	}
	if overlap < 0 || overlap >= maxTokTokens {
		return nil, nil, fmt.Errorf("%w: must be >= 0 and < %d", ErrInvalidOverlap, maxTokTokens)
	}

	cap := (len(tokens) + step - 1) / step
	ranges := make([]rng, 0, cap)
	chunks := make([][]uint32, 0, cap)
	for start := 0; start < len(tokens); start += step {
		end := min(start+maxTokTokens, len(tokens))

		chunk := make([]uint32, chunkSize)
		copy(chunk, prefix)
		copy(chunk[len(prefix):], tokens[start:end])
		chunks = append(chunks, chunk)
	}

	return ranges, chunks, nil
}

func (e *Embed) inference(shape ort.Shape, ids, attention []uint32) (*ort.Tensor[float32], error) {
	idsTensor, err := ort.NewTensor(shape, ids)
	if err != nil {
		return nil, fmt.Errorf("input_ids tensor: %w", err)
	}
	defer idsTensor.Destroy()

	attentionTensor, err := ort.NewTensor(shape, attention)
	if err != nil {
		return nil, fmt.Errorf("attention_mask tensor: %w", err)
	}
	defer attentionTensor.Destroy()

	tokenTypeIDs := make([]int64, len(ids))
	ttTensor, err := ort.NewTensor(shape, tokenTypeIDs)
	if err != nil {
		return nil, fmt.Errorf("token_type_ids tensor: %w", err)
	}
	defer ttTensor.Destroy()

	inputs := []ort.Value{idsTensor, attentionTensor, ttTensor}
	outputs := []ort.Value{nil}

	if err := e.session.Run(inputs, outputs); err != nil {
		return nil, fmt.Errorf("run: %w", err)
	}

	outTensor, ok := outputs[0].(*ort.Tensor[float32])
	if !ok {
		return nil, ErrUnexpectedOutputTensorType
	}
	return outTensor, nil
}

func meanPoolAndNormalize(data []float32, attentionMask []uint32, n, seqLen, hidden int) [][]float32 {
	embeddings := make([][]float32, n)
	for i := range n {
		vec := make([]float32, hidden)
		var count float32
		for j := range seqLen {
			if attentionMask[i*seqLen+j] == 0 {
				continue
			}
			base := (i*seqLen + j) * hidden
			for k := range hidden {
				vec[k] += data[base+k]
			}
			count++
		}
		if count > 0 {
			for k := range vec {
				vec[k] /= count
			}
		}
		var norm float64
		for _, v := range vec {
			norm += float64(v) * float64(v)
		}
		norm = math.Sqrt(norm)
		if norm > 0 {
			for k := range vec {
				vec[k] = float32(float64(vec[k]) / norm)
			}
		}
		embeddings[i] = vec
	}
	return embeddings
}

func (e *Embed) batchInference(chunks [][]uint32) ([][]float32, error) {
	// XLM-RoBERTa pad token id, used by multilingual-e5-small's tokenizer.
	const padTokenID uint32 = 1

	n := len(chunks)
	if n == 0 {
		return nil, nil
	}

	maxLen := len(chunks[0])
	inputIDs := make([]uint32, n*maxLen)
	attentionMask := make([]uint32, n*maxLen)
	for i, c := range chunks {
		base := i * maxLen
		copy(inputIDs[base:base+len(c)], c)
		for j := range len(c) {
			attentionMask[base+j] = 1
		}
		if len(c) < maxLen {
			for j := len(c); j < maxLen; j++ {
				inputIDs[base+j] = padTokenID
			}
		}
	}

	inputShape := ort.NewShape(int64(n), int64(maxLen))
	outTensor, err := e.inference(inputShape, inputIDs, attentionMask)
	if err != nil {
		return nil, fmt.Errorf("inference: %w", err)
	}
	defer outTensor.Destroy()

	outputShape := outTensor.GetShape() // [n, maxLen, hidden]
	N := int(outputShape[0])
	if n != N {
		return nil, fmt.Errorf("%w: expected %d, got %d", ErrMismatchedBatchSize, n, N)
	}
	hidden := int(outputShape[2])
	data := outTensor.GetData()

	embeddings := meanPoolAndNormalize(data, attentionMask, n, maxLen, hidden)
	return embeddings, nil
}

func (e *Embed) Embed(ctx context.Context, kind EmbeddingKind, text string) ([]Embedding, error) {
	tokens, _, err := e.tokenizer.EncodeErr(text, false)
	if err != nil {
		return nil, fmt.Errorf("tokenizing input: %w", err)
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	var prefixTokens []uint32
	switch kind {
	case EmbeddingKindPassage:
		prefixTokens = e.passageTokens
	case EmbeddingKindQuery:
		prefixTokens = e.queryTokens
	default:
		return nil, fmt.Errorf("%w: %d", ErrUnexpectedEmbeddingKind, kind)
	}
	ranges, chunks, err := chunk(prefixTokens, tokens, e.chunkSize, e.overlap)
	if err != nil {
		return nil, fmt.Errorf("chunking tokens: %w", err)
	}

	switch kind {
	case EmbeddingKindPassage:
		embs, err := e.batchInference(chunks)
		if err != nil {
			return nil, fmt.Errorf("embedding: %w", err)
		}
		if len(embs) != len(chunks) || len(embs) != len(ranges) {
			return nil, fmt.Errorf("%w: expected the same amoutn of embeddings as chunks, got %d embeddings, have %d chunks", ErrMismatchedEmbeddingsLength, len(embs), len(chunks))
		}

		embeddings := make([]Embedding, 0, len(embs))
		for i, emb := range embs {
			text := e.tokenizer.Decode(tokens[ranges[i].start:ranges[i].end], false)
			embeddings = append(embeddings, Embedding{
				Vector: emb,
				Text:   text,
			})
		}

		return embeddings, nil
	case EmbeddingKindQuery:
		if len(chunks) > 1 {
			return nil, fmt.Errorf("%w: expected exactly one chunk, got %d", ErrQueryTooLong, len(chunks))
		}

		chunk := chunks[0]
		n := len(chunk)
		attention := slices.Repeat([]uint32{1}, n)
		inputShape := ort.NewShape(int64(1), int64(n))
		outTensor, err := e.inference(inputShape, chunk, attention)
		if err != nil {
			return nil, fmt.Errorf("inference: %w", err)
		}

		outputShape := outTensor.GetShape() // [1, n, hidden]
		if outputShape[0] != 1 {
			return nil, fmt.Errorf("%w: expected 1, got %d", ErrMismatchedBatchSize, outputShape[0])
		}
		hidden := int(outputShape[2])
		data := outTensor.GetData()

		embeddings := meanPoolAndNormalize(data, attention, 1, n, hidden)
		text := e.tokenizer.Decode(tokens, false)
		return []Embedding{{Vector: embeddings[0], Text: text}}, nil
	default:
		return nil, fmt.Errorf("%w: %d", ErrUnexpectedEmbeddingKind, kind)
	}
}

// Register implements grpcsrv.GRPCService
func (e *Embed) Register(server *grpc.Server) {
	svc := newEmbedServer(e, e.logger)
	embed.RegisterEmbedderServer(server, svc)
}

// Run implements run.Runnable.
func (e *Embed) Run(ctx context.Context, notify run.Notify) (err error) {
	notify.Notify()

	<-ctx.Done()

	if err := e.tokenizer.Close(); err != nil {
		return fmt.Errorf("closing tokenizer: %w", err)
	}

	if err := e.session.Destroy(); err != nil {
		return fmt.Errorf("destroying ONNX session: %w", err)
	}

	if err := ort.DestroyEnvironment(); err != nil {
		return fmt.Errorf("destroying up ONNX runtime: %w", err)
	}
	return nil
}
