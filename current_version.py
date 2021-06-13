#!/usr/bin/env python

import toml

with open('pyproject.toml', 'r') as f:
    cfg = f.read()
    cfg = toml.loads(cfg)
    print(cfg['tool']['poetry']['version'])

