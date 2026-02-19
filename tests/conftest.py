from collections.abc import Generator
from unittest import mock

import pytest

from tests.factories import ItemFactory, ProfileFactory


@pytest.fixture
def mock_find_profile() -> Generator:
    with mock.patch("wolt_cli.services.profile.find_profile") as mocked_find_profile:
        mocked_find_profile.return_value = ProfileFactory.create()
        yield mocked_find_profile


@pytest.fixture
def mock_wolt_items() -> Generator:
    with mock.patch("wolt_cli.gateways.wolt.items") as mocked_wolt_items:
        mocked_wolt_items.return_value = [ItemFactory.create() for _ in range(10)]
        yield mocked_wolt_items
