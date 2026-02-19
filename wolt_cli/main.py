import textwrap
from typing import Optional

import click
import typer
from rich.progress import Progress, SpinnerColumn, TextColumn
from typer.core import TyperGroup

from wolt_cli import config, controllers, version_callback
from wolt_cli.commands_observability import discover_app, item_app, search_app, venue_app
from wolt_cli.gateways import wolt
from wolt_cli.gateways.location import LocationError
from wolt_cli.gateways.wolt import WoltApiError
from wolt_cli.models.query import Ordering, Sort
from wolt_cli.services.profile import find_profile
from wolt_cli.utils import handle, validators


class CompactDocsTyperGroup(TyperGroup):
    def format_help(self, ctx: click.Context, formatter: click.HelpFormatter) -> None:
        if ctx.parent is not None:
            return super().format_help(ctx, formatter)

        command_name = ctx.command_path or ctx.info_name or "wolt-cli"
        summary = (self.help or "").strip()
        click.echo(f"{command_name}: {summary}" if summary else command_name)
        click.echo("")
        click.echo(f"usage: {command_name} <command> [options]")

        global_opts = self._format_global_options(ctx)
        if global_opts:
            click.echo("global options:")
            for option in global_opts:
                click.echo(f"  {option}")

        click.echo("")
        click.echo("commands:")
        for name in self.list_commands(ctx):
            command = self.get_command(ctx, name)
            if command is None or command.hidden:
                continue
            click.echo(f"  {name}")
            click.echo(f"    {self._summary(command)}")

        click.echo("full reference:")
        self._emit_reference(group=self, ctx=ctx, path=command_name)

    @staticmethod
    def _summary(command: click.Command) -> str:
        for text in (command.short_help, command.help):
            if text:
                return text.strip().splitlines()[0]
        return "-"

    @staticmethod
    def _option_display(option: click.Option, *, include_metavar: bool, ctx: click.Context) -> str:
        long_opt = next((opt for opt in option.opts if opt.startswith("--")), option.opts[0])
        short_opt = next((opt for opt in option.opts if opt.startswith("-") and not opt.startswith("--")), None)
        label = f"{long_opt}/{short_opt}" if short_opt else long_opt
        if not include_metavar or option.is_flag:
            return label
        try:
            metavar = option.make_metavar(ctx).strip()
        except TypeError:
            metavar = option.make_metavar().strip()
        metavar = metavar or option.name.upper()
        return f"{label} {metavar}"

    @classmethod
    def _param_signature(cls, param: click.Parameter, *, ctx: click.Context) -> str | None:
        if isinstance(param, click.Option):
            if param.name == "help" or param.hidden:
                return None
            token = cls._option_display(param, include_metavar=True, ctx=ctx)
            return token if param.required else f"[{token}]"

        if isinstance(param, click.Argument):
            if param.hidden:
                return None
            token = f"<{param.name.upper()}>"
            return token if param.required else f"[{token}]"

        return None

    @classmethod
    def _format_global_options(cls, ctx: click.Context) -> list[str]:
        options: list[str] = []
        for param in ctx.command.get_params(ctx):
            if not isinstance(param, click.Option) or param.hidden:
                continue
            options.append(cls._option_display(param, include_metavar=False, ctx=ctx))
        return options

    @classmethod
    def _emit_reference(cls, *, group: click.MultiCommand, ctx: click.Context, path: str) -> None:
        for name in group.list_commands(ctx):
            command = group.get_command(ctx, name)
            if command is None or command.hidden:
                continue

            child_ctx = click.Context(command, info_name=name, parent=ctx)
            params = [
                signature
                for param in command.get_params(child_ctx)
                if (signature := cls._param_signature(param, ctx=child_ctx)) is not None
            ]
            signature = f"{path} {name}"
            if params:
                signature = f"{signature} {' '.join(params)}"
            click.echo(textwrap.fill(signature, width=110, initial_indent="- ", subsequent_indent="  "))
            click.echo(f"  {cls._summary(command)}")
            click.echo("")

            if isinstance(command, click.MultiCommand):
                cls._emit_reference(group=command, ctx=child_ctx, path=f"{path} {name}")


app = typer.Typer(
    no_args_is_help=True,
    cls=CompactDocsTyperGroup,
    help="Browse Wolt venues, inspect menus, and manage local profiles.",
)
app.add_typer(discover_app, name="discover", help="Read discovery feed and browse categories.")
app.add_typer(search_app, name="search", help="Search venues and menu items by query.")
app.add_typer(venue_app, name="venue", help="Inspect venue details, menus, and opening hours.")
app.add_typer(item_app, name="item", help="Inspect a single menu item for a venue.")


@app.callback()
def common(
    _: bool = typer.Option(
        None,
        "--version",
        "-v",
        callback=version_callback,
        is_eager=True,
        help="Show CLI version and exit.",
    ),
):
    pass


@app.command()
@handle.exception(WoltApiError)
def ls(
    restaurant: Optional[str] = typer.Argument(  # noqa: U007
        default=None,
        help="Restaurant name or slug to inspect directly.",
        callback=validators.validate_restaurant,
    ),
    query: Optional[str] = typer.Option(  # noqa: U007
        None,
        "--query",
        "-q",
        help="Filter by restaurant name, address, or tags.",
        callback=validators.validate_query,
    ),
    profile_name: Optional[str] = typer.Option(
        None,
        "--profile",
        "-p",
        help="Profile to use for location resolution.",
    ),  # noqa: U007
    tag: Optional[str] = typer.Option(None, "--tag", "-t", help="Filter results by tag."),  # noqa: U007
    sort: Sort = typer.Option(
        Sort.NONE,
        "--sort",
        "-s",
        help=f"Sort rows by: {', '.join(Sort.choices())}.",
        case_sensitive=False,
    ),
    ordering: Ordering = typer.Option(
        Ordering.ASC,
        "--ordering",
        "-o",
        help=f"Sort ordering: {', '.join(Ordering.choices())}.",
        case_sensitive=False,
    ),
    limit: Optional[int] = typer.Option(None, "--limit", "-l", help="Maximum number of rows to show."),  # noqa: U007
) -> None:
    """List restaurants near the selected profile location."""
    profile = find_profile(profile_name)
    with Progress(
        SpinnerColumn(),
        TextColumn("[progress.description]{task.description}"),
        transient=True,
    ) as progress:
        progress.add_task(description="Fetching data...", total=None)
        items = wolt.items(location=profile.location)

    if restaurant:
        return controllers.restaurant_controller(items, restaurant)
    return controllers.items_controller(items, profile, query, tag, sort, ordering, limit)


@app.command()
@handle.exception(LocationError)
def configure() -> None:
    """Create and manage local profile configuration."""
    config.manage()
