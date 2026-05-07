import unittest

from flatten import flatten


class TestFlatten(unittest.TestCase):
    def test_empty(self):
        self.assertEqual(flatten([]), [])

    def test_already_flat(self):
        self.assertEqual(flatten([1, 2, 3]), [1, 2, 3])

    def test_one_level(self):
        self.assertEqual(flatten([[1, 2], [3]]), [1, 2, 3])

    def test_deeply_nested(self):
        self.assertEqual(flatten([1, [2, [3, [4]]]]), [1, 2, 3, 4])

    def test_mixed_types(self):
        self.assertEqual(flatten([1, ["a", [True, None]], 2]), [1, "a", True, None, 2])

    def test_strings_atomic(self):
        # Strings should not be iterated character by character.
        self.assertEqual(flatten(["abc", ["de"]]), ["abc", "de"])


if __name__ == "__main__":
    unittest.main()
