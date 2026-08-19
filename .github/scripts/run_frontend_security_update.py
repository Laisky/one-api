#!/usr/bin/env python3
"""Run the temporary generator with the latest compatible security floors.

The original generator was authored before React Router v6.30.6 was released on
2026-08-18. Keep this small override separate so the generated final snapshot
can still be assembled from a narrowly reviewed artifact and all bootstrap
files can be removed together.
"""

from __future__ import annotations

import importlib.util
from pathlib import Path

GENERATOR_PATH = Path(__file__).with_name("prepare_frontend_security_update.py")
spec = importlib.util.spec_from_file_location("frontend_security_generator", GENERATOR_PATH)
if spec is None or spec.loader is None:
    raise SystemExit(f"cannot load generator from {GENERATOR_PATH}")

generator = importlib.util.module_from_spec(spec)
spec.loader.exec_module(generator)

# React Router 6.30.5 was an incomplete publish. 6.30.6 is the complete v6
# release and pairs with @remix-run/router 1.23.4.
generator.MANIFEST_UPDATES["air"]["react-router-dom"] = "^6.30.6"
generator.MANIFEST_UPDATES["berry"]["react-router"] = "6.30.6"
generator.MANIFEST_UPDATES["berry"]["react-router-dom"] = "6.30.6"
generator.MANIFEST_UPDATES["modern"]["react-router-dom"] = "^6.30.6"
generator.MINIMUM_BY_MAJOR["@remix-run/router"] = {1: (1, 23, 4)}
generator.MINIMUM_BY_MAJOR["react-router"] = {6: (6, 30, 6)}
generator.MINIMUM_BY_MAJOR["react-router-dom"] = {6: (6, 30, 6)}

generator.TEMPORARY_PATHS.add(".github/scripts/run_frontend_security_update.py")
generator.main()
