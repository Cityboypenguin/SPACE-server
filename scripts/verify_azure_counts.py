#!/usr/bin/env python3
"""Compare a frozen v73 baseline to v102 after course purge; no DB writes."""
import argparse
import json
from pathlib import Path


def load(path):
    result = {}
    for line in path.read_text().splitlines():
        parts = line.split('\t')
        if parts[0] in result:
            raise ValueError('duplicate count')
        result[parts[0]] = [int(x) for x in parts[1:]]
    return result


def verify(before, after, journal):
    if before.get('schema') != [73, 0] or after.get('schema') != [102, 0]:
        raise ValueError('expected clean 73 -> 102')
    for table in ('users', 'messages', 'questions', 'answers', 'polls', 'media',
                  'notifications', 'user_reports', 'archives'):
        removed = journal['counts'].get(table, 0)
        if table not in before or table not in after or after[table] != [before[table][0] - removed]:
            raise ValueError('count mismatch: ' + table)
    if after.get('user_accounts') != before['users']:
        raise ValueError('account copy mismatch')
    for table in ('messages', 'questions', 'answers', 'polls'):
        if after.get('course_' + table) != [0]:
            raise ValueError('course content remains: ' + table)


def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument('--before', type=Path, required=True)
    p.add_argument('--after', type=Path, required=True)
    p.add_argument('--manifest', type=Path, required=True)
    args = p.parse_args()
    verify(load(args.before), load(args.after), json.loads(args.manifest.read_text()))
    print('Counts match baseline minus recorded course purge.')


if __name__ == '__main__':
    main()
