import hashlib
import lzma
import pathlib
import subprocess

blobs = [
    (b''.join(pathlib.Path('.github/pr421-review-patch.'+str(i)).read_bytes() for i in range(2)), 'acaedd644377d017fd682f42087be707c0f0f2aebd548094c7890e3a1119cad5'),
    (pathlib.Path('.github/pr421-review-extra.xz').read_bytes(), '26633a8ccfa7fe93e8833b08520f66e0f89520f3f5f5a12a6e09716b73e56996'),
    (pathlib.Path('.github/pr421-review-refund.xz').read_bytes(), 'd33b9bbdfacb476bd69ca36efa4346e35e739f2865c5034a6f95287a0705a757'),
]
for blob, digest in blobs:
    patch = lzma.decompress(blob)
    assert hashlib.sha256(patch).hexdigest() == digest
    subprocess.run(['git', 'apply', '--check', '-'], input=patch, check=True)
    subprocess.run(['git', 'apply', '-'], input=patch, check=True)
subprocess.run(['git', 'apply', '.github/pr421-review-fixture.patch'], check=True)
