"""Capture offline OEM wire-contract examples from the reference Python SDK."""

import json
from pathlib import Path
from types import SimpleNamespace

from redfish_sdk.managers.update_strategies.registry import UpdateStrategyRegistry


class HTTP:
    def get_raw(self, _path):
        return {
            "Actions": {"#UpdateService.SimpleUpdate": {"target": "/update/action"}}
        }

    def post(self, _path, _model, *, raw_body):
        return raw_body


client = SimpleNamespace(
    _http_client=HTTP(),
    _get_update_service=lambda: SimpleNamespace(odata_id="/update"),
)
cases = []
for vendor in (
    "generic",
    "enginetech",
    "inspur",
    "zte",
    "h3c",
    "nettrix",
    "xfusion",
    "lenovo",
    "ieit",
):
    for preserve in (None, False, True):
        options = {} if preserve is None else {"preserve_config": preserve}
        expected = UpdateStrategyRegistry.get(vendor).execute(
            client,
            "https://images.invalid/fw.bin",
            "HTTPS",
            ["/components/bmc"],
            **options,
        )
        cases.append(
            {"vendor": vendor, "preserve_config": preserve, "expected": expected}
        )
root = Path(__file__).resolve().parents[2]
(root / "testdata/python_update_bodies.json").write_text(
    json.dumps(cases, indent=2) + "\n"
)
