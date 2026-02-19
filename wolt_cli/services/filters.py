from wolt_cli.models.query import Ordering, Sort
from wolt_cli.models.wolt import Item
from wolt_cli.utils import cache


@cache.apply()
def filter_by_title(items: list[Item], name: str) -> list[Item]:
    name = name.lower().strip()
    return list(filter(lambda i: name in i.title.lower(), items))


@cache.apply()
def filter_by_tag(items: list[Item], tag: str) -> list[Item]:
    tag = tag.lower().strip()
    return list(filter(lambda i: i.venue and tag in i.venue.tags, items))


@cache.apply()
def sort_by(items: list[Item], sort: Sort = Sort.NONE, ordering: Ordering = Ordering.ASC) -> list[Item]:
    reverse = ordering == Ordering.DESC.value
    match sort:
        case Sort.RESTAURANT:
            return sorted(items, key=lambda i: i.title, reverse=reverse)
        case Sort.INVERTED_RESTAURANT:
            return sorted(items, key=lambda i: i.title, reverse=not reverse)
        case Sort.ADDRESS:
            return sorted(items, key=lambda i: i.venue.address, reverse=reverse)
        case Sort.INVERTED_ADDRESS:
            return sorted(items, key=lambda i: i.venue.address, reverse=not reverse)
        case Sort.DELIVERY_COST:
            return sorted(items, key=lambda i: i.venue.estimate_range, reverse=reverse)
        case Sort.INVERTED_DELIVERY_COST:
            return sorted(items, key=lambda i: i.venue.estimate_range, reverse=not reverse)
        case Sort.ESTIMATE_TIME:
            return sorted(items, key=lambda i: i.venue.delivery_price_int, reverse=reverse)
        case Sort.INVERTED_ESTIMATE_TIME:
            return sorted(items, key=lambda i: i.venue.delivery_price_int, reverse=not reverse)
        case Sort.PRICE:
            return sorted(items, key=lambda i: i.venue.price_range, reverse=reverse)
        case Sort.INVERTED_PRICE:
            return sorted(items, key=lambda i: i.venue.price_range, reverse=not reverse)
        case Sort.RATING:
            items_without_rating = list(filter(lambda i: i.venue and not i.venue.rating, items))
            items_with_rating = list(filter(lambda i: i.venue and i.venue.rating, items))
            return (
                sorted(
                    items_with_rating,
                    key=lambda i: i.venue.rating.score,
                    reverse=not reverse,
                )
                + items_without_rating
            )
        case Sort.INVERTED_RATING:
            items_without_rating = list(filter(lambda i: i.venue and not i.venue.rating, items))
            items_with_rating = list(filter(lambda i: i.venue and i.venue.rating, items))
            return (
                sorted(
                    items_with_rating,
                    key=lambda i: i.venue.rating.score,
                    reverse=reverse,
                )
                + items_without_rating
            )
        case _:
            return items


@cache.apply()
def filter_by_query(items: list[Item], query: str) -> list[Item]:
    def _show_query(s: str) -> str:
        pos = s.lower().index(query)
        return s[:pos] + f"[u][yellow]{s[pos:pos + len(query)]}[/yellow][/u]" + s[pos + len(query) :]

    query = query.lower().strip()
    results = []
    for item in items:
        if query in item.title.lower():
            item.title = _show_query(item.title)
            results.append(item)
        elif item.venue and query in item.venue.address.lower():
            item.venue.address = _show_query(item.venue.address)
            results.append(item)
        elif item.venue and item.venue.tags:
            for i, tag in enumerate(item.venue.tags):
                if query in tag.lower():
                    item.venue.tags[i] = _show_query(tag)
                    results.append(item)
                    break
    return results
