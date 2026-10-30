import importlib.util
import json
import os
from pathlib import Path
import subprocess
import tempfile
import unittest
from unittest.mock import patch

ROOT = Path(__file__).resolve().parents[1]


class DeployTests(unittest.TestCase):
    def test_success_and_rollback_preserve_secrets_and_dependencies(self):
        for fail in (False, True):
            with self.subTest(fail=fail), tempfile.TemporaryDirectory() as directory:
                d = Path(directory)
                (d / 'scripts').mkdir()
                (d / 'scripts/acr_login.sh').write_text('exit 0\n')
                old = '20260926project202609cr.azurecr.io/space-server:' + 'a' * 40
                new = '20260926project202609cr.azurecr.io/space-server:' + 'b' * 40
                (d / '.env').write_text(f'ACR_IMAGE={old}\nJWT_SECRET=test-placeholder\n')
                (d / '.env').chmod(0o600)
                commands = {
                    'flock': '#!/bin/sh\nexit 0\n',
                    'sleep': '#!/bin/sh\nexit 0\n',
                    'docker': '#!/bin/sh\nprintf "%s\\n" "$*" >> "$TRACE"\n',
                    'curl': '#!/bin/sh\nif test "$FAIL_NEW" = yes && grep -q "' + new + '" .env; then exit 1; fi\nprintf 200\n',
                }
                for name, content in commands.items():
                    (d / name).write_text(content)
                    (d / name).chmod(0o700)
                env = dict(os.environ, PATH=str(d) + ':' + os.environ['PATH'],
                           SPACE_DEPLOY_DIR=str(d), TRACE=str(d / 'trace'), FAIL_NEW='yes' if fail else 'no')
                result = subprocess.run(['bash', str(ROOT / 'scripts/deploy_azure.sh'), new],
                                        env=env, capture_output=True, text=True)
                self.assertEqual(result.returncode, 1 if fail else 0, result.stderr)
                self.assertEqual((d / '.env').read_text(),
                                 f'ACR_IMAGE={old if fail else new}\nJWT_SECRET=test-placeholder\n')
                trace = (d / 'trace').read_text()
                self.assertIn('up -d --no-deps app', trace)
                self.assertNotIn(' down', trace)
                self.assertNotIn('test-placeholder', result.stdout + result.stderr)
                self.assertEqual((d / '.env').stat().st_mode & 0o777, 0o600)


class ObjectCopyTests(unittest.TestCase):
    def test_exclusions_prevent_reupload_and_uploaded_bytes_are_verified(self):
        spec = importlib.util.spec_from_file_location('sync', ROOT / 'scripts/sync_azure_objects.py')
        module = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(module)
        for apply in (False, True):
            with self.subTest(apply=apply), tempfile.TemporaryDirectory() as directory:
                manifest = Path(directory) / 'purge.json'
                manifest.write_text(json.dumps({'account': 'test', 'objects': [{'key': 'deleted', 'container': 'test'}]}))
                calls = []

                def fake(*args):
                    calls.append(args)
                    if args[:3] == ('aws', 's3api', 'list-objects-v2'):
                        return b'{"Contents":[{"Key":"deleted"},{"Key":"keep"}]}'
                    if args[:3] == ('aws', 's3api', 'get-object'):
                        Path(args[-3]).write_bytes(b'content')
                        return b'{"ContentType":"image/png"}'
                    if args[:4] == ('az', 'storage', 'blob', 'exists'):
                        return b'false'
                    if args[:4] == ('az', 'storage', 'blob', 'download'):
                        Path(args[args.index('--file') + 1]).write_bytes(b'content')
                    return b''

                argv = ['sync', '--account', 'test', '--container', 'test', '--bucket', 'source',
                        '--exclude-manifest', str(manifest)] + (['--apply'] if apply else [])
                with patch.object(module, 'run', fake), patch('sys.argv', argv):
                    module.main()
                self.assertTrue(all('deleted' not in call for call in calls))
                self.assertEqual(any('upload' in call for call in calls), apply)
                self.assertEqual(any('download' in call for call in calls), apply)

class CountVerificationTests(unittest.TestCase):
    def test_expected_deletions_pass_and_unrelated_losses_fail(self):
        spec = importlib.util.spec_from_file_location('verify', ROOT / 'scripts/verify_azure_counts.py')
        module = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(module)
        before = {key: [100] for key in ('users', 'messages', 'questions', 'answers', 'polls',
                                        'media', 'notifications', 'user_reports', 'archives')}
        before['schema'] = [73, 0]
        after = {**before, 'schema': [102, 0], 'user_accounts': [100], 'messages': [40]}
        after.update({'course_' + key: [0] for key in ('messages', 'questions', 'answers', 'polls')})
        module.verify(before, after, {'counts': {'messages': 60}})
        after['users'] = [99]
        with self.assertRaises(ValueError):
            module.verify(before, after, {'counts': {'messages': 60}})


class MessageMediaTests(unittest.TestCase):
    def test_copy_failure_and_public_delete_retry(self):
        with tempfile.TemporaryDirectory() as directory:
            d = Path(directory)
            state = d / 'state.json'
            state.write_text(json.dumps({'key': 'media/1/test.png', 'private': False,
                                         'public': True, 'failcopy': True, 'faildelete': False}))
            ssh = """#!/usr/bin/env python3
import json, os, sys
from pathlib import Path
p=Path(os.environ['FAKE_STATE']); s=json.loads(p.read_text()); q=sys.stdin.read()
if q.startswith('SELECT DISTINCT'): print('1\t'+s['key'])
elif q.startswith('UPDATE media'): s['key']='message-media/1/test.png'
elif q.startswith('SELECT COUNT(*) FROM media m'): print(1 if s['key'].startswith('media/') else 0)
elif q.startswith('SELECT (SELECT COUNT(*)'): print(0)
else: sys.exit(2)
p.write_text(json.dumps(s))
"""
            az = """#!/usr/bin/env python3
import json, os, sys
from pathlib import Path
p=Path(os.environ['FAKE_STATE']); s=json.loads(p.read_text()); a=sys.argv[1:]
if a[:4]==['storage','blob','copy','start']:
    if s['failcopy']: sys.exit(1)
    s['private']=True
elif a[:3]==['storage','blob','exists']:
    private=a[a.index('--container-name')+1]=='space-message-media'
    print(str(s['private'] if private else s['public']).lower())
elif a[:3]==['storage','blob','delete']:
    if s['faildelete']: sys.exit(1)
    s['public']=False
else: sys.exit(2)
p.write_text(json.dumps(s))
"""
            for name, body in [('ssh', ssh), ('az', az)]:
                (d / name).write_text(body)
                (d / name).chmod(0o700)
            env = dict(os.environ, PATH=str(d)+':'+os.environ['PATH'], MODE='run', FAKE_STATE=str(state))
            command = ['bash', str(ROOT / 'scripts/migrate_message_media.sh')]
            result = subprocess.run(command, env=env, capture_output=True, text=True)
            self.assertNotEqual(result.returncode, 0)
            data = json.loads(state.read_text())
            self.assertEqual(data['key'], 'media/1/test.png')
            data.update(failcopy=False, faildelete=True)
            state.write_text(json.dumps(data))
            result = subprocess.run(command, env=env, capture_output=True, text=True)
            self.assertNotEqual(result.returncode, 0)
            data = json.loads(state.read_text())
            self.assertEqual(data['key'], 'message-media/1/test.png')
            self.assertTrue(data['public'])
            data['faildelete'] = False
            state.write_text(json.dumps(data))
            result = subprocess.run(command, env=env, capture_output=True, text=True)
            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertFalse(json.loads(state.read_text())['public'])


if __name__ == '__main__':
    unittest.main()
