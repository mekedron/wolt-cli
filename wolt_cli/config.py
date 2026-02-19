from pathlib import Path
from typing import Final

import inquirer
import typer
from pydantic import ValidationError
from rich import print

from wolt_cli.gateways import location
from wolt_cli.models.config import Config, Profile
from wolt_cli.models.location import Location
from wolt_cli.utils.prompt import confirm_overwrite, get_address, get_profile_name, select_action

default_app_dir: Final[Path] = Path.home() / ".wolt-cli"
config_file: Final[Path] = default_app_dir / ".wolt-cli-config.json"


def load() -> Config:
    if not config_file.is_file():
        print("[red]💥 Config file not found, run [italic]wolt-cli configure[/italic] first")
        raise typer.Exit(1)
    try:
        config = Config.model_validate_json(config_file.read_text())
    except ValidationError as e:
        print(f"[red]💥 Config file is invalid: {e}")
        raise typer.Exit(1)

    return config


def manage() -> None:
    match select_action():
        case "Create new configuration":
            _create()
        case "Add profile":
            _add()
        case "List profiles":
            _list()
        case "Edit profile":
            _edit()
        case "Set profile as default":
            _set_default()
        case "Remove profile":
            _remove()


def _create() -> Config:
    if config_file.is_file():
        confirm_overwrite()

    profile_name = get_profile_name()
    address = get_address()
    detailed_location = _get_detailed_location(address)

    config = Config(
        profiles=[
            Profile(
                name=profile_name,
                is_default=True,
                address=address,
                location=detailed_location,
            )
        ]
    )

    if not default_app_dir.is_dir():
        default_app_dir.mkdir()

    config_file.write_bytes(config.model_dump_json().encode())

    print("[green]🏁 Config was created successfully!")
    return config


def _add() -> Config:
    config = load()

    profile_name = get_profile_name()
    if profile_name in (p.name for p in config.profiles):
        print("[red]💥 Profile with this name already exists")
        raise typer.Exit(1)

    address = get_address()
    detailed_location = _get_detailed_location(address)

    config.profiles.append(Profile(name=profile_name, address=address, location=detailed_location))

    config_file.write_bytes(config.model_dump_json().encode())

    print("[green]🏁 Profile was added successfully!")
    return config


def _edit() -> Config:
    config = load()

    profile = inquirer.list_input(
        "Which profile do you want to edit?",
        choices=[p.name for p in config.profiles],
    )

    profile = next(filter(lambda p: p.name == profile, config.profiles))

    profile.name = get_profile_name(profile.name)
    profile.address = get_address(profile.address)
    profile.location = _get_detailed_location(profile.address)

    config_file.write_bytes(config.model_dump_json().encode())

    print("[green]🏁 Profile was updated successfully!")
    return config


def _list() -> None:
    config = load()
    for profile in config.profiles:
        print(profile)


def _set_default() -> Config:
    config = load()

    profile = inquirer.list_input(
        "Which profile do you want to set as default?",
        choices=[p.name for p in config.profiles],
    )

    for p in config.profiles:
        p.is_default = p.name == profile

    config_file.write_bytes(config.model_dump_json().encode())

    print("[green]🏁 Profile was updated successfully!")
    return config


def _remove() -> Config:
    config = load()

    profile = inquirer.list_input(
        "Which profile do you want to remove?",
        choices=[p.name for p in config.profiles],
    )

    config.profiles = list(filter(lambda p: p.name != profile, config.profiles))

    config_file.write_bytes(config.model_dump_json().encode())

    print("[green]🏁 Profile was updated successfully!")
    return config


def _get_detailed_location(address: str) -> Location:
    return location.get(address)
