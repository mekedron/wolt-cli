from importlib import metadata
from importlib.metadata import PackageNotFoundError

import typer
from rich import print

try:
    __version__ = metadata.version(__package__)
except PackageNotFoundError:
    __version__ = "1.1.1"


def version_callback(value: bool) -> None:
    if value:
        print(__version__)
        raise typer.Exit()
