import itertools
import urllib.parse
from typing import Final
from typing import Any

import httpx
from pydantic import TypeAdapter

from wolt_cli.models.location import Location
from wolt_cli.models.wolt import Item, Restaurant, Section
from wolt_cli.utils import cache

consumer_wolt_api_url: Final[str] = "https://consumer-api.wolt.com/v1/pages/front"
search_wolt_api_url: Final[str] = "https://restaurant-api.wolt.com/v1/pages/search"
venue_page_api_url: Final[str] = "https://restaurant-api.wolt.com/order-xp/web/v1/pages/venue/slug/"
venue_item_api_url: Final[str] = "https://restaurant-api.wolt.com/order-xp/web/v1/pages/venue/"
restaurant_wolt_api_url: Final[str] = "https://restaurant-api.wolt.com/v3/venues/"


class WoltApiError(Exception):
    def __init__(self):
        super().__init__("[Wolt] Error when trying to get response from wolt api")


def _build_headers(locale: str = "en") -> dict[str, str]:
    return {"app-language": locale}


def _ensure_success(response: httpx.Response) -> None:
    if not response.is_success:
        raise WoltApiError()


def _request_json(
    method: str,
    url: str,
    *,
    headers: dict[str, str],
    params: str | dict[str, Any] | None = None,
    json_payload: dict[str, Any] | None = None,
) -> dict[str, Any]:
    try:
        if method == "GET":
            response = httpx.get(url, params=params, headers=headers)
        else:
            response = httpx.post(url, json=json_payload, headers=headers)
    except httpx.HTTPError as exc:
        raise WoltApiError() from exc

    _ensure_success(response)

    try:
        return response.json()
    except ValueError as exc:
        raise WoltApiError() from exc


@cache.apply()
def _front_page(location: Location) -> dict[str, Any]:
    params = urllib.parse.urlencode({"lat": location.lat, "lon": location.lon})
    return _request_json("GET", consumer_wolt_api_url, params=params, headers=_build_headers())


@cache.apply()
def _sections(location: Location) -> list[Section]:
    return TypeAdapter(list[Section]).validate_python(_front_page(location)["sections"])


@cache.apply()
def _restaurant(venue_id: str) -> Restaurant:
    return TypeAdapter(Restaurant).validate_python(
        _request_json("GET", restaurant_wolt_api_url + venue_id, headers=_build_headers())["results"][0]
    )


@cache.apply()
def _search(location: Location, query: str) -> dict[str, Any]:
    payload = {"q": query, "target": None, "lat": location.lat, "lon": location.lon}
    return _request_json(
        "POST",
        search_wolt_api_url,
        json_payload=payload,
        headers={**_build_headers(), "Content-Type": "application/json"},
    )


@cache.apply()
def _venue_page_static(slug: str) -> dict[str, Any]:
    return _request_json("GET", venue_page_api_url + f"{slug}/static", headers=_build_headers())


@cache.apply()
def _venue_page_dynamic(slug: str) -> dict[str, Any]:
    return _request_json("GET", venue_page_api_url + f"{slug}/dynamic", headers=_build_headers())


@cache.apply()
def _venue_item_page(venue_id: str, item_id: str) -> dict[str, Any]:
    return _request_json("GET", venue_item_api_url + f"{venue_id}/item/{item_id}", headers=_build_headers())


def front_page(location: Location) -> dict[str, Any]:
    return _front_page(location)


def sections(location: Location) -> list[Section]:
    return _sections(location)


def items(location: Location) -> list[Item]:
    return list({item for item in itertools.chain.from_iterable(s.items for s in sections(location)) if item.venue})


def restaurant(item: Item) -> Restaurant:
    return _restaurant(item.link.target)


def restaurant_by_id(venue_id: str) -> Restaurant:
    return _restaurant(venue_id)


def search(location: Location, query: str) -> dict[str, Any]:
    return _search(location, query)


def venue_page_static(slug: str) -> dict[str, Any]:
    return _venue_page_static(slug)


def venue_page_dynamic(slug: str) -> dict[str, Any]:
    return _venue_page_dynamic(slug)


def venue_item_page(venue_id: str, item_id: str) -> dict[str, Any]:
    return _venue_item_page(venue_id, item_id)


def item_by_slug(location: Location, slug: str) -> Item | None:
    return next((item for item in items(location) if item.venue and item.venue.slug == slug), None)
