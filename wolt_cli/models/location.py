from wolt_cli.models import HashableModel


class Location(HashableModel):
    lat: float
    lon: float
