package embed

import (
	"time"

	flag "github.com/spf13/pflag"
)

func EmbedFlagSet() (*flag.FlagSet, func() EmbedConfig) {
	fs := flag.NewFlagSet("embed", flag.ExitOnError)

	tokenizerPath := fs.String("embed-tokenizer-path", "", "the path to the tokenizer configuration")
	onnxRuntimePath := fs.String("embed-onnx-runtime-path", "/lib/libonnxruntime.so", "the path to the libonnxruntime.so file")
	modelPath := fs.String("embed-model-path", "", "the path to the model weights")
	chunkSize := fs.Uint32("embed-chunk-size", 256, "the size of each chunk of text to embed")
	overlap := fs.Float32("embed-overlap", 0.15, "the overlap of text between chunks")
	shutdownDelay := fs.Duration("embed-shutdown-delay", time.Second, "allowed wait time for graceful shutdown of the tokenizers pool and onnxruntime")

	return fs, func() EmbedConfig {
		return EmbedConfig{
			TokenizerPath:   *tokenizerPath,
			ONNXRuntimePath: *onnxRuntimePath,
			ModelPath:       *modelPath,
			ChunkSize:       *chunkSize,
			Overlap:         *overlap,
			ShutdownDelay:   *shutdownDelay,
		}
	}
}
