import json
from pathlib import Path
from unittest.mock import MagicMock, patch

import pytest
from httpx import Response

from tests.factories import ItemFactory
from wolt_cli.gateways import wolt
from wolt_cli.models.location import Location
from wolt_cli.models.wolt import Item, Translation


def read_data(filename: str) -> dict:
	path = Path(__file__).parent / 'fixtures' / filename
	return json.loads(path.read_text())


def create_success_response(json_data: dict) -> Response:
	return Response(
		status_code=200,
		json=json_data,
	)


@pytest.fixture
def mock__httpx__get():
	with patch("wolt_cli.gateways.wolt.httpx.get") as mock:
		yield mock


def test__sections__with__success_response(mock__httpx__get: MagicMock) -> None:
	sections_json = read_data("wolt/sections.json")
	mock__httpx__get.return_value = create_success_response(sections_json)

	location = Location(lat=10, lon=10)
	items = wolt.items(location)

	assert len(items) == 30


def test__restaurant__with__success_response(mock__httpx__get: MagicMock) -> None:
	restaurants_json = read_data("wolt/restaurants.json")
	mock__httpx__get.return_value = create_success_response(restaurants_json)

	item = ItemFactory.create()
	restaurant = wolt.restaurant(item)

	assert restaurant.name[0] == Translation(lang="en", value="N'Pizza ")
	assert restaurant.city == "Kraków"
	assert restaurant.country == "POL"
	assert restaurant.currency == "PLN"
