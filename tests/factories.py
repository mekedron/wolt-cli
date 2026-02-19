from factory import Factory

from wolt_cli.models.config import Config, Profile
from wolt_cli.models.location import Location
from wolt_cli.models.wolt import Item, Link, Rating, Venue


class ConfigFactory(Factory):
    class Meta:
        model = Config

    profiles = [
        Profile(
            name="default",
            address="Test address",
            location=Location(lat=0.0, lon=0.0),
            is_default=True,
        )
    ]


class ProfileFactory(Factory):
    class Meta:
        model = Profile

    name = "default"
    address = "Test address"
    location = Location(lat=0.0, lon=0.0)
    is_default = True


class ItemFactory(Factory):
    class Meta:
        model = Item

    title = "Test item"
    track_id = "test-item"
    link = Link(target="https://wolt.com/en/restaurant/test-item")


class VenueFactory(Factory):
    class Meta:
        model = Venue

    id = "venue-id-1"
    slug = "test-venue"
    name = "Test venue"
    address = "Test address"
    badges = []
    country = "PL"
    currency = "PLN"
    delivery_price_int = 1000
    estimate_range = "10-20"
    estimate = 15.0
    online = True
    product_line = "restaurant"
    show_wolt_plus = True
    delivers = True
    short_description = "Test description"
    tags = ["test"]
    rating = Rating(rating=3, score=10)
    price_range = 2
