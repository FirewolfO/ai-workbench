#!/usr/bin/env python3
"""Convert a local PDF to DOCX. Inputs are supplied by the whitelisted Go file-tools service."""

import sys

from pdf2docx import Converter


def main() -> int:
    if len(sys.argv) != 3:
        return 2
    converter = Converter(sys.argv[1])
    try:
        converter.convert(sys.argv[2], start=0, end=None)
    finally:
        converter.close()
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
