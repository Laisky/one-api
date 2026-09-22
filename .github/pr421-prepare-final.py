import hashlib
import lzma
import pathlib
import subprocess

patch = lzma.decompress(pathlib.Path('.github/pr421-refund-recovery.xz').read_bytes())
assert hashlib.sha256(patch).hexdigest() == '4b88389db9d8ed3259d02957ae4ae0d4afaa57ce798a862bcec3a08c68aaeeb4'
subprocess.run(['git', 'apply', '--check', '-'], input=patch, check=True)
subprocess.run(['git', 'apply', '-'], input=patch, check=True)
path = pathlib.Path('model/cache.go')
source = path.read_text()
old = '\tquota, err := GetUserQuota(id)\n\tif err != nil {\n\t\treturn errors.Wrapf(err, "get database quota for user %d", id)\n\t}'
new = '\tvar quota int64\n\t// Keep the database read on the same bounded context as Redis. In\n\t// particular, refund recovery must stop before shutdown closes its pool.\n\terr := DB.WithContext(ctx).Model(&User{}).Where("id = ?", id).Select("quota").Find(&quota).Error\n\tif err != nil {\n\t\treturn errors.Wrapf(err, "get database quota for user %d", id)\n\t}'
assert source.count(old) == 1
path.write_text(source.replace(old, new))
