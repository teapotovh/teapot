from dataclasses import asdict, dataclass

import numpy as np
from onnxruntime import InferenceSession
from semantic_text_splitter import TextSplitter
from tokenizers import Encoding, Tokenizer


@dataclass
class EmbedConfig:
    tokenizer_path: str
    model_path: str
    chunk_size: int
    overlap: int


@dataclass
class Input:
    input_ids: np.ndarray
    attention_mask: np.ndarray
    token_type_ids: np.ndarray


@dataclass
class Embedding:
    vector: np.ndarray
    text: str


class Embed:
    tokenizer: Tokenizer
    session: InferenceSession

    splitter: TextSplitter

    def __init__(self, config: EmbedConfig) -> None:
        self.tokenizer = Tokenizer.from_file(config.tokenizer_path)
        self.session = InferenceSession(config.model_path)

        self.splitter = TextSplitter.from_huggingface_tokenizer(
            self.tokenizer, config.chunk_size, overlap=config.overlap
        )

    def _tokenize(self, text: str) -> Input:
        encoded: Encoding = self.tokenizer.encode(text)

        input_ids = np.array([encoded.ids], dtype=np.int64)
        attention_mask = np.array([encoded.attention_mask], dtype=np.int64)
        token_type_ids = np.zeros_like(input_ids, dtype=np.int64)

        return Input(
            input_ids=input_ids,
            attention_mask=attention_mask,
            token_type_ids=token_type_ids,
        )

    def _mean_pooling(self, input: Input, output: np.ndarray) -> np.ndarray:
        input_mask_expanded = np.expand_dims(input.attention_mask, -1).astype(float)
        embeddings = np.sum(output * input_mask_expanded, 1) / np.clip(
            input_mask_expanded.sum(1), a_min=1e-9, a_max=None
        )
        return embeddings

    def embed(self, text: str) -> list[Embedding]:
        embeddings: list[Embedding] = []
        for chunk in self.splitter.chunks(text):
            embedding = self.embed_single(chunk)
            embeddings.append(embedding)

        return embeddings

    def embed_single(self, text: str) -> Embedding:
        # tokenize the input from text into a list of tokens
        input = self._tokenize(text)
        self.tokenizer
        # run inference on the tokens
        outputs = self.session.run(None, asdict(input))
        # apply mean pooling
        embeddings = self._mean_pooling(input, outputs[0])
        # normalize the result
        embeddings = embeddings / np.linalg.norm(embeddings, axis=1, keepdims=True)

        return Embedding(vector=embeddings[0], text=text)
