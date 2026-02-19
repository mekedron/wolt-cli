from collections.abc import Callable
from enum import Enum
from typing import Final, TypeAlias

from wolt_cli.models.wolt import Item

Evaluator: TypeAlias = Callable[[Item], float]

default_weight: Final[float] = 20.0
min_weight: Final[float] = 10.0
max_weight: Final[float] = 100.0


class EvaluateTechnique(str, Enum):
    DELIVERY_PRICE = "delivery_price"
    INVERTED_DELIVERY_PRICE = "-delivery_price"
    RATING = "rating"
    INVERTED_RATING = "-rating"
    MIX = "mix"
    INVERTED_MIX = "-mix"
    RANDOM = "random"

    @classmethod
    def choices(cls) -> list[str]:
        return list(cls)


def by_delivery_price(item: Item) -> float:
    if not (venue := item.venue):
        return default_weight

    if not venue.delivers:
        return min_weight

    rating = max_weight - venue.delivery_price_int / 10 + 30

    return max((min((rating, max_weight)), min_weight))


def by_rating(item: Item) -> float:
    if not (venue := item.venue):
        return default_weight

    if not venue.rating:
        return min_weight

    rating = (venue.rating.score - 4) * 15 + 30

    return max((min((rating, max_weight)), min_weight))


def by_mix(item: Item) -> float:
    return max((by_delivery_price(item), by_rating(item)))


def by_random(_: Item) -> float:
    return default_weight


def by_inverted_mix(item: Item) -> float:
    return max_weight - by_mix(item)


def by_inverted_delivery_price(item: Item) -> float:
    return max_weight - by_delivery_price(item)


def by_inverted_rating(item: Item) -> float:
    return max_weight - by_rating(item)


evaluators: dict[EvaluateTechnique, Evaluator] = {
    EvaluateTechnique.DELIVERY_PRICE: by_delivery_price,
    EvaluateTechnique.INVERTED_DELIVERY_PRICE: by_inverted_delivery_price,
    EvaluateTechnique.RATING: by_rating,
    EvaluateTechnique.INVERTED_RATING: by_inverted_rating,
    EvaluateTechnique.MIX: by_mix,
    EvaluateTechnique.INVERTED_MIX: by_inverted_mix,
    EvaluateTechnique.RANDOM: by_random,
}


def build_weights(items: list[Item], evaluator: EvaluateTechnique = EvaluateTechnique.MIX) -> list[float]:
    return [evaluators.get(evaluator, by_mix)(item) for item in items]
