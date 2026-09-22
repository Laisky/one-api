import hashlib, lzma, pathlib, subprocess
root = pathlib.Path('.github')
for prefix, digest in [('core', 'f7428fc50f92e8c902f65f3a8494c9b82150e4046fb8b712c519923bc7196233'), ('media', '909a43ff48a25319d1f8d1a949fe93d3995499405b55e04a21499a7df8176c29'), ('continuation', '72062ca56114c326a5666b3767d5d31b403c1835c8332f74677c556256c54e01'), ('ui', 'be0bff958d9bb5b62c6b65f49cb9cddaa9c05ac7b141a0a499b2f20bebc1895b'), ('final', '7b6ad23d32a0d2dcfd0f659cb131449da7ec87b46ac27ea4aaec127eeda784da'), ('compat', 'ea182a25783dc34717a59b84ff0b222b2d62552bfb536a538c1c278962c9f6d5')]:
    if prefix == 'compat':
        patch = (root/'protocol-compat.patch').read_bytes()
    else:
        paths = [root / ('protocol-' + prefix + '.xz')] if prefix == 'continuation' else sorted(root.glob('protocol-' + prefix + '.[0-3]'))
        patch = lzma.decompress(b''.join(p.read_bytes() for p in paths))
    assert hashlib.sha256(patch).hexdigest() == digest, prefix
    subprocess.run(['git', 'apply', '--check', '-'], input=patch, check=True)
    subprocess.run(['git', 'apply', '-'], input=patch, check=True)
