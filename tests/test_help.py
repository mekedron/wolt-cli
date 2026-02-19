from typer.testing import CliRunner

from wolt_cli.main import app

runner = CliRunner()


def test_root_help_includes_command_descriptions() -> None:
    result = runner.invoke(app, ["--help"])

    assert result.exit_code == 0
    assert "full reference:" in result.output
    assert "commands:" in result.output
    assert "Read discovery feed and browse categories." in result.output
    assert "Search venues and menu items by query." in result.output
    assert "Inspect venue details, menus, and opening hours." in result.output
    assert "discover feed" in result.output
    assert "search venues" in result.output
    assert "item show" in result.output
    assert "--include-upsell" in result.output
    assert "╭" not in result.output
    assert "╰" not in result.output
    assert "random" not in result.output


def test_random_command_is_removed() -> None:
    result = runner.invoke(app, ["random"])

    assert result.exit_code == 2
    assert "No such command 'random'" in result.output
