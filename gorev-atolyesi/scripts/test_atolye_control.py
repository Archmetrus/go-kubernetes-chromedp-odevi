import importlib.util
import json
import os
from pathlib import Path
import subprocess
import socket
import tempfile
import unittest
from unittest.mock import patch

spec = importlib.util.spec_from_file_location('control', Path(__file__).with_name('atolye_control.py'))
control = importlib.util.module_from_spec(spec)
spec.loader.exec_module(control)


class ControlTests(unittest.TestCase):
    def test_demo_cannot_control_kind(self):
        with patch.dict(os.environ, {'ATOLYE_LOCAL_ONLY': 'true'}):
            with patch.object(control, 'run') as run:
                with self.assertRaisesRegex(RuntimeError, 'kendi yerel'):
                    control.action('down', 'kind')
                run.assert_not_called()

    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.patch = patch.object(control, 'STATE', Path(self.temp.name))
        self.patch.start()

    def tearDown(self):
        self.patch.stop()
        self.temp.cleanup()

    def test_unmanaged_process_is_never_signalled(self):
        child = subprocess.Popen(['sleep', '30'])
        try:
            record = {'pid': child.pid, 'identity': {'start': 'wrong'}}
            (control.STATE / 'local.json').write_text(json.dumps(record))
            self.assertIsNone(control.managed())
            self.assertIn('dokunulmadı', control.stop_local())
            self.assertIsNone(child.poll())
        finally:
            child.terminate()
            child.wait()

    def test_tracked_process_stops(self):
        child = subprocess.Popen(['sleep', '30'])
        try:
            record = {'pid': child.pid, 'identity': control.identity(child.pid)}
            (control.STATE / 'local.json').write_text(json.dumps(record))
            self.assertIn('durduruldu', control.stop_local())
            child.wait(timeout=2)
        finally:
            if child.poll() is None:
                child.terminate()
                child.wait()

    def test_mutual_exclusion(self):
        with control.locked('local'):
            with self.assertRaisesRegex(RuntimeError, 'başka bir işlem'):
                with control.locked('local'):
                    pass

    def test_terminal_and_secret_sanitizing(self):
        output = control.clean('\x1b]52;c;clipboard\x07\x1b[31mAuthorization: Bearer abc123\npostgres://u:pass@host\u202e')
        for forbidden in ('\x1b', 'clipboard', 'abc123', 'u:pass', '\u202e'):
            self.assertNotIn(forbidden, output)

    def test_account_mode_environment_persists_without_passwords(self):
        (control.STATE / 'accounts-enabled').write_text('enabled\n')
        env = control.local_env()
        self.assertEqual(env['AUTH_ENABLED'], 'true')
        self.assertEqual(env['ACCOUNT_APPROVAL'], 'true')
        self.assertEqual(env['ALLOW_SIGNUP'], 'true')

    def test_development_mailbox_is_explicit_and_copyable(self):
        code = 'a' * 32
        result = control.mailbox_text(json.dumps([{'email': 'demo@example.test', 'code': code, 'expires_at': 2000000000}]))
        self.assertIn('gerçek e-posta gönderilmedi', result)
        self.assertIn('\n' + code + '\n', result)
        self.assertIn('Geçerli ileti yok', control.mailbox_text('[]'))

    def test_bootstrap_uses_fixed_command_and_email_environment(self):
        with patch.object(control, 'run', return_value='ok') as run:
            email = 'name+test@example.test'
            control.admin_command('admin-bootstrap', email)
            self.assertEqual(run.call_args.args[0], ['go', 'run', './cmd/gorev-atolyesi', 'admin-bootstrap'])
            self.assertEqual(run.call_args.kwargs['env']['USER_EMAIL'], email)

    def test_tui_assign_uses_api_client_without_shell_or_database(self):
        with patch.object(control, 'run', return_value='{"ready":true}') as run:
            path = '/tmp/tasks $(not-a-command).json'
            control.action('assignment-preview:' + path, 'local')
            args = run.call_args.args[0]
            self.assertEqual(args[:4], ['python3', 'scripts/atolye_assign.py', 'preview', '--file'])
            self.assertEqual(args[4], path)
            self.assertIn(str(control.STATE / 'assignment-preview.json'), args)

    def test_port_check_rejects_active_listener(self):
        with socket.create_server(('127.0.0.1', 0)) as server:
            address = server.getsockname()
            with self.assertRaisesRegex(RuntimeError, 'başka bir süreç'):
                control.check_port(*address)
            # The original listener remains usable.
            with socket.create_connection(address):
                connection, _ = server.accept()
                connection.close()

    def test_port_check_allows_time_wait(self):
        with socket.create_server(('127.0.0.1', 0)) as server:
            address = server.getsockname()
            with socket.create_connection(address) as client:
                connection, _ = server.accept()
                # Server closes first, leaving TIME_WAIT on its listening port.
                connection.shutdown(socket.SHUT_WR)
                self.assertEqual(client.recv(1), b'')
                connection.close()
        control.check_port(*address)
        with socket.create_server(address):
            pass


if __name__ == '__main__':
    unittest.main()
