from enum import Enum
from typing import Any

from wolt_cli.models.wolt import Item


class ItemSort(str, Enum):
    RELEVANCE = "relevance"
    PRICE = "price"
    NAME = "name"


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


def _walk_objects(node: Any) -> list[dict[str, Any]]:
    objects: list[dict[str, Any]] = []

    def _walk(value: Any) -> None:
        if isinstance(value, dict):
            objects.append(value)
            for nested in value.values():
                _walk(nested)
            return
        if isinstance(value, list):
            for nested in value:
                _walk(nested)

    _walk(node)
    return objects


def _extract_amount(node: dict[str, Any]) -> int | None:
    for key in ("base_price", "price_int", "amount", "minor_units"):
        value = node.get(key)
        if isinstance(value, (int, float)):
            return int(value)
    for key in ("price", "basePrice", "base_price"):
        value = node.get(key)
        if isinstance(value, dict):
            nested = _extract_amount(value)
            if nested is not None:
                return nested
        if isinstance(value, (int, float)):
            return int(value)
    return None


def _extract_currency(node: dict[str, Any]) -> str | None:
    for key in ("currency", "currency_code", "currencyCode"):
        value = node.get(key)
        if isinstance(value, str) and value.strip():
            return value.strip()
    for key in ("price", "basePrice"):
        value = node.get(key)
        if isinstance(value, dict):
            nested = _extract_currency(value)
            if nested:
                return nested
    return None


def _extract_option_group_ids(node: dict[str, Any]) -> list[str]:
    option_group_ids = node.get("option_group_ids")
    if isinstance(option_group_ids, list):
        return [str(value) for value in option_group_ids if value is not None]

    groups = node.get("option_groups")
    if not isinstance(groups, list):
        return []

    ids: list[str] = []
    for group in groups:
        if not isinstance(group, dict):
            continue
        group_id = group.get("group_id") or group.get("id")
        if group_id is None:
            continue
        ids.append(str(group_id))
    return ids


def _extract_option_groups(node: Any) -> list[dict[str, Any]]:
    groups: list[dict[str, Any]] = []
    for obj in _walk_objects(node):
        group_list = obj.get("option_groups")
        if not isinstance(group_list, list):
            continue
        for group in group_list:
            if not isinstance(group, dict):
                continue
            group_id = group.get("group_id") or group.get("id")
            name = group.get("name") or group.get("title")
            if group_id is None or not isinstance(name, str):
                continue
            groups.append(
                {
                    "group_id": str(group_id),
                    "name": name,
                    "required": bool(group.get("required", False)),
                    "min": int(group.get("min", 0)),
                    "max": int(group.get("max", 0)),
                }
            )
    deduped: dict[str, dict[str, Any]] = {}
    for group in groups:
        deduped[group["group_id"]] = group
    return list(deduped.values())


def _extract_upsell_items(node: Any) -> list[dict[str, Any]]:
    candidate_keys = ("upsell_items", "related_items", "recommended_items")
    upsell: list[dict[str, Any]] = []
    for obj in _walk_objects(node):
        for key in candidate_keys:
            value = obj.get(key)
            if not isinstance(value, list):
                continue
            for item in value:
                if not isinstance(item, dict):
                    continue
                item_id = item.get("item_id") or item.get("id")
                name = item.get("name") or item.get("title")
                if item_id is None or not isinstance(name, str):
                    continue
                amount = _extract_amount(item)
                currency = _extract_currency(item)
                upsell.append(
                    {
                        "item_id": str(item_id),
                        "name": name,
                        "price": {
                            "amount": amount,
                            "formatted_amount": _format_amount(amount, currency),
                        },
                    }
                )
    deduped: dict[str, dict[str, Any]] = {}
    for item in upsell:
        deduped[item["item_id"]] = item
    return list(deduped.values())


def extract_menu_items(payload: dict[str, Any], *, venue_id: str | None = None, venue_slug: str | None = None) -> list[dict[str, Any]]:
    items: list[dict[str, Any]] = []
    seen: set[tuple[str, str, str]] = set()
    for obj in _walk_objects(payload):
        item_id = obj.get("item_id") or obj.get("id")
        name = obj.get("name") or obj.get("title")
        if item_id is None or not isinstance(name, str):
            continue

        signal_keys = {"option_group_ids", "option_groups", "base_price", "price", "is_sold_out", "sold_out", "item_id"}
        if signal_keys.isdisjoint(obj.keys()):
            continue

        resolved_item_id = str(item_id)
        resolved_venue_id = str(obj.get("venue_id") or venue_id or "")
        resolved_venue_slug = str(obj.get("venue_slug") or venue_slug or "")
        dedupe_key = (resolved_item_id, name, resolved_venue_id)
        if dedupe_key in seen:
            continue
        seen.add(dedupe_key)

        amount = _extract_amount(obj)
        currency = _extract_currency(obj)
        category_name = obj.get("category_name") or obj.get("category") or obj.get("section_name") or "uncategorized"
        sold_out = bool(obj.get("is_sold_out") or obj.get("sold_out"))
        items.append(
            {
                "item_id": resolved_item_id,
                "venue_id": resolved_venue_id,
                "venue_slug": resolved_venue_slug,
                "name": name,
                "description": obj.get("description") if isinstance(obj.get("description"), str) else "",
                "base_price": {
                    "amount": amount,
                    "currency": currency,
                    "formatted_amount": _format_amount(amount, currency),
                },
                "option_group_ids": _extract_option_group_ids(obj),
                "category": str(category_name),
                "is_sold_out": sold_out,
            }
        )
    return items


def build_venue_menu(
    *,
    venue_id: str,
    payloads: list[dict[str, Any]],
    category: str | None,
    include_options: bool,
    limit: int | None,
) -> tuple[dict[str, Any], list[str]]:
    warnings: list[str] = []
    menu_items: list[dict[str, Any]] = []

    for payload in payloads:
        menu_items.extend(extract_menu_items(payload, venue_id=venue_id))

    if category:
        lowered_category = category.lower().strip()
        menu_items = [item for item in menu_items if lowered_category in item["category"].lower()]

    if limit is not None:
        menu_items = menu_items[:limit]

    if not menu_items:
        warnings.append("no menu items were discovered in upstream venue payloads")

    categories = sorted({item["category"] for item in menu_items})
    rows: list[dict[str, Any]] = []
    for item in menu_items:
        row: dict[str, Any] = {
            "item_id": item["item_id"],
            "name": item["name"],
            "base_price": item["base_price"],
        }
        if include_options:
            row["option_group_ids"] = item["option_group_ids"]
        rows.append(row)

    return {"venue_id": venue_id, "categories": categories, "items": rows}, warnings


def build_item_search_result(
    *,
    query: str,
    payloads: list[dict[str, Any]],
    sort: ItemSort,
    category: str | None,
    limit: int | None,
    offset: int,
    fallback_items: list[Item] | None = None,
) -> tuple[dict[str, Any], list[str]]:
    warnings: list[str] = []
    menu_items: list[dict[str, Any]] = []
    lowered_query = query.lower().strip()
    lowered_category = category.lower().strip() if category else None

    for payload in payloads:
        menu_items.extend(extract_menu_items(payload))

    menu_items = [item for item in menu_items if lowered_query in item["name"].lower()]
    if lowered_category:
        menu_items = [item for item in menu_items if lowered_category in item["category"].lower()]

    if not menu_items and fallback_items:
        warnings.append("item-level search is unavailable upstream; returning venue-level placeholders")
        for item in fallback_items:
            if not item.venue:
                continue
            if lowered_query not in item.title.lower():
                continue
            menu_items.append(
                {
                    "item_id": item.track_id,
                    "venue_id": _normalize_id(item.venue.id or item.link.target),
                    "venue_slug": item.venue.slug or "",
                    "name": item.title,
                    "base_price": {
                        "amount": None,
                        "currency": item.venue.currency,
                        "formatted_amount": None,
                    },
                    "category": "venue",
                    "is_sold_out": False,
                }
            )

    if sort == ItemSort.PRICE:
        menu_items = sorted(menu_items, key=lambda value: value["base_price"]["amount"] or 0)
    elif sort == ItemSort.NAME:
        menu_items = sorted(menu_items, key=lambda value: value["name"].lower())

    total = len(menu_items)
    if offset > 0:
        menu_items = menu_items[offset:]
    if limit is not None:
        menu_items = menu_items[:limit]

    rows = [
        {
            "item_id": item["item_id"],
            "venue_id": item.get("venue_id", ""),
            "venue_slug": item.get("venue_slug", ""),
            "name": item["name"],
            "base_price": item["base_price"],
            "currency": item["base_price"].get("currency"),
            "is_sold_out": bool(item.get("is_sold_out", False)),
        }
        for item in menu_items
    ]

    return {"query": query, "total": total, "items": rows}, warnings


def build_item_detail(
    *,
    item_id: str,
    venue_id: str,
    payload: dict[str, Any],
    include_upsell: bool,
) -> tuple[dict[str, Any], list[str]]:
    warnings: list[str] = []

    menu_items = extract_menu_items(payload, venue_id=venue_id)
    source_item = next((item for item in menu_items if item["item_id"] == item_id), None)

    if source_item is None:
        warnings.append("item payload did not contain a complete menu entry; returning minimal details")
        source_item = {
            "item_id": item_id,
            "name": item_id,
            "description": "",
            "base_price": {
                "amount": None,
                "currency": None,
                "formatted_amount": None,
            },
        }

    data: dict[str, Any] = {
        "item_id": item_id,
        "venue_id": venue_id,
        "name": source_item["name"],
        "description": source_item.get("description", ""),
        "price": source_item["base_price"],
        "option_groups": _extract_option_groups(payload),
        "upsell_items": _extract_upsell_items(payload) if include_upsell else [],
    }
    return data, warnings
