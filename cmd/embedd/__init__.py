from argparse import ArgumentParser
from logging import INFO as LEVEL_INFO
from logging import basicConfig as loggingBasicConfig
from logging import getLogger

from embed import Embed, EmbedConfig

from embedd.server import Servicer, listen

_logger = getLogger("embedd")
_logger.setLevel(LEVEL_INFO)


# This is the main entrypoint for the program
def embedd():
    loggingBasicConfig(level=LEVEL_INFO)
    parser = ArgumentParser(
        prog="embedd",
        description="A gRPC service to generate vector embeddings for text inputs",
    )

    _ = parser.add_argument(
        "-a", "--addr", nargs="?", default="[::]:8150", type=str
    )
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
        overlap=int(
            args.overlap
            if args.overlap >= 1
            else args.overlap * args.chunk_size
        ),
    )
    _logger.info("running with configuration: %s", svc_config)

    svc = Embed(svc_config)
    svcr = Servicer(svc)
    server = listen(svcr, args.addr)

    _logger.info("listening on %s", args.addr)
    server.wait_for_termination()
