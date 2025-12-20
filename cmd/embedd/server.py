from concurrent import futures
from logging import INFO as LEVEL_INFO
from logging import getLogger

from embed import Embed
from embed import Embedding as LibEmbedding
from grpc import Server, StatusCode
from grpc import server as grpc_server

from proto.embed import (
    EmbedderServicer,
    Embedding,
    EmbedReply,
    EmbedRequest,
    EmbedSingleReply,
    EmbedSingleRequest,
    add_EmbedderServicer_to_server,
)

_logger = getLogger("embedd.server")
_logger.setLevel(LEVEL_INFO)


class Servicer(EmbedderServicer):
    service: Embed

    def __init__(self, service: Embed) -> None:
        super().__init__()
        self.service = service

    def _is_valid(self, text: str, context) -> bool:
        if len(text) <= 0:
            context.set_code(StatusCode.INVALID_ARGUMENT)
            context.set_details("Cannot generate embedding for an empty text")
            return False

        return True

    def _to_embedding(self, embedding: LibEmbedding) -> Embedding:
        return Embedding(vector=embedding.vector, text=embedding.text)

    def Embed(self, request: EmbedRequest, context) -> EmbedReply:
        if not self._is_valid(request.text, context):
            return EmbedReply()

        _logger.info("got request to embed: %s", request)
        embeddings = self.service.embed(request.text)
        embeddings = (self._to_embedding(embedding) for embedding in embeddings)
        return EmbedReply(embeddings=embeddings)

    def EmbedSingle(
        self, request: EmbedSingleRequest, context
    ) -> EmbedSingleReply:
        if not self._is_valid(request.text, context):
            return EmbedSingleReply()

        _logger.info("got request to embed: %s", request)
        embedding = self.service.embed_single(request.text)
        return EmbedSingleReply(embedding=self._to_embedding(embedding))


def listen(svcr: Servicer, addr: str) -> Server:
    server = grpc_server(futures.ThreadPoolExecutor(max_workers=10))
    add_EmbedderServicer_to_server(svcr, server)
    _ = server.add_insecure_port(addr)
    server.start()
    return server
