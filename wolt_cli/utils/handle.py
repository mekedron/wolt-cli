from collections.abc import Callable
from functools import wraps
from typing import TypeVar

import typer
from rich import print

T = TypeVar("T")


def exception(expected_exception: type[Exception], exit_code: int = 1) -> Callable[..., T]:
    def decorator(func: Callable[..., T]):
        @wraps(func)
        def wrapper(*args, **kwargs) -> T:
            try:
                return func(*args, **kwargs)
            except typer.Exit:
                raise
            except expected_exception as e:
                print(f"[red u]{e}[/]")
            except Exception as e:
                print(f"[red u]💥 Unexpected error: {e}[/]")
            finally:
                raise typer.Exit(code=exit_code)

        return wrapper

    return decorator
