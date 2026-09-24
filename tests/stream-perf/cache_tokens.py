#!/usr/bin/env python3
"""Prepare content-verified public tokenizer assets before starting the offline workload."""
from __future__ import annotations
import argparse
import hashlib
from pathlib import Path
import tempfile
import urllib.request

ASSETS = {
    'cl100k_base': '223921b76ee99bde995b7ff738513eef100fb51d18c93597a113bcffe865b2a7',
    'o200k_base': '446a9538cb6c348e3516120d7c08b09f57c36495e2acfffe59a5bf8b0cfb1a2d',
}


def prepare(cache: Path) -> None:
    """prepare verifies cached assets or atomically downloads exactly the pinned tokenizer dictionaries."""
    cache.mkdir(parents=True, exist_ok=True)
    for name, expected in ASSETS.items():
        url = f'https://openaipublic.blob.core.windows.net/encodings/{name}.tiktoken'
        target = cache / hashlib.sha1(url.encode()).hexdigest()
        if target.is_file() and hashlib.sha256(target.read_bytes()).hexdigest() == expected:
            continue
        with urllib.request.urlopen(url, timeout=60) as response:
            data = response.read(8 << 20)
        if hashlib.sha256(data).hexdigest() != expected:
            raise ValueError(f'checksum mismatch for {name}')
        with tempfile.NamedTemporaryFile(dir=cache, delete=False) as stream:
            temporary = Path(stream.name)
            stream.write(data)
        try:
            temporary.replace(target)
        finally:
            temporary.unlink(missing_ok=True)


def main() -> None:
    """main reads the destination cache directory and prepares its verified encoding files."""
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--cache', type=Path, required=True)
    prepare(parser.parse_args().cache)


if __name__ == '__main__':
    main()
