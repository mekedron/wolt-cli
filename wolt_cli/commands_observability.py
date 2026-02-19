from pathlib import Path
from typing import Annotated, Any

import typer
from rich.table import Table

from wolt_cli.gateways import wolt
from wolt_cli.gateways.wolt import WoltApiError
from wolt_cli.models.location import Location
from wolt_cli.services.observability import (
    ItemSort,
    VenueSort,
    VenueType,
    build_category_list,
    build_discovery_feed,
    build_item_detail,
    build_item_search_result,
    build_venue_detail,
    build_venue_hours,
    build_venue_menu,
    build_venue_search_result,
)
from wolt_cli.services.output import OutputFormat, build_envelope, emit_machine_payload, emit_table
from wolt_cli.services.profile import find_profile

discover_app = typer.Typer(
    no_args_is_help=True,
    help="Read discovery feed sections and available categories.",
)
search_app = typer.Typer(
    no_args_is_help=True,
    help="Search venues and menu items with filters.",
)
venue_app = typer.Typer(
    no_args_is_help=True,
    help="Inspect venue metadata, menu, and opening hours.",
)
item_app = typer.Typer(
    no_args_is_help=True,
    help="Inspect a single item for a venue.",
)

FormatOption = Annotated[
    OutputFormat,
    typer.Option(
        "--format",
        help="Output format: table, json, or yaml.",
        case_sensitive=False,
    ),
]
ProfileOption = Annotated[
    str | None,
    typer.Option("--profile", help="Profile to use for location resolution."),
]
LocaleOption = Annotated[
    str,
    typer.Option("--locale", help="Response locale in BCP-47 format, for example en-FI."),
]
NoColorOption = Annotated[
    bool,
    typer.Option("--no-color", help="Disable ANSI color codes in table output."),
]
OutputPathOption = Annotated[
    Path | None,
    typer.Option("--output", help="Write the command output to a file."),
]


def _emit(
    *,
    output_format: OutputFormat,
    profile: str,
    locale: str,
    data: dict[str, Any],
    warnings: list[str],
    table: Table,
    no_color: bool,
    output_path: Path | None,
) -> None:
    if output_format == OutputFormat.TABLE:
        emit_table(table, no_color=no_color, output_path=output_path)
        return

    payload = build_envelope(profile=profile, locale=locale, data=data, warnings=warnings)
    emit_machine_payload(payload, output_format, output_path)


def _split_csv(value: str | None) -> set[str]:
    if not value:
        return set()
    return {part.strip().lower() for part in value.split(",") if part.strip()}


def _emit_error(
    *,
    output_format: OutputFormat,
    profile: str,
    locale: str,
    output_path: Path | None,
    code: str,
    message: str,
) -> None:
    if output_format == OutputFormat.TABLE:
        typer.secho(message, fg=typer.colors.RED)
        raise typer.Exit(1)

    payload = build_envelope(
        profile=profile,
        locale=locale,
        data=None,
        warnings=[],
        error={"code": code, "message": message},
    )
    emit_machine_payload(payload, output_format, output_path)
    raise typer.Exit(1)


def _resolve_location(
    *,
    lat: float | None,
    lon: float | None,
    profile_name: str | None,
    output_format: OutputFormat,
    locale: str,
    output_path: Path | None,
) -> tuple[Location, str]:
    if lat is None and lon is None:
        profile = find_profile(profile_name)
        return profile.location, profile.name

    if lat is None or lon is None:
        _emit_error(
            output_format=output_format,
            profile=profile_name or "anonymous",
            locale=locale,
            output_path=output_path,
            code="WOLT_INVALID_ARGUMENT",
            message="Both --lat and --lon must be provided together, or omit both to use profile location.",
        )

    return Location(lat=lat, lon=lon), profile_name or "anonymous"


def _build_discovery_table(data: dict[str, Any]) -> Table:
    table = Table(title=f"Discover feed: {data['city']}")
    table.add_column("Section")
    table.add_column("Venue")
    table.add_column("Rating")
    table.add_column("Delivery estimate")
    table.add_column("Delivery fee")

    for section in data["sections"]:
        section_name = section["title"]
        rows = section["items"] or [{"name": "-", "rating": "-", "delivery_estimate": "-", "delivery_fee": {"formatted_amount": "-"}}]
        for index, item in enumerate(rows):
            delivery_fee = item["delivery_fee"]["formatted_amount"] or "-"
            table.add_row(
                section_name if index == 0 else "",
                str(item["name"]),
                "-" if item["rating"] is None else str(item["rating"]),
                str(item["delivery_estimate"]),
                delivery_fee,
            )
    return table


def _build_categories_table(data: dict[str, Any]) -> Table:
    table = Table(title="Discover categories")
    table.add_column("Category")
    table.add_column("Slug")
    table.add_column("ID")

    for category in data["categories"]:
        table.add_row(category["name"], category["slug"], category["id"])

    return table


def _build_venue_search_table(data: dict[str, Any]) -> Table:
    table = Table(title=f"Venue search: {data['query']}")
    table.add_column("Venue")
    table.add_column("Address")
    table.add_column("Rating")
    table.add_column("Delivery")
    table.add_column("Fee")
    table.add_column("Wolt+")

    for row in data["items"]:
        delivery_fee = row["delivery_fee"]["formatted_amount"] or "-"
        table.add_row(
            row["name"],
            row["address"],
            "-" if row["rating"] is None else str(row["rating"]),
            row["delivery_estimate"],
            delivery_fee,
            "yes" if row["wolt_plus"] else "no",
        )
    return table


def _build_item_search_table(data: dict[str, Any]) -> Table:
    table = Table(title=f"Item search: {data['query']}")
    table.add_column("Item")
    table.add_column("Venue")
    table.add_column("Price")
    table.add_column("Sold out")

    for row in data["items"]:
        price = row["base_price"]["formatted_amount"] or "-"
        table.add_row(
            row["name"],
            row["venue_slug"] or row["venue_id"] or "-",
            price,
            "yes" if row["is_sold_out"] else "no",
        )
    return table


def _build_venue_detail_table(data: dict[str, Any]) -> Table:
    table = Table(title=f"Venue: {data['name']}")
    table.add_column("Field")
    table.add_column("Value")
    table.add_row("Venue ID", data["venue_id"])
    table.add_row("Slug", data["slug"])
    table.add_row("Address", data["address"])
    table.add_row("Currency", data["currency"])
    table.add_row("Rating", "-" if data["rating"] is None else str(data["rating"]))
    table.add_row("Delivery methods", ", ".join(data["delivery_methods"]) or "-")
    minimum = data["order_minimum"]["formatted_amount"] or "-"
    table.add_row("Order minimum", minimum)

    optional_fields = ("tags", "opening_windows", "rating_details", "delivery_fee")
    for field in optional_fields:
        if field not in data:
            continue
        table.add_row(field.replace("_", " ").capitalize(), str(data[field]))
    return table


def _build_venue_menu_table(data: dict[str, Any]) -> Table:
    table = Table(title=f"Venue menu: {data['venue_id']}")
    table.add_column("Item ID")
    table.add_column("Name")
    table.add_column("Price")
    table.add_column("Option groups")

    for row in data["items"]:
        price = row["base_price"]["formatted_amount"] or "-"
        option_groups = ", ".join(row.get("option_group_ids", [])) if "option_group_ids" in row else "-"
        table.add_row(row["item_id"], row["name"], price, option_groups or "-")
    return table


def _build_venue_hours_table(data: dict[str, Any]) -> Table:
    table = Table(title=f"Venue hours ({data['timezone']})")
    table.add_column("Day")
    table.add_column("Open")
    table.add_column("Close")

    for window in data["opening_windows"]:
        table.add_row(window["day"], window["open"], window["close"])

    return table


def _build_item_detail_table(data: dict[str, Any]) -> Table:
    table = Table(title=f"Item: {data['name']}")
    table.add_column("Field")
    table.add_column("Value")
    table.add_row("Item ID", data["item_id"])
    table.add_row("Venue ID", data["venue_id"])
    table.add_row("Description", data["description"] or "-")
    table.add_row("Price", data["price"]["formatted_amount"] or "-")
    table.add_row("Option groups", str(data["option_groups"]))
    table.add_row("Upsell items", str(data["upsell_items"]))
    return table


@discover_app.command("feed", help="Show discovery feed sections and venues.")
def discover_feed(
    lat: Annotated[float | None, typer.Option("--lat", help="Latitude (optional with profile)")] = None,
    lon: Annotated[float | None, typer.Option("--lon", help="Longitude (optional with profile)")] = None,
    limit: Annotated[int | None, typer.Option("--limit", help="Limit sections and items")] = None,
    output_format: FormatOption = OutputFormat.TABLE,
    profile_name: ProfileOption = None,
    locale: LocaleOption = "en-FI",
    no_color: NoColorOption = False,
    output_path: OutputPathOption = None,
) -> None:
    """Show discovery feed sections and venues for a location."""
    try:
        location, profile = _resolve_location(
            lat=lat,
            lon=lon,
            profile_name=profile_name,
            output_format=output_format,
            locale=locale,
            output_path=output_path,
        )
        page = wolt.front_page(location)
        city = page.get("city_data", {}).get("name") or page.get("city")
        data = build_discovery_feed(wolt.sections(location), city, limit)
        _emit(
            output_format=output_format,
            profile=profile,
            locale=locale,
            data=data,
            warnings=[],
            table=_build_discovery_table(data),
            no_color=no_color,
            output_path=output_path,
        )
    except WoltApiError as exc:
        _emit_error(
            output_format=output_format,
            profile=profile,
            locale=locale,
            output_path=output_path,
            code="WOLT_UPSTREAM_ERROR",
            message=str(exc),
        )


@discover_app.command("categories", help="List available discovery categories.")
def discover_categories(
    lat: Annotated[float | None, typer.Option("--lat", help="Latitude (optional with profile)")] = None,
    lon: Annotated[float | None, typer.Option("--lon", help="Longitude (optional with profile)")] = None,
    output_format: FormatOption = OutputFormat.TABLE,
    profile_name: ProfileOption = None,
    locale: LocaleOption = "en-FI",
    no_color: NoColorOption = False,
    output_path: OutputPathOption = None,
) -> None:
    """List available discovery categories for a location."""
    try:
        location, profile = _resolve_location(
            lat=lat,
            lon=lon,
            profile_name=profile_name,
            output_format=output_format,
            locale=locale,
            output_path=output_path,
        )
        data = build_category_list(wolt.sections(location))
        _emit(
            output_format=output_format,
            profile=profile,
            locale=locale,
            data=data,
            warnings=[],
            table=_build_categories_table(data),
            no_color=no_color,
            output_path=output_path,
        )
    except WoltApiError as exc:
        _emit_error(
            output_format=output_format,
            profile=profile,
            locale=locale,
            output_path=output_path,
            code="WOLT_UPSTREAM_ERROR",
            message=str(exc),
        )


@search_app.command("venues", help="Search venues by query.")
def search_venues(
    query: Annotated[str, typer.Option(..., "--query", help="Search query")],
    sort: Annotated[
        VenueSort,
        typer.Option("--sort", case_sensitive=False, help="Sort strategy"),
    ] = VenueSort.RECOMMENDED,
    venue_type: Annotated[
        VenueType | None,
        typer.Option("--type", case_sensitive=False, help="Venue type"),
    ] = None,
    category: Annotated[str | None, typer.Option("--category", help="Category slug")] = None,
    open_now: Annotated[bool, typer.Option("--open-now", help="Only include currently open venues")] = False,
    wolt_plus: Annotated[bool, typer.Option("--wolt-plus", help="Only include Wolt+ venues")] = False,
    limit: Annotated[int | None, typer.Option("--limit", help="Limit returned rows")] = None,
    offset: Annotated[int, typer.Option("--offset", help="Offset returned rows")] = 0,
    output_format: FormatOption = OutputFormat.TABLE,
    profile_name: ProfileOption = None,
    locale: LocaleOption = "en-FI",
    no_color: NoColorOption = False,
    output_path: OutputPathOption = None,
) -> None:
    """Search venues by query with filters and sorting."""
    profile = find_profile(profile_name)
    try:
        data, warnings = build_venue_search_result(
            items=wolt.items(profile.location),
            query=query,
            sort=sort,
            venue_type=venue_type,
            category=category,
            open_now=open_now,
            wolt_plus=wolt_plus,
            limit=limit,
            offset=offset,
        )
        _emit(
            output_format=output_format,
            profile=profile.name,
            locale=locale,
            data=data,
            warnings=warnings,
            table=_build_venue_search_table(data),
            no_color=no_color,
            output_path=output_path,
        )
    except WoltApiError as exc:
        _emit_error(
            output_format=output_format,
            profile=profile.name,
            locale=locale,
            output_path=output_path,
            code="WOLT_UPSTREAM_ERROR",
            message=str(exc),
        )


@search_app.command("items", help="Search menu items by query.")
def search_items(
    query: Annotated[str, typer.Option(..., "--query", help="Search query")],
    sort: Annotated[
        ItemSort,
        typer.Option("--sort", case_sensitive=False, help="Sort strategy"),
    ] = ItemSort.RELEVANCE,
    category: Annotated[str | None, typer.Option("--category", help="Category slug")] = None,
    limit: Annotated[int | None, typer.Option("--limit", help="Limit returned rows")] = None,
    offset: Annotated[int, typer.Option("--offset", help="Offset returned rows")] = 0,
    output_format: FormatOption = OutputFormat.TABLE,
    profile_name: ProfileOption = None,
    locale: LocaleOption = "en-FI",
    no_color: NoColorOption = False,
    output_path: OutputPathOption = None,
) -> None:
    """Search menu items with fallback when search endpoint is unavailable."""
    profile = find_profile(profile_name)
    try:
        fallback_items = wolt.items(profile.location)
    except WoltApiError as exc:
        _emit_error(
            output_format=output_format,
            profile=profile.name,
            locale=locale,
            output_path=output_path,
            code="WOLT_UPSTREAM_ERROR",
            message=str(exc),
        )

    payloads: list[dict[str, Any]] = []
    warnings: list[str] = []
    try:
        payloads.append(wolt.search(profile.location, query))
    except WoltApiError:
        warnings.append("search endpoint unavailable; using basic fallback data")

    data, item_warnings = build_item_search_result(
        query=query,
        payloads=payloads,
        sort=sort,
        category=category,
        limit=limit,
        offset=offset,
        fallback_items=fallback_items,
    )
    warnings.extend(item_warnings)
    _emit(
        output_format=output_format,
        profile=profile.name,
        locale=locale,
        data=data,
        warnings=warnings,
        table=_build_item_search_table(data),
        no_color=no_color,
        output_path=output_path,
    )


@venue_app.command("show", help="Show venue details by slug.")
def venue_show(
    slug: Annotated[str, typer.Argument(..., help="Venue slug")],
    include: Annotated[str | None, typer.Option("--include", help="Include sections: hours,tags,rating,fees")] = None,
    output_format: FormatOption = OutputFormat.TABLE,
    profile_name: ProfileOption = None,
    locale: LocaleOption = "en-FI",
    no_color: NoColorOption = False,
    output_path: OutputPathOption = None,
) -> None:
    """Show venue metadata and optional sections."""
    profile = find_profile(profile_name)
    try:
        item = wolt.item_by_slug(profile.location, slug)
        if item is None:
            raise typer.BadParameter(f"venue slug '{slug}' was not found in profile '{profile.name}' catalog")

        data, warnings = build_venue_detail(item, wolt.restaurant_by_id(item.link.target), include=_split_csv(include))
        _emit(
            output_format=output_format,
            profile=profile.name,
            locale=locale,
            data=data,
            warnings=warnings,
            table=_build_venue_detail_table(data),
            no_color=no_color,
            output_path=output_path,
        )
    except WoltApiError as exc:
        _emit_error(
            output_format=output_format,
            profile=profile.name,
            locale=locale,
            output_path=output_path,
            code="WOLT_UPSTREAM_ERROR",
            message=str(exc),
        )


@venue_app.command("menu", help="Show venue menu by slug.")
def venue_menu(
    slug: Annotated[str, typer.Argument(..., help="Venue slug")],
    category: Annotated[str | None, typer.Option("--category", help="Category slug")] = None,
    include_options: Annotated[bool, typer.Option("--include-options", help="Include option group IDs")] = False,
    limit: Annotated[int | None, typer.Option("--limit", help="Limit returned rows")] = None,
    output_format: FormatOption = OutputFormat.TABLE,
    profile_name: ProfileOption = None,
    locale: LocaleOption = "en-FI",
    no_color: NoColorOption = False,
    output_path: OutputPathOption = None,
) -> None:
    """Show menu items for a venue slug."""
    profile = find_profile(profile_name)
    try:
        item = wolt.item_by_slug(profile.location, slug)
        if item is None:
            raise typer.BadParameter(f"venue slug '{slug}' was not found in profile '{profile.name}' catalog")
    except WoltApiError as exc:
        _emit_error(
            output_format=output_format,
            profile=profile.name,
            locale=locale,
            output_path=output_path,
            code="WOLT_UPSTREAM_ERROR",
            message=str(exc),
        )

    payloads: list[dict[str, Any]] = []
    warnings: list[str] = []
    try:
        payloads.append(wolt.venue_page_static(slug))
    except WoltApiError:
        warnings.append("venue static page endpoint unavailable")
    try:
        payloads.append(wolt.venue_page_dynamic(slug))
    except WoltApiError:
        warnings.append("venue dynamic page endpoint unavailable")

    data, menu_warnings = build_venue_menu(
        venue_id=item.link.target,
        payloads=payloads,
        category=category,
        include_options=include_options,
        limit=limit,
    )
    warnings.extend(menu_warnings)
    _emit(
        output_format=output_format,
        profile=profile.name,
        locale=locale,
        data=data,
        warnings=warnings,
        table=_build_venue_menu_table(data),
        no_color=no_color,
        output_path=output_path,
    )


@venue_app.command("hours", help="Show venue opening hours by slug.")
def venue_hours(
    slug: Annotated[str, typer.Argument(..., help="Venue slug")],
    timezone: Annotated[str | None, typer.Option("--timezone", help="Timezone override")] = None,
    output_format: FormatOption = OutputFormat.TABLE,
    profile_name: ProfileOption = None,
    locale: LocaleOption = "en-FI",
    no_color: NoColorOption = False,
    output_path: OutputPathOption = None,
) -> None:
    """Show opening windows for a venue."""
    profile = find_profile(profile_name)
    try:
        item = wolt.item_by_slug(profile.location, slug)
        if item is None:
            raise typer.BadParameter(f"venue slug '{slug}' was not found in profile '{profile.name}' catalog")

        data = build_venue_hours(wolt.restaurant_by_id(item.link.target), timezone=timezone)
        _emit(
            output_format=output_format,
            profile=profile.name,
            locale=locale,
            data=data,
            warnings=[],
            table=_build_venue_hours_table(data),
            no_color=no_color,
            output_path=output_path,
        )
    except WoltApiError as exc:
        _emit_error(
            output_format=output_format,
            profile=profile.name,
            locale=locale,
            output_path=output_path,
            code="WOLT_UPSTREAM_ERROR",
            message=str(exc),
        )


@item_app.command("show", help="Show item details by venue slug and item ID.")
def item_show(
    venue_slug: Annotated[str, typer.Argument(..., help="Venue slug")],
    item_id: Annotated[str, typer.Argument(..., help="Item identifier")],
    include_upsell: Annotated[bool, typer.Option("--include-upsell", help="Include upsell items")] = False,
    output_format: FormatOption = OutputFormat.TABLE,
    profile_name: ProfileOption = None,
    locale: LocaleOption = "en-FI",
    no_color: NoColorOption = False,
    output_path: OutputPathOption = None,
) -> None:
    """Show details for a single venue item."""
    profile = find_profile(profile_name)
    try:
        item = wolt.item_by_slug(profile.location, venue_slug)
        if item is None:
            raise typer.BadParameter(f"venue slug '{venue_slug}' was not found in profile '{profile.name}' catalog")
    except WoltApiError as exc:
        _emit_error(
            output_format=output_format,
            profile=profile.name,
            locale=locale,
            output_path=output_path,
            code="WOLT_UPSTREAM_ERROR",
            message=str(exc),
        )

    payload: dict[str, Any] = {}
    warnings: list[str] = []
    try:
        payload = wolt.venue_item_page(item.link.target, item_id)
    except WoltApiError:
        warnings.append("item endpoint unavailable; falling back to venue payloads")
        try:
            payload = wolt.venue_page_dynamic(venue_slug)
        except WoltApiError:
            warnings.append("venue payload fallback unavailable")

    data, item_warnings = build_item_detail(
        item_id=item_id,
        venue_id=item.link.target,
        payload=payload,
        include_upsell=include_upsell,
    )
    warnings.extend(item_warnings)
    _emit(
        output_format=output_format,
        profile=profile.name,
        locale=locale,
        data=data,
        warnings=warnings,
        table=_build_item_detail_table(data),
        no_color=no_color,
        output_path=output_path,
    )
