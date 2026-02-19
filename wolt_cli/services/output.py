import datetime
import json
import uuid
from enum import Enum
from pathlib import Path
from typing import Any

import typer
from rich.console import Console
from rich.table import Table


class OutputFormat(str, Enum):
    TABLE = "table"
    JSON = "json"
    YAML = "yaml"


def build_envelope(
    *,
    profile: str,
    locale: str,
    data: Any,
    warnings: list[str] | None = None,
    error: dict[str, Any] | None = None,
) -> dict[str, Any]:
    envelope: dict[str, Any] = {
        "meta": {
            "request_id": f"req_{uuid.uuid4().hex}",
            "generated_at": datetime.datetime.now(datetime.UTC).replace(microsecond=0).isoformat().replace("+00:00", "Z"),
            "profile": profile,
            "locale": locale,
        },
        "data": data,
        "warnings": warnings or [],
    }
    if error is not None:
        envelope["error"] = error
    return envelope


def render_payload(payload: dict[str, Any], output_format: OutputFormat) -> str:
    if output_format == OutputFormat.JSON:
        return json.dumps(payload, ensure_ascii=False, indent=2)
    if output_format == OutputFormat.YAML:
        try:
            import yaml
        except ModuleNotFoundError as exc:
            raise RuntimeError("yaml output requires PyYAML to be installed") from exc
        return yaml.safe_dump(payload, sort_keys=False, allow_unicode=True)
    raise ValueError("render_payload() only supports json or yaml output")


def emit_machine_payload(payload: dict[str, Any], output_format: OutputFormat, output_path: Path | None = None) -> None:
    rendered = render_payload(payload, output_format)
    if output_path:
        output_path.write_text(rendered)
    typer.echo(rendered)


def emit_table(table: Table, *, no_color: bool = False, output_path: Path | None = None) -> None:
    console = Console(no_color=no_color, record=output_path is not None)
    console.print(table)
    if output_path:
        output_path.write_text(console.export_text(styles=not no_color))
