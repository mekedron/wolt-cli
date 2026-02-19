import re
from pathlib import Path
from typing import Final

import httpx
from pydantic import SecretStr
from pydantic_settings import BaseSettings


class Settings(BaseSettings):
    github_repository: str
    github_ref: str
    github_base_ref: str
    input_github_token: SecretStr


settings: Final[Settings] = Settings()
pyproject_toml: Final[Path] = Path("./pyproject.toml")

version_regex: Final[str] = r"\"(?P<version>[0-9]+\.[0-9]+\.[0-9]+)\""


def get_pyproject_toml_version() -> str:
    content = pyproject_toml.read_text()
    match = re.search(version_regex, content)
    if match is None:
        raise ValueError("Could not find version in pyproject.toml")
    return match.group("version")


def get_master_version() -> str:
    response = httpx.get(f"https://raw.githubusercontent.com/{settings.github_repository}/master/pyproject.toml")
    response.raise_for_status()
    content = response.text
    match = re.search(version_regex, content)
    if match is None:
        raise ValueError(f"[{settings.github_ref}] Could not find version in wolt_cli/__init__.py")
    return match.group("version")


def should_compare_versions() -> bool:
    return settings.github_base_ref == "master"


def is_updated(old: str, new: str) -> bool:
    new = new.split(".")
    old = old.split(".")
    for i in range(3):
        if int(new[i]) > int(old[i]):
            return True
        elif int(new[i]) < int(old[i]):
            return False
    return False


if __name__ == "__main__":
    if should_compare_versions():
        pyproject_toml_version = get_pyproject_toml_version()
        master_version = get_master_version()

        if not is_updated(master_version, pyproject_toml_version):
            raise ValueError(
                f"Version in pyproject.toml ({pyproject_toml_version}) "
                f"must be greater than version in master ({master_version})"
            )
