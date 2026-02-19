import typer


def validate_query(ctx: typer.Context, query: str | None) -> str | None:
    if not query:
        return query

    if "tag" in ctx.params and ctx.params["tag"]:
        raise typer.BadParameter("You can't use --query and --tag at the same time")

    return query


def validate_restaurant(ctx: typer.Context, restaurant: str | None) -> str | None:
    if not restaurant:
        return restaurant

    if any(ctx.params):
        raise typer.BadParameter("You can't use any other option with RESTAURANT argument")

    return restaurant
