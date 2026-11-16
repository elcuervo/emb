#!/usr/bin/env python3
"""Read and profile the USDA branded-food releases the retail plates use.

The parser is deliberately a stream, not a dataframe: the pinned archives hold
about a million rows each, and the plates need counts, a bounded selection and
the overlap between two releases — not the whole catalog in memory. It reads
the columns the lock names, reports every missing field, and never invents a
description, identifier or category.

    from parse_catalog import profile_release, compare_releases
    earlier = profile_release("…/2026-04-30/branded_food.csv")
    later = profile_release("…/2026-10-29/branded_food.csv")
    print(earlier["stats"], compare_releases(earlier["identity"], later["identity"]))

A product's identity is its `fdc_id`. Two releases can carry the same id with a
corrected description; that is a revision, not a new product, so the comparison
counts it under `revised` and keeps it out of `new`.
"""

from __future__ import annotations

import csv
import hashlib
import heapq
import re
from collections import Counter
from dataclasses import dataclass
from pathlib import Path

REQUIRED = ("fdc_id", "short_description", "gtin_upc", "branded_food_category")
DESCRIPTION_FILE = "food.csv"
KNOWN = REQUIRED + (
    "brand_owner", "brand_name", "subbrand_name", "ingredients",
    "not_a_significant_source_of", "serving_size", "serving_size_unit",
    "household_serving_fulltext", "data_source", "package_weight",
    "modified_date", "available_date", "market_country", "discontinued_date",
    "preparation_state_code", "trade_channel",
)
DIGITS = re.compile(r"\D")


@dataclass(frozen=True)
class Product:
    fdc_id: int
    description: str
    brand: str
    gtin: str
    category: str
    pack: str
    modified: str
    available: str
    discontinued: str

    def text(self) -> str:
        """The one string embedded for this product: no category, no pack."""
        return f"{self.description} — {self.brand}" if self.brand else self.description

    def signature(self) -> str:
        """What a provider correction can change without making a new product."""
        return f"{self.description}\x1f{self.brand}"


def clean(value) -> str:
    return " ".join((value or "").split())


def gtin_digits(value: str) -> str:
    digits = DIGITS.sub("", value or "")
    return digits if 8 <= len(digits) <= 14 else ""


def pack(row: dict) -> str:
    parts = []
    weight = clean(row.get("package_weight"))
    if weight:
        parts.append(weight)
    size = clean(row.get("serving_size"))
    unit = clean(row.get("serving_size_unit"))
    if size and unit:
        parts.append(f"{size} {unit}")
    household = clean(row.get("household_serving_fulltext"))
    if household:
        parts.append(household)
    return " · ".join(parts)


def describe(fdc_id: int, short: str, descriptions: dict[int, str] | None) -> str:
    """The product name: the release's own `food.csv` description, else the branded row's.

    `short_description` is empty for most branded rows in the pinned releases;
    `food.csv` carries the catalog description ("WESSON Vegetable Oil 1 GAL").
    """
    if descriptions is not None:
        full = descriptions.get(fdc_id)
        if full:
            return full
    return short


def product(row: dict, descriptions: dict[int, str] | None = None) -> Product:
    owner = clean(row.get("brand_owner"))
    brand = owner or clean(row.get("brand_name")) or clean(row.get("subbrand_name"))
    try:
        fdc_id = int(clean(row.get("fdc_id")))
    except ValueError:
        fdc_id = 0
    return Product(
        fdc_id=fdc_id,
        description=describe(fdc_id, clean(row.get("short_description")), descriptions),
        brand=brand,
        gtin=gtin_digits(row.get("gtin_upc")),
        category=clean(row.get("branded_food_category")),
        pack=pack(row),
        modified=clean(row.get("modified_date")),
        available=clean(row.get("available_date")),
        discontinued=clean(row.get("discontinued_date")),
    )


def defects(item: Product) -> list[str]:
    missing = []
    if not item.fdc_id:
        missing.append("fdc_id")
    if not item.description:
        missing.append("description")
    if not item.brand:
        missing.append("brand")
    if not item.gtin:
        missing.append("gtin")
    if not item.category:
        missing.append("category")
    if not item.pack:
        missing.append("pack")
    return missing


def load_descriptions(path: str | Path) -> dict[int, str]:
    """`fdc_id -> description` from the release's food.csv."""
    out: dict[int, str] = {}
    with open(path, newline="", encoding="utf-8") as handle:
        reader = csv.DictReader(handle)
        if "fdc_id" not in (reader.fieldnames or []) or "description" not in (reader.fieldnames or []):
            raise ValueError(f"{path}: food.csv needs fdc_id and description")
        for row in reader:
            try:
                out[int(row["fdc_id"])] = clean(row["description"])
            except ValueError:
                continue
    return out


def iter_products(path: str | Path, descriptions: dict[int, str] | None = None):
    """Yield (Product, defects) for every data row of a branded_food.csv."""
    with open(path, newline="", encoding="utf-8") as handle:
        reader = csv.DictReader(handle)
        fields = reader.fieldnames or []
        absent = [name for name in REQUIRED if name not in fields]
        if absent:
            raise ValueError(f"{path}: missing required columns: {', '.join(absent)}")
        for row in reader:
            item = product(row, descriptions)
            yield item, defects(item)


def profile_release(path: str | Path, descriptions: dict[int, str] | None = None, keep: bool = False) -> dict:
    """Count what the release carries and return its usable products.

    `usable` means a description that can be embedded and no discontinue date.
    `catalog_ready` additionally has the structured evidence the catalog plate
    shows: a GTIN, a brand and a category.
    """
    missing = Counter()
    categories: Counter = Counter()
    duplicates: Counter = Counter()
    gtins: Counter = Counter()
    total = usable = catalog_ready = discontinued = rows_without_id = 0
    chars = 0
    longest = 0
    products: list[Product] = []
    identity: dict[int, str] = {}

    for item, flaws in iter_products(path, descriptions):
        total += 1
        for flaw in ("fdc_id", "description", "brand", "gtin", "category", "pack"):
            if flaw in flaws:
                missing[flaw] += 1
        if item.fdc_id:
            duplicates[item.fdc_id] += 1
            identity[item.fdc_id] = item.signature()
        else:
            rows_without_id += 1
        if item.gtin:
            gtins[item.gtin] += 1
        if item.discontinued:
            discontinued += 1
            continue
        if not item.description:
            continue
        usable += 1
        chars += len(item.description)
        longest = max(longest, len(item.description))
        categories[item.category or "(missing)"] += 1
        if item.gtin and item.brand and item.category:
            catalog_ready += 1
        if keep:
            products.append(item)

    counted = sum(count for count in categories.values()) or 1
    ranked = categories.most_common()
    stats = {
        "path": str(path),
        "rows": total,
        "usable": usable,
        "catalog_ready": catalog_ready,
        "discontinued": discontinued,
        "missing": dict(sorted(missing.items())),
        "duplicate_ids": sum(count - 1 for count in duplicates.values() if count > 1),
        "duplicate_gtins": sum(count - 1 for count in gtins.values() if count > 1),
        "rows_without_id": rows_without_id,
        "description_chars": {"total": chars, "mean": round(chars / usable, 2) if usable else 0, "max": longest},
        "categories": {
            "distinct": len(categories),
            "largest": ranked[0][0] if ranked else None,
            "largest_share": round(ranked[0][1] / counted, 4) if ranked else 0,
            "top5_share": round(sum(count for _, count in ranked[:5]) / counted, 4) if ranked else 0,
            "counts": dict(ranked),
        },
    }
    return {"stats": stats, "products": products if keep else None, "identity": identity}


def compare_releases(earlier: dict[int, str], later: dict[int, str]) -> dict:
    """Overlap, revisions and genuinely new ids between two releases.

    `revised` is an id present in both whose description or brand changed: USDA
    lets providers correct a description without the product becoming another
    product. `new` is the set difference, so a revision never counts as new.
    """
    common = earlier.keys() & later.keys()
    revised = sum(1 for key in common if earlier[key] != later[key])
    return {
        "earlier_ids": len(earlier),
        "later_ids": len(later),
        "common": len(common),
        "revised": revised,
        "unchanged": len(common) - revised,
        "new": len(later.keys() - earlier.keys()),
        "removed": len(earlier.keys() - later.keys()),
    }


def ranked(stream, cap: int, salt: str) -> list[Product]:
    """The `cap` products whose salted id hash sorts lowest.

    Ranking by the id's own hash, not by file order, keeps a bounded selection
    stable when a later release appends rows. The same rule and salt select the
    same products on every run.
    """
    seen: set[int] = set()

    def unique():
        for item in stream:
            if item.fdc_id in seen:
                continue
            seen.add(item.fdc_id)
            yield item

    def key(item: Product) -> tuple[str, int]:
        return hashlib.sha256(f"{salt}:{item.fdc_id}".encode()).hexdigest(), item.fdc_id

    return heapq.nsmallest(cap, unique(), key=key)


def is_held_out(item: Product) -> bool:
    """The split used by the analysis: fixed by the id, not by the sample cap."""
    digest = hashlib.sha256(f"split:{item.fdc_id}".encode()).digest()
    return digest[0] < 204  # 80 % train, 20 % held out


if __name__ == "__main__":
    import sys
    import json

    for path in sys.argv[1:]:
        result = profile_release(path)
        print(json.dumps(result["stats"], indent=2, ensure_ascii=False))
