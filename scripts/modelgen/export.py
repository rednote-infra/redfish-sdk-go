"""Export Python SDK model contracts for the dependency-free Go generator.

Usage: PYTHONPATH=/path/to/redfish-python-sdk python3 scripts/modelgen/export.py
The Python SDK and pydantic are needed only when refreshing this snapshot.
"""

import enum
import importlib
import inspect
import json
import pathlib
import pkgutil
import subprocess
import types
import typing
from collections import Counter

import pydantic
import redfish_sdk.models as package

classes = set()
for module in pkgutil.iter_modules(package.__path__):
    imported = importlib.import_module(package.__name__ + "." + module.name)
    for value in vars(imported).values():
        if (
            inspect.isclass(value)
            and issubclass(value, pydantic.BaseModel)
            and value.__module__ == imported.__name__
        ):
            classes.add(value)
counts = Counter(cls.__name__ for cls in classes)
names = {
    cls: (
        "StandaloneFan"
        if cls.__module__.endswith("cooling") and cls.__name__ == "Fan"
        else cls.__name__
    )
    for cls in classes
}
acronyms = {
    x.lower(): x
    for x in (
        "ETag",
        "ID",
        "IDs",
        "URI",
        "URL",
        "UUID",
        "HTTP",
        "HTTPS",
        "IP",
        "IPv4",
        "IPv6",
        "MAC",
        "BIOS",
        "BMC",
        "OEM",
        "PCIe",
        "CXL",
        "GPU",
        "SNMP",
        "NTP",
        "DNS",
        "KVM",
        "LLDP",
        "VNC",
        "SOL",
        "FRU",
        "SKU",
        "CPU",
        "DIMM",
        "SSH",
        "IPMI",
        "RFB",
        "TCP",
        "UDP",
        "SSL",
        "TLS",
        "JSON",
        "UALink",
        "UEFI",
        "HTML5",
        "RDP",
        "KVMIP",
    )
}


def camel(name):
    return "".join(acronyms.get(part, part.capitalize()) for part in name.split("_"))


enums = {}


def go_type(annotation):
    origin = typing.get_origin(annotation)
    args = typing.get_args(annotation)
    if origin in (typing.Union, getattr(types, "UnionType", typing.Union)):
        args = tuple(arg for arg in args if arg is not type(None))
        if len(args) == 1:
            inner = go_type(args[0])
        elif set(args) == {str, int}:
            inner = "StringOrNumber"
        elif str in args and all(
            arg is str or (inspect.isclass(arg) and issubclass(arg, enum.Enum))
            for arg in args
        ):
            inner = "string"
        else:
            # Genuine protocol unions retain their complete JSON value.
            inner = "json.RawMessage"
        return (
            inner
            if inner.startswith(("*", "[]", "map[")) or inner == "json.RawMessage"
            else "*" + inner
        )
    if origin in (list, tuple, set):
        return "[]" + go_type(args[0])
    if origin is dict:
        return "map[string]" + go_type(args[1])
    if annotation is list:
        return "[]json.RawMessage"
    if annotation is dict:
        return "map[string]json.RawMessage"
    if inspect.isclass(annotation) and issubclass(annotation, dict):
        return "map[string]json.RawMessage"
    if inspect.isclass(annotation) and annotation.__name__.startswith("Collection["):
        return "Collection[Link]"
    if annotation in names:
        return names[annotation]
    if inspect.isclass(annotation) and issubclass(annotation, enum.Enum):
        enums[annotation.__name__] = {
            camel(k): v.value for k, v in annotation.__members__.items()
        }
        return annotation.__name__
    if annotation in (str, int, float, bool):
        return {str: "string", int: "int64", float: "float64", bool: "bool"}[annotation]
    if origin is typing.Literal:
        return go_type(type(args[0]))
    if annotation in (typing.Any, object, typing.TypeVar) or isinstance(
        annotation, typing.TypeVar
    ):
        return "json.RawMessage"
    raise ValueError(f"Unsupported model annotation: {annotation!r}")


for module in pkgutil.iter_modules(package.__path__):
    imported = importlib.import_module(package.__name__ + "." + module.name)
    for value in vars(imported).values():
        if (
            inspect.isclass(value)
            and issubclass(value, enum.Enum)
            and value.__module__ == imported.__name__
        ):
            enums[value.__name__] = {
                camel(k.lower()): v.value for k, v in value.__members__.items()
            }
models = []
for cls in sorted(classes, key=lambda c: names[c]):
    if cls.__name__ in ("Collection", "Oem", "IdentifiedEntity"):
        continue
    cls.model_rebuild()
    fields = []
    for name, field in cls.model_fields.items():
        alias = field.alias or name
        if isinstance(alias, pydantic.AliasChoices):
            alias = alias.choices[0]
        if field.exclude is True:
            alias = "-"
        fields.append(
            {
                "name": camel(name).replace("Odata", "OData"),
                "type": (
                    "OneOrMany[Message]"
                    if cls.__name__ == "Task" and name == "messages"
                    else "*Percentage"
                    if cls.__name__ == "Task" and name == "percent_complete"
                    else go_type(field.annotation)
                ),
                "json": alias,
                "required": field.is_required(),
                "sensitive": not field.repr,
                "aliases": [
                    choice
                    for choice in getattr(field.validation_alias, "choices", [])
                    if isinstance(choice, str) and choice != alias
                ],
            }
        )
    if cls.__name__ == "Fan" and cls.__module__.endswith("thermal"):
        # The Go convenience getter retains modern fan precision in its unified view.
        fields.append(
            {
                "name": "SpeedPercent",
                "type": "*FanSpeedExcerpt",
                "json": "SpeedPercent",
                "required": False,
                "sensitive": False,
                "aliases": [],
            }
        )
    models.append(
        {
            "name": names[cls],
            "fields": fields,
            "root": getattr(cls, "__pydantic_root_model__", False),
        }
    )
root = pathlib.Path(__file__).resolve().parents[2]
source = pathlib.Path(package.__file__).resolve().parents[2]
commit = subprocess.check_output(
    ["git", "-C", str(source), "rev-parse", "HEAD"], text=True
).strip()
(root / "scripts/modelgen/schema.json").write_text(
    json.dumps(
        {
            "source": "rednote-infra/redfish-python-sdk",
            "commit": commit,
            "models": models,
            "enums": enums,
        },
        indent=2,
    )
    + "\n"
)
print(f"Exported {len(models)} models and {len(enums)} enums from {commit}")
