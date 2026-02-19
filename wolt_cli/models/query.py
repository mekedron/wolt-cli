from enum import Enum


class Sort(str, Enum):
    NONE = "none"
    RESTAURANT = "restaurant"
    INVERTED_RESTAURANT = "-restaurant"
    ADDRESS = "address"
    INVERTED_ADDRESS = "-address"
    DELIVERY_COST = "delivery_cost"
    INVERTED_DELIVERY_COST = "-delivery_cost"
    ESTIMATE_TIME = "estimate_time"
    INVERTED_ESTIMATE_TIME = "-estimate_time"
    RATING = "rating"
    INVERTED_RATING = "-rating"
    PRICE = "price"
    INVERTED_PRICE = "-price"

    @classmethod
    def choices(cls) -> list[str]:
        return list(cls)


class Ordering(str, Enum):
    ASC = "asc"
    DESC = "desc"

    @classmethod
    def choices(cls) -> list[str]:
        return list(cls)
