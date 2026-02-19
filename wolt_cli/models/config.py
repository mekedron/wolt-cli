from wolt_cli.models import HashableModel
from wolt_cli.models.location import Location


class Profile(HashableModel):
    name: str
    is_default: bool = False
    address: str
    location: Location


class Config(HashableModel):
    profiles: list[Profile]
