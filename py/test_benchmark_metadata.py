import unittest
from types import SimpleNamespace
from unittest.mock import patch
from importlib import metadata

from benchmark_metadata import package_version


class MetadataTests(unittest.TestCase):
    def test_module_version(self):
        self.assertEqual(package_version(SimpleNamespace(__version__='9.2'), 'example'), '9.2')

    def test_distribution_version(self):
        with patch('benchmark_metadata.metadata.version', return_value='8.1'):
            self.assertEqual(package_version(SimpleNamespace(), 'example'), '8.1')

    def test_missing_version_is_not_guessed(self):
        with patch('benchmark_metadata.metadata.version', side_effect=metadata.PackageNotFoundError):
            self.assertEqual(package_version(SimpleNamespace(), 'example'), 'unknown')
