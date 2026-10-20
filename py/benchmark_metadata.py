"""Metadata helpers that do not import optional benchmark dependencies."""
from importlib import metadata


def package_version(module, distribution):
    version = getattr(module, '__version__', None)
    if version:
        return str(version)
    try:
        return metadata.version(distribution)
    except metadata.PackageNotFoundError:
        return 'unknown'
