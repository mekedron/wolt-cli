from typer.testing import CliRunner

from wolt_cli.main import app

runner = CliRunner()


def test_ls_without_config() -> None:
    result = runner.invoke(app, ["--version"])
    assert result.output == "1.1.1\n"
