import datetime
from enum import Enum
from typing import TypeAlias

from pydantic import AnyUrl, HttpUrl

from wolt_cli.models import HashableModel


class Rating(HashableModel):
    rating: float
    score: float


class RatingDetail(HashableModel):
    negative_percentage: int
    neutral_percentage: int
    positive_percentage: int
    rating: int
    score: float
    text: str
    volume: int


class Badge(HashableModel):
    text: str
    variant: str


class Venue(HashableModel):
    id: str | dict[str, str] | None = None
    slug: str | None = None
    name: str
    address: str
    badges: list[Badge]
    country: str
    currency: str
    delivers: bool
    delivery_price_int: int | None = None
    estimate_range: str | None = None
    estimate: float
    online: bool | None = None
    product_line: str | None = None
    show_wolt_plus: bool = False
    # short_description: str | None = None
    tags: list[str]
    rating: Rating | None = None
    price_range: int

    def format_badges(self) -> str:
        if not self.badges:
            return ""
        return ", ".join(badge.text for badge in self.badges)

    def format_tags(self) -> str:
        return ", ".join(map(str.capitalize, self.tags))

    def format_address(self) -> str:
        return self.address

    def format_estimate_range(self) -> str:
        if not self.estimate_range:
            return "-"
        return self.estimate_range.replace("-", " - ") + " min"

    def format_delivery_price(self) -> str:
        if self.delivery_price_int is None:
            return "-"
        if not self.delivers:
            return "[green](No delivery)[/green]"
        return f"{self.delivery_price_int / 100:.2f} {self.currency}"

    def format_rating(self) -> str:
        if not self.rating:
            return "[green](No rating)[/green]"
        return str(self.rating.score)

    def format_price_range(self) -> str:
        if not self.price_range:
            return "-"
        return ":money_bag:" * self.price_range


class Link(HashableModel):
    target: str


class Item(HashableModel):
    title: str
    track_id: str
    link: Link
    venue: Venue | None = None

    def format_title(self) -> str:
        if self.venue is None:
            return self.title
        if not self.venue.badges:
            return self.title
        return f"{self.title} [green italic]({self.venue.format_badges()})[/green italic]"


class Section(HashableModel):
    name: str
    title: str | None = None
    items: list[Item] = []


class Translation(HashableModel):
    lang: str
    value: str


Weekday: TypeAlias = str


class TimesType(str, Enum):
    OPEN = "open"
    CLOSE = "close"


class Times(HashableModel):
    type: str
    value: dict[str, int]

    def format(self) -> str:
        if not (value := self.value.get("$date")):
            return "-"
        return datetime.datetime.fromtimestamp(value / 1000).strftime("%H:%M")


class Statistics(HashableModel):
    mean: int | None = None
    max: int | None = None
    min: int | None = None


class Estimates(HashableModel):
    delivery: Statistics
    pickup: Statistics
    preparation: Statistics
    total: Statistics

    def format(self) -> str:
        if not self.total.mean:
            return "-"
        return f"{self.total.mean} minutes"


class Restaurant(HashableModel):
    id: str | dict[str, str] | None = None
    slug: str | None = None
    name: list[Translation]
    address: str
    city: str
    country: str
    currency: str
    food_tags: list[str]
    phone: str | None = None
    price_range: int
    public_url: HttpUrl | AnyUrl
    rating: RatingDetail | None = None
    website: HttpUrl | AnyUrl | None = None
    allowed_payment_methods: list[str]
    description: list[Translation]
    short_description: list[Translation] = []
    estimates: Estimates | None = None
    opening_times: dict[Weekday, list[Times]] = {}
    delivery_methods: list[str] = []
    timezone_name: str | None = None

    def format_delivery_time(self) -> str:
        if not self.estimates:
            return "-"
        return self.estimates.format()

    def format_description(self) -> str:
        if not self.short_description:
            return "-"

        description = self.short_description[0].value

        if len(description) > 60:
            description = description[:60] + "..."

        return description

    def format_opening_time(self) -> str:
        weekday = datetime.datetime.now().strftime("%A").lower()
        opening_times = self.opening_times.get(weekday)

        if not opening_times:
            return "-"

        open_time = next(time for time in opening_times if time.type == TimesType.OPEN)
        close_time = next(time for time in opening_times if time.type == TimesType.CLOSE)

        return f"{open_time.format()} - {close_time.format()}"

    def format_name(self) -> str:
        return f":pizza: [u]{self.name[0].value}[/u]"

    def format_address(self) -> str:
        return f"[u]{self.city}, {self.address}[/u] :pizza:"

    def format_price_range(self) -> str:
        if not self.price_range:
            return "-"
        return ":money_bag:" * self.price_range

    def format_phone(self) -> str:
        if not self.phone:
            return "-"
        return f"{self.phone[:3]} {self.phone[3:]}"

    def format_public_url(self) -> str:
        return str(self.public_url)

    def format_rating(self) -> str:
        if not self.rating:
            return "[green](No rating)[/green]"
        return f"{self.rating.text} ({self.rating.score} / {self.rating.volume} reviews)"

    def format_tags(self) -> str:
        if not self.food_tags:
            return "-"
        return ", ".join(f"[black on yellow]{tag.capitalize()}[/]" for tag in self.food_tags)

    def format_allowed_payment_methods(self) -> str:
        return ", ".join(map(str.capitalize, self.allowed_payment_methods))

    def format_delivery_methods(self) -> str:
        if not self.delivery_methods:
            return "[green](No delivery)[/green]"
        return ", ".join(f"[white on green]{tag.capitalize()}[/]" for tag in self.delivery_methods)
