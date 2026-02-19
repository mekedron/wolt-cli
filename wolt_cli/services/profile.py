import typer
from rich import print

from wolt_cli import config
from wolt_cli.models.config import Profile


def default_profile() -> Profile | None:
    _config = config.load()
    if not (profile := next(filter(lambda p: p.is_default, _config.profiles))):
        print("[red]No default profile found")
        raise typer.Exit(1)
    return profile


def find_profile(profile_name: str | None) -> Profile:
    _config = config.load()
    if not profile_name:
        return default_profile()

    profile_name = profile_name.lower().strip()
    for profile in _config.profiles:
        if profile.name.lower() == profile_name:
            return profile

    available_profiles = ", ".join(p.name for p in _config.profiles)
    print(
        f"[red]Profile [italic cyan]{profile_name}[/][red] not found, "
        f"available profiles: [italic cyan]{available_profiles}"
    )
    raise typer.Exit(1)
