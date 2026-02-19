import inquirer
import typer
from rich.progress import Progress, SpinnerColumn, TextColumn

from wolt_cli.gateways import wolt
from wolt_cli.models.wolt import Item, Restaurant


def select_action() -> str:
    questions = [
        inquirer.List(
            "action",
            message="What do you want to do?",
            choices=[
                "Create new configuration",
                "Add profile",
                "List profiles",
                "Edit profile",
                "Set profile as default",
                "Remove profile",
            ],
        ),
    ]

    answers = inquirer.prompt(questions) or {}
    action = answers.get("action")

    return action


def select_restaurant(items: list[Item]) -> Restaurant:
    questions = [
        inquirer.List(
            "restaurant",
            message="Several restaurants found, can you specify which one exactly?",
            choices=[item.title for item in items],
        ),
    ]
    answers = inquirer.prompt(questions) or {}
    answer = answers.get("restaurant")

    with Progress(
        SpinnerColumn(),
        TextColumn("[progress.description]{task.description}"),
        transient=True,
    ) as progress:
        progress.add_task(description="Loading...", total=None)
        return wolt.restaurant(next(item for item in items if item.title == answer))


def confirm_overwrite() -> None:
    if not inquirer.confirm("Config file already exists, overwrite?"):
        raise typer.Exit(0)


def get_profile_name(default: str = "Default") -> str:
    return inquirer.text("Profile name", default=default)


def get_address(default: str = "Kraków") -> str:
    return inquirer.text("Your address", default=default)
