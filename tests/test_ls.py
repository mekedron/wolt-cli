import pytest
from typer.testing import CliRunner

from wolt_cli.main import app

runner = CliRunner()


@pytest.mark.skip(reason="Not implemented yet")
def test_ls_without_config() -> None:
    result = runner.invoke(app, ["ls", "--profile", "invalid"])
    assert result.exit_code == 1
    assert result.output == "💥 Config file not found, run wolt-cli configure first\n"
