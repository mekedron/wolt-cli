import json

from typer.testing import CliRunner

from tests.factories import ItemFactory, ProfileFactory, VenueFactory
from wolt_cli.gateways.wolt import WoltApiError
from wolt_cli.main import app
from wolt_cli.models.wolt import Rating, Restaurant, Section, Translation

runner = CliRunner()


def _build_restaurant(venue_id: str, slug: str, name: str) -> Restaurant:
    return Restaurant(
        id=venue_id,
        slug=slug,
        name=[Translation(lang="en", value=name)],
        address="Street 1",
        city="Krakow",
        country="POL",
        currency="PLN",
        food_tags=["burger"],
        price_range=2,
        public_url="https://wolt.com/test",
        allowed_payment_methods=["card"],
        description=[Translation(lang="en", value="Description")],
        delivery_methods=["homedelivery"],
    )


def test_discover_feed_json(monkeypatch) -> None:
    venue = VenueFactory.create(id="venue-1", slug="venue-one")
    section = Section(name="popular", title="Popular", items=[ItemFactory.create(title="Venue One", venue=venue)])
    monkeypatch.setattr("wolt_cli.commands_observability.wolt.front_page", lambda _: {"city_data": {"name": "Krakow"}})
    monkeypatch.setattr("wolt_cli.commands_observability.wolt.sections", lambda _: [section])

    result = runner.invoke(app, ["discover", "feed", "--lat", "50.0", "--lon", "19.0", "--format", "json"])

    assert result.exit_code == 0
    payload = json.loads(result.output)
    assert payload["data"]["city"] == "Krakow"
    assert payload["data"]["sections"][0]["items"][0]["venue_id"] == "venue-1"
    assert payload["data"]["sections"][0]["items"][0]["delivery_fee"]["formatted_amount"] == "PLN 10.00"


def test_discover_feed_uses_default_profile_location(monkeypatch) -> None:
    profile = ProfileFactory.create()
    venue = VenueFactory.create(id="venue-1", slug="venue-one")
    section = Section(name="popular", title="Popular", items=[ItemFactory.create(title="Venue One", venue=venue)])
    seen: dict[str, float] = {}

    def _front_page(location):
        seen["lat"] = location.lat
        seen["lon"] = location.lon
        return {"city_data": {"name": "Krakow"}}

    monkeypatch.setattr("wolt_cli.commands_observability.find_profile", lambda _: profile)
    monkeypatch.setattr("wolt_cli.commands_observability.wolt.front_page", _front_page)
    monkeypatch.setattr("wolt_cli.commands_observability.wolt.sections", lambda _: [section])

    result = runner.invoke(app, ["discover", "feed", "--format", "json"])

    assert result.exit_code == 0
    assert seen == {"lat": profile.location.lat, "lon": profile.location.lon}


def test_discover_feed_requires_lat_and_lon_together(monkeypatch) -> None:
    result = runner.invoke(app, ["discover", "feed", "--lat", "50.0", "--format", "json"])

    assert result.exit_code == 1
    payload = json.loads(result.output)
    assert payload["error"]["code"] == "WOLT_INVALID_ARGUMENT"
    assert "both --lat and --lon" in payload["error"]["message"].lower()


def test_search_venues_json(monkeypatch) -> None:
    matching = ItemFactory.create(
        title="Burger Place",
        venue=VenueFactory.create(
            id="venue-1",
            slug="burger-place",
            address="Burger Street",
            show_wolt_plus=True,
            rating=Rating(rating=3, score=9.1),
        ),
    )
    non_matching = ItemFactory.create(
        title="Sushi Place",
        venue=VenueFactory.create(
            id="venue-2",
            slug="sushi-place",
            address="Sushi Street",
            show_wolt_plus=False,
            rating=Rating(rating=3, score=8.4),
        ),
    )
    profile = ProfileFactory.create()

    monkeypatch.setattr("wolt_cli.commands_observability.find_profile", lambda _: profile)
    monkeypatch.setattr("wolt_cli.commands_observability.wolt.items", lambda _: [matching, non_matching])

    result = runner.invoke(app, ["search", "venues", "--query", "burger", "--format", "json"])

    assert result.exit_code == 0
    payload = json.loads(result.output)
    assert payload["data"]["query"] == "burger"
    assert payload["data"]["total"] == 1
    assert payload["data"]["items"][0]["slug"] == "burger-place"


def test_search_items_fallback_json(monkeypatch) -> None:
    profile = ProfileFactory.create()
    fallback_item = ItemFactory.create(
        title="Whopper Meal",
        track_id="item-track-1",
        venue=VenueFactory.create(id="venue-1", slug="burger-place"),
    )
    monkeypatch.setattr("wolt_cli.commands_observability.find_profile", lambda _: profile)
    monkeypatch.setattr("wolt_cli.commands_observability.wolt.items", lambda _: [fallback_item])
    monkeypatch.setattr(
        "wolt_cli.commands_observability.wolt.search",
        lambda _, __: (_ for _ in ()).throw(WoltApiError()),
    )

    result = runner.invoke(app, ["search", "items", "--query", "whopper", "--format", "json"])

    assert result.exit_code == 0
    payload = json.loads(result.output)
    assert payload["data"]["total"] == 1
    assert payload["data"]["items"][0]["item_id"] == "item-track-1"
    assert "fallback" in " ".join(payload["warnings"]).lower()


def test_venue_show_json(monkeypatch) -> None:
    profile = ProfileFactory.create()
    venue_item = ItemFactory.create(
        title="Burger Place",
        link={"target": "venue-1"},
        venue=VenueFactory.create(id="venue-1", slug="burger-place", address="Burger Street"),
    )
    restaurant = _build_restaurant("venue-1", "burger-place", "Burger Place")

    monkeypatch.setattr("wolt_cli.commands_observability.find_profile", lambda _: profile)
    monkeypatch.setattr("wolt_cli.commands_observability.wolt.item_by_slug", lambda _, __: venue_item)
    monkeypatch.setattr("wolt_cli.commands_observability.wolt.restaurant_by_id", lambda _: restaurant)

    result = runner.invoke(app, ["venue", "show", "burger-place", "--include", "tags", "--format", "json"])

    assert result.exit_code == 0
    payload = json.loads(result.output)
    assert payload["data"]["venue_id"] == "venue-1"
    assert payload["data"]["slug"] == "burger-place"
    assert payload["data"]["tags"] == ["burger"]


def test_item_show_json(monkeypatch) -> None:
    profile = ProfileFactory.create()
    venue_item = ItemFactory.create(
        title="Burger Place",
        link={"target": "venue-1"},
        venue=VenueFactory.create(id="venue-1", slug="burger-place"),
    )
    item_payload = {
        "item_id": "item-1",
        "name": "Whopper Meal",
        "description": "Burger with fries",
        "price": {"amount": 1595, "currency": "PLN"},
        "option_groups": [{"id": "group-1", "name": "Choose drink", "required": True, "min": 1, "max": 1}],
        "upsell_items": [{"item_id": "item-2", "name": "Nuggets", "price": {"amount": 745, "currency": "PLN"}}],
    }

    monkeypatch.setattr("wolt_cli.commands_observability.find_profile", lambda _: profile)
    monkeypatch.setattr("wolt_cli.commands_observability.wolt.item_by_slug", lambda _, __: venue_item)
    monkeypatch.setattr("wolt_cli.commands_observability.wolt.venue_item_page", lambda _, __: item_payload)

    result = runner.invoke(
        app,
        ["item", "show", "burger-place", "item-1", "--include-upsell", "--format", "json"],
    )

    assert result.exit_code == 0
    payload = json.loads(result.output)
    assert payload["data"]["item_id"] == "item-1"
    assert payload["data"]["name"] == "Whopper Meal"
    assert len(payload["data"]["upsell_items"]) == 1


def test_discover_feed_json_returns_error_envelope(monkeypatch) -> None:
    monkeypatch.setattr("wolt_cli.commands_observability.wolt.front_page", lambda _: (_ for _ in ()).throw(WoltApiError()))

    result = runner.invoke(app, ["discover", "feed", "--lat", "50.0", "--lon", "19.0", "--format", "json"])

    assert result.exit_code == 1
    payload = json.loads(result.output)
    assert payload["data"] is None
    assert payload["error"]["code"] == "WOLT_UPSTREAM_ERROR"
    assert "wolt api" in payload["error"]["message"].lower()
