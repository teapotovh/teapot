from argparse import ArgumentParser
from logging import INFO as LEVEL_INFO
from logging import basicConfig as loggingBasicConfig
from logging import getLogger

from grpclib.utils import graceful_exit
from grpclib.server import Server

from service.embed import Embed, EmbedConfig
from cmd.embedd.server import Embedder

_logger = getLogger("embedd")
_logger.setLevel(LEVEL_INFO)


# This is the main entrypoint for the program
async def embedd() -> None:
    loggingBasicConfig(level=LEVEL_INFO)
    parser = ArgumentParser(
        prog="embedd",
        description="A gRPC service to generate vector embeddings for text inputs",
    )

    _ = parser.add_argument("-a", "--host", nargs="?", default="0.0.0.0", type=str)
    _ = parser.add_argument("-p", "--port", nargs="?", default="8150", type=int)
    _ = parser.add_argument("-t", "--tokenizer-path", type=str)
    _ = parser.add_argument("-m", "--model-path", type=str)
    _ = parser.add_argument("-c", "--chunk_size", type=int, default=196)
    _ = parser.add_argument("-o", "--overlap", type=float, default=0.15)
    args = parser.parse_args()

    svc_config = EmbedConfig(
        tokenizer_path=args.tokenizer_path,
        model_path=args.model_path,
        chunk_size=args.chunk_size,
        # support both overlap=10 for 10 tokens overlap, or overlap=0.1 for 10% token overlap
        overlap=int(args.overlap if args.overlap >= 1 else args.overlap * args.chunk_size),
    )
    _logger.info("running with configuration: %s", svc_config)

    svc = Embed(svc_config)
    svcr = Embedder(svc)
    server = Server([svcr])

    with graceful_exit([server]):
        await server.start(args.host, args.port)
        _logger.info("listening on %s:%d", args.host, args.port)
        await server.wait_closed()
