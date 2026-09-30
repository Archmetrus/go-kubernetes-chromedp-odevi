import importlib.util
import os
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

spec = importlib.util.spec_from_file_location('assign', Path(__file__).with_name('atolye_assign.py'))
assign = importlib.util.module_from_spec(spec)
spec.loader.exec_module(assign)

class AssignmentClientTests(unittest.TestCase):
    def test_remote_http_and_credential_urls_rejected(self):
        for url in ['http://example.com', 'https://user:pass@example.com', 'https://example.com/path', 'https://example.com?token=secret']:
            with self.assertRaises(ValueError):
                assign.base_url(url)
        self.assertEqual(assign.base_url('http://127.0.0.1:8092/'), 'http://127.0.0.1:8092')
        self.assertEqual(assign.base_url('https://example.com'), 'https://example.com')

    def test_token_permissions_and_symlinks(self):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / 'token'
            path.write_text('atolye_' + 'a' * 64)
            path.chmod(0o644)
            with self.assertRaises(ValueError):
                assign.token_file(path)
            path.chmod(0o600)
            self.assertEqual(len(assign.token_file(path)), 71)
            link = Path(directory) / 'link'
            link.symlink_to(path)
            with self.assertRaises(OSError):
                assign.token_file(link)

    def test_redirect_never_forwards_token(self):
        with self.assertRaises(ValueError):
            assign.NoRedirect().redirect_request(None, None, 302, '', {}, 'https://elsewhere.example')

    def test_failed_preview_invalidates_old_confirmation(self):
        with tempfile.TemporaryDirectory() as directory:
            output = Path(directory) / 'preview.json'
            assign.save(output, {'preview': {'ready': True, 'id': 'old'}})
            with patch('sys.argv', ['assign', 'preview', '--file', '/missing.json', '--preview-file', str(output), '--token-file', str(Path(directory) / 'missing.token')]):
                with self.assertRaises(OSError):
                    assign.main()
            self.assertIn('false', output.read_text())
            self.assertEqual(output.stat().st_mode & 0o777, 0o600)

if __name__ == '__main__':
    unittest.main()
