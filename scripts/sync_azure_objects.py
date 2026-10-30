#!/usr/bin/env python3
"""S3 read-only export -> Blob copy. No S3 mutation; default dry run.
Requires AWS CLI, Azure CLI, authenticated operators; content never goes to stdout.
"""
import argparse
import hashlib
import json
import os
from pathlib import Path
import subprocess
import tempfile


def run(*args):
    return subprocess.check_output(args, stderr=subprocess.DEVNULL)


def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument('--bucket', required=True)
    p.add_argument('--container', required=True)
    p.add_argument('--account', required=True)
    p.add_argument('--prefix', default='')
    p.add_argument('--apply', action='store_true')
    p.add_argument('--exclude-manifest', type=Path, help='coursepurge journal; prevents reupload of deleted images')
    args = p.parse_args()
    excluded = set()
    if args.exclude_manifest:
        journal = json.loads(args.exclude_manifest.read_text())
        if journal['account'] != args.account:
            raise ValueError('journal account mismatch')
        excluded = {o['key'] for o in journal['objects'] if o['container'] == args.container}
    # CLI paginates automatically; do not use --no-paginate.
    objects = json.loads(run('aws', 's3api', 'list-objects-v2', '--bucket', args.bucket,
                             '--prefix', args.prefix, '--output', 'json')).get('Contents', [])
    moved = 0
    for obj in objects:
        key = obj['Key']
        if key in excluded:
            continue
        if not args.apply:
            moved += 1
            continue
        # Download each object and hash it: size alone cannot detect changed content.
        with tempfile.TemporaryDirectory(prefix='space-object-') as directory:
            path = Path(directory) / 'object'
            info = json.loads(run('aws', 's3api', 'get-object', '--bucket', args.bucket,
                                  '--key', key, str(path), '--output', 'json'))
            digest = hashlib.sha256(path.read_bytes()).hexdigest()
            common = ('--account-name', args.account, '--container-name', args.container,
                      '--name', key, '--auth-mode', 'login', '--only-show-errors')
            exists = run('az', 'storage', 'blob', 'exists', *common, '--query', 'exists', '-o', 'tsv').strip()
            if exists not in (b'true', b'false'):
                raise RuntimeError('invalid existence result')
            if exists == b'true':
                current = json.loads(run('az', 'storage', 'blob', 'show', *common, '-o', 'json'))
                if current.get('metadata', {}).get('migration_sha256') == digest:
                    continue
            run('az', 'storage', 'blob', 'upload', *common, '--file', str(path), '--overwrite', 'true',
                '--content-type', info.get('ContentType', 'application/octet-stream'),
                '--metadata', 'migration_sha256=' + digest, '-o', 'none')
            verify = Path(directory) / 'verify'
            run('az', 'storage', 'blob', 'download', *common, '--file', str(verify), '-o', 'none')
            if hashlib.sha256(verify.read_bytes()).hexdigest() != digest:
                raise RuntimeError('object verification failed')
            moved += 1
    print(f"{'Verified copies' if args.apply else 'Dry-run candidates'}: {moved}; excluded: {len(excluded)}")


if __name__ == '__main__':
    os.umask(0o077)
    try:
        main()
    except Exception:
        raise SystemExit('Object migration failed; check authentication and rerun. Details suppressed to protect credentials.')
