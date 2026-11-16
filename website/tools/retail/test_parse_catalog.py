#!/usr/bin/env python3
"""The retail parser's focused check — stdlib only, no download, no model.

    python3 -m unittest website/tools/retail/test_parse_catalog.py

The fixture rows are hand-written to carry one defect each: so the counts below
are a reading of the parser, not of USDA's data.
"""

from __future__ import annotations

import tempfile
import unittest
from pathlib import Path

import sys

HERE = Path(__file__).resolve().parent
sys.path.insert(0, str(HERE))

from parse_catalog import compare_releases, is_held_out, profile_release, ranked  # noqa: E402


class ProfileTest(unittest.TestCase):
    def setUp(self):
        self.earlier = profile_release(HERE / "fixtures" / "earlier.csv", keep=True)
        self.later = profile_release(HERE / "fixtures" / "later.csv", keep=True)
        self.stats = self.earlier["stats"]

    def test_missing_fields_are_counted_once_each(self):
        self.assertEqual(
            self.stats["missing"],
            {"brand": 1, "category": 1, "description": 1, "gtin": 1, "pack": 2},
        )

    def test_usable_excludes_missing_description_and_discontinued(self):
        self.assertEqual(self.stats["rows"], 8)
        self.assertEqual(self.stats["usable"], 6)
        self.assertEqual(self.stats["discontinued"], 1)
        self.assertEqual(self.stats["catalog_ready"], 4)

    def test_duplicates_are_counted_as_extra_rows(self):
        # 1001 appears twice; 1001 and 1002 share one GTIN.
        self.assertEqual(self.stats["duplicate_ids"], 1)
        self.assertEqual(self.stats["duplicate_gtins"], 2)

    def test_category_imbalance_is_measured(self):
        categories = self.stats["categories"]
        self.assertEqual(categories["distinct"], 4)
        self.assertEqual(categories["largest"], "Soda")
        self.assertEqual(categories["largest_share"], 0.5)
        self.assertEqual(categories["top5_share"], 1.0)

    def test_embedding_text_excludes_the_category(self):
        product = next(item for item in self.earlier["products"] if item.fdc_id == 1001)
        self.assertIn("Acme Cola", product.text())
        self.assertNotIn("Soda", product.text())

    def test_revision_is_not_a_new_product(self):
        delta = compare_releases(self.earlier["identity"], self.later["identity"])
        # 1001, 1002 and 1003 are in both; only 1002's description changed.
        self.assertEqual(delta["common"], 3)
        self.assertEqual(delta["revised"], 1)
        self.assertEqual(delta["unchanged"], 2)
        # 3001, 3002 and 3003 are introduced after the training release.
        self.assertEqual(delta["new"], 3)
        self.assertEqual(delta["removed"], 4)

    def test_selection_is_bounded_and_repeatable(self):
        pool = self.earlier["products"]
        first = ranked(pool, 3, "sample")
        second = ranked(pool, 3, "sample")
        self.assertEqual([p.fdc_id for p in first], [p.fdc_id for p in second])
        self.assertEqual(len(first), 3)
        smaller = ranked(pool, 2, "sample")
        self.assertEqual([p.fdc_id for p in smaller], [p.fdc_id for p in first][:2])

    def test_split_is_a_function_of_the_id(self):
        self.assertEqual(is_held_out(next(p for p in self.earlier["products"] if p.fdc_id == 1001)),
                         is_held_out(next(p for p in self.earlier["products"] if p.fdc_id == 1001)))

    def test_a_missing_required_column_fails_loudly(self):
        with tempfile.TemporaryDirectory() as tmp:
            broken = Path(tmp) / "broken.csv"
            broken.write_text("fdc_id,short_description\n1,Only two columns\n", encoding="utf-8")
            with self.assertRaisesRegex(ValueError, "gtin_upc"):
                profile_release(broken)


if __name__ == "__main__":
    unittest.main()
