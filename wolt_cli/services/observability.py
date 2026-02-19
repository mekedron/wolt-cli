import re
from enum import Enum
from typing import Any

from wolt_cli.models.wolt import Item, Restaurant, Section
from wolt_cli.services.observability_items import (
    ItemSort,
    build_item_detail,
    build_item_search_result,
    build_venue_menu,
    extract_menu_items,
)


class VenueSort(str, Enum):
    RECOMMENDED = "recommended"
    DISTANCE = "distance"
    RATING = "rating"
    DELIVERY_PRICE = "delivery_price"
    DELIVERY_TIME = "delivery_time"


class VenueType(str, Enum):
    RESTAURANT = "restaurant"
    GROCERY = "grocery"
    PHARMACY = "pharmacy"
    RETAIL = "retail"


def _slugify(text: str) -> str:
    normalized = re.sub(r"[^a-zA-Z0-9]+", "-", text.lower()).strip("-")
    return normalized or "unknown"


def _format_amount(amount: int | None, currency: str | None) -> str | None:
    if amount is None or currency is None:
        return None
    return f"{currency} {amount / 100:.2f}"


def _normalize_id(value: Any) -> str:
    if isinstance(value, str):
        return value
    if isinstance(value, dict):
        if oid := value.get("$oid"):
            return str(oid)
        return str(value)
    if value is None:
        return ""
    return str(value)


def build_discovery_feed(sections: list[Section], city: str | None, limit: int | None = None) -> dict[str, Any]:
    resolved_sections = sections[:limit] if limit is not None else sections
    section_rows: list[dict[str, Any]] = []

    for section in resolved_sections:
        section_items = section.items[:limit] if limit is not None else section.items
        rows: list[dict[str, Any]] = []
        for item in section_items:
            if not item.venue:
                continue
            rows.append(
                {
                    "venue_id": _normalize_id(item.venue.id or item.link.target),
                    "slug": item.venue.slug or "",
                    "name": item.title,
                    "rating": item.venue.rating.score if item.venue.rating else None,
                    "delivery_estimate": item.venue.format_estimate_range(),
                    "delivery_fee": {
                        "amount": item.venue.delivery_price_int,
                        "formatted_amount": _format_amount(item.venue.delivery_price_int, item.venue.currency),
                    },
                }
            )

        section_rows.append(
            {
                "name": section.name,
                "title": section.title or section.name,
                "items": rows,
            }
        )

    return {"city": city or "unknown", "sections": section_rows}


def build_category_list(sections: list[Section]) -> dict[str, Any]:
    categories: dict[str, dict[str, str]] = {}
    for section in sections:
        for item in section.items:
            if not item.venue:
                continue
            for tag in item.venue.tags:
                slug = _slugify(tag)
                categories[slug] = {"id": slug, "name": tag.capitalize(), "slug": slug}

    return {"categories": sorted(categories.values(), key=lambda value: value["name"])}


def build_venue_search_result(
    *,
    items: list[Item],
    query: str,
    sort: VenueSort,
    venue_type: VenueType | None,
    category: str | None,
    open_now: bool,
    wolt_plus: bool,
    limit: int | None,
    offset: int,
) -> tuple[dict[str, Any], list[str]]:
    warnings: list[str] = []
    lowered_query = query.lower().strip()
    lowered_category = category.lower().strip() if category else None

    filtered = [item for item in items if item.venue]
    filtered = [
        item
        for item in filtered
        if lowered_query in item.title.lower()
        or lowered_query in item.venue.address.lower()
        or any(lowered_query in tag.lower() for tag in item.venue.tags)
    ]

    if venue_type:
        filtered = [item for item in filtered if (item.venue.product_line or "restaurant") == venue_type.value]

    if lowered_category:
        filtered = [item for item in filtered if any(lowered_category in tag.lower() for tag in item.venue.tags)]

    if open_now:
        filtered = [item for item in filtered if bool(item.venue.online)]

    if wolt_plus:
        filtered = [item for item in filtered if bool(item.venue.show_wolt_plus)]

    if sort == VenueSort.DISTANCE:
        warnings.append("distance sort is approximated with delivery estimate in basic mode")
        filtered = sorted(filtered, key=lambda value: value.venue.estimate)
    elif sort == VenueSort.RATING:
        filtered = sorted(filtered, key=lambda value: value.venue.rating.score if value.venue.rating else 0, reverse=True)
    elif sort == VenueSort.DELIVERY_PRICE:
        filtered = sorted(filtered, key=lambda value: value.venue.delivery_price_int or 0)
    elif sort == VenueSort.DELIVERY_TIME:
        filtered = sorted(filtered, key=lambda value: value.venue.estimate)

    total = len(filtered)
    if offset > 0:
        filtered = filtered[offset:]
    if limit is not None:
        filtered = filtered[:limit]

    rows = []
    for item in filtered:
        rows.append(
            {
                "venue_id": _normalize_id(item.venue.id or item.link.target),
                "slug": item.venue.slug or "",
                "name": item.title,
                "address": item.venue.address,
                "rating": item.venue.rating.score if item.venue.rating else None,
                "delivery_estimate": item.venue.format_estimate_range(),
                "delivery_fee": {
                    "amount": item.venue.delivery_price_int,
                    "formatted_amount": _format_amount(item.venue.delivery_price_int, item.venue.currency),
                },
                "wolt_plus": bool(item.venue.show_wolt_plus),
            }
        )

    return {"query": query, "total": total, "items": rows}, warnings


def build_venue_detail(item: Item, restaurant: Restaurant, include: set[str]) -> tuple[dict[str, Any], list[str]]:
    warnings: list[str] = []
    venue = item.venue
    if venue is None:
        raise ValueError("Item does not include venue details")

    rating_value = None
    if restaurant.rating:
        rating_value = restaurant.rating.score
    elif venue.rating:
        rating_value = venue.rating.score

    minimum_amount = None
    warnings.append("order minimum is unavailable in basic mode and returned as null")

    data: dict[str, Any] = {
        "venue_id": _normalize_id(restaurant.id or venue.id or item.link.target),
        "slug": restaurant.slug or venue.slug or "",
        "name": item.title,
        "address": restaurant.address,
        "currency": restaurant.currency,
        "rating": rating_value,
        "delivery_methods": restaurant.delivery_methods,
        "order_minimum": {
            "amount": minimum_amount,
            "formatted_amount": _format_amount(minimum_amount, restaurant.currency),
        },
    }

    if "hours" in include:
        data["opening_windows"] = _build_opening_windows(restaurant)
    if "tags" in include:
        data["tags"] = restaurant.food_tags
    if "rating" in include and restaurant.rating:
        data["rating_details"] = {
            "score": restaurant.rating.score,
            "text": restaurant.rating.text,
            "volume": restaurant.rating.volume,
        }
    if "fees" in include:
        data["delivery_fee"] = {
            "amount": venue.delivery_price_int,
            "formatted_amount": _format_amount(venue.delivery_price_int, venue.currency),
        }

    return data, warnings


def _build_opening_windows(restaurant: Restaurant) -> list[dict[str, str]]:
    windows: list[dict[str, str]] = []
    weekday_order = ["monday", "tuesday", "wednesday", "thursday", "friday", "saturday", "sunday"]
    for weekday in weekday_order:
        values = restaurant.opening_times.get(weekday, [])
        open_time = next((value.format() for value in values if value.type == "open"), "-")
        close_time = next((value.format() for value in values if value.type == "close"), "-")
        windows.append({"day": weekday, "open": open_time, "close": close_time})
    return windows


def build_venue_hours(restaurant: Restaurant, timezone: str | None) -> dict[str, Any]:
    return {
        "venue_id": _normalize_id(restaurant.id or ""),
        "timezone": timezone or restaurant.timezone_name or "UTC",
        "opening_windows": _build_opening_windows(restaurant),
        "delivery_windows": [],
    }


__all__ = [
    "ItemSort",
    "VenueSort",
    "VenueType",
    "build_category_list",
    "build_discovery_feed",
    "build_item_detail",
    "build_item_search_result",
    "build_venue_detail",
    "build_venue_hours",
    "build_venue_menu",
    "build_venue_search_result",
    "extract_menu_items",
]
