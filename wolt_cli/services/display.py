from rich.table import Table

from wolt_cli.models.wolt import Item, Restaurant


def build_items_table(items: list[Item]) -> Table:
    table = Table()

    table.add_column("No.", justify="left", style="white", no_wrap=False)
    table.add_column("Restaurant", justify="right", style="cyan")
    table.add_column("Address", justify="right", style="magenta")
    table.add_column("Estimate time", justify="right", style="green")
    table.add_column("Delivery cost", justify="right", style="green")
    table.add_column("Rating", justify="right", style="green")
    table.add_column("Price", justify="right", style="green")
    table.add_column("Tags", justify="right", style="yellow")

    num = 1
    for item in items:
        if venue := item.venue:
            table.add_row(
                str(num),
                item.format_title(),
                venue.format_address(),
                venue.format_estimate_range(),
                venue.format_delivery_price(),
                venue.format_rating(),
                venue.format_price_range(),
                venue.format_tags(),
            )
            num += 1

    return table


def build_restaurant_table(restaurant: Restaurant) -> Table:
    table = Table()

    table.add_column(restaurant.format_name(), justify="right", style="cyan")
    table.add_column(restaurant.format_address(), justify="right", style="cyan")

    table.add_row("Rating", restaurant.format_rating())
    table.add_row("Price", restaurant.format_price_range())
    table.add_row("Opening time", restaurant.format_opening_time())
    table.add_row("Delivery time", restaurant.format_delivery_time())
    table.add_row("Delivery methods", restaurant.format_delivery_methods())
    table.add_row("Website", restaurant.format_public_url())
    table.add_row("Phone", restaurant.format_phone())
    table.add_row("Payment methods", restaurant.format_allowed_payment_methods())
    table.add_row("Description", restaurant.format_description())
    table.add_row("Tags", restaurant.format_tags())

    return table
