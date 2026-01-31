from concurrent import futures
from logging import INFO as LEVEL_INFO
from logging import getLogger
from typing import override

from grpclib.server import Stream
from grpclib.exceptions import GRPCError
from grpclib.const import Status

from service.embed import Embed
from service.embed import Embedding as LibEmbedding
from proto.embed_pb2 import (
    Embedding,
    EmbedRequest,
    EmbedReply,
    EmbedSingleReply,
    EmbedSingleRequest
)
from proto.embed_grpc import EmbedderBase


_logger = getLogger("embedd.server")
_logger.setLevel(LEVEL_INFO)


class Embedder(EmbedderBase):
    service: Embed

    def __init__(self, service: Embed) -> None:
        super().__init__()
        self.service = service

    def _validate(self, text: str) -> None:
        if len(text) <= 0:
            raise GRPCError(
                Status.INVALID_ARGUMENT,
                "Cannot generate embedding for an empty text"
            )

    def _to_embedding(self, embedding: LibEmbedding) -> Embedding:
        return Embedding(vector=embedding.vector, text=embedding.text)

    @override
    async def Embed(self, stream: Stream[EmbedRequest, EmbedReply]) -> None:
        request = await stream.recv_message()
        self._validate(request.text)

        _logger.info("got request to embed: %s", request)
        embeddings = self.service.embed(request.text)
        embeddings = (self._to_embedding(embedding) for embedding in embeddings)
        await stream.send_message(EmbedReply(embeddings=embeddings))

    @override
    async def EmbedSingle(self, stream: Stream[EmbedSingleRequest, EmbedSingleReply]) -> None:
        request = await stream.recv_message()
        self._validate(request.text)

        _logger.info("got request to embed: %s", request)
        embedding = self.service.embed_single(request.text)
        await stream.send_message(EmbedSingleReply(embedding=self._to_embedding(embedding)))
