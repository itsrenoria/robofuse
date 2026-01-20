#!/usr/bin/env python3
"""Main entry point for the robofuse application."""

import sys
from robofuse.cli.commands import cli


def main():
    """Entry point for the application."""
    # Pass an empty list as object to the CLI context
    cli(obj={})


if __name__ == "__main__":
    sys.exit(main())