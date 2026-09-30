"""Scoped assignment API client; no database access or authority supplied by CLI roles."""
import argparse
import getpass
import json
import os
from pathlib import Path
import stat
import sys
import urllib.error
import urllib.parse
import urllib.request
import uuid

class NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, req, fp, code, msg, headers, newurl):
        raise ValueError('API yönlendirmesi reddedildi; hedef adresi kontrol et.')

def token_file(path):
    fd = os.open(path, os.O_RDONLY | os.O_NOFOLLOW)
    with os.fdopen(fd) as f:
        s = os.fstat(f.fileno())
        if not stat.S_ISREG(s.st_mode) or s.st_mode & 0o077 or s.st_uid != os.getuid():
            raise ValueError('Anahtar dosyası sana ait, normal dosya ve 0600 izinli olmalı.')
        value = f.read(512).strip()
    if len(value) != 71 or not value.startswith('atolye_'):
        raise ValueError('Geçersiz anahtar dosyası.')
    return value

def base_url(url):
    p = urllib.parse.urlsplit(url)
    if p.username or p.password or p.query or p.fragment or p.path not in ('', '/'):
        raise ValueError('Yalnızca API sunucusunun kök adresini kullan.')
    if p.scheme != 'https' and not (p.scheme == 'http' and p.hostname in ('127.0.0.1', 'localhost', '::1')):
        raise ValueError('HTTPS gerekli; yalnızca loopback için HTTP kullanılabilir.')
    if not p.hostname:
        raise ValueError('Sunucu adresi gerekli.')
    return url.rstrip('/')

def save(path, value):
    path = Path(path)
    path.parent.mkdir(mode=0o700, parents=True, exist_ok=True)
    # Refuse symlinks and do not make an existing permissive file a secret store.
    fd = os.open(path, os.O_WRONLY | os.O_CREAT | os.O_TRUNC | os.O_NOFOLLOW, 0o600)
    with os.fdopen(fd, 'w') as f:
        os.fchmod(f.fileno(), 0o600)
        json.dump(value, f, ensure_ascii=False, indent=2)

def call(url, token, route, data=None, media='application/json', key=None):
    headers = {'Authorization': 'Bearer ' + token, 'Content-Type': media}
    if key:
        headers['Idempotency-Key'] = key
    req = urllib.request.Request(url + route, data=data, headers=headers)
    opener = urllib.request.build_opener(NoRedirect())
    try:
        with opener.open(req, timeout=30) as response:
            return json.load(response)
    except urllib.error.HTTPError as e:
        body = e.read(8192)
        try:
            message = json.loads(body).get('error', str(e.code))
        except (ValueError, AttributeError):
            message = str(e.code)
        raise ValueError(f'API {e.code}: {message}') from None

def main():
    p = argparse.ArgumentParser(description='Excel/JSON toplu görev atama API istemcisi')
    p.add_argument('command', choices=['setup', 'preview', 'commit', 'results'])
    p.add_argument('--url', default=os.environ.get('ATOLYE_API_URL', 'http://127.0.0.1:8090'))
    p.add_argument('--token-file', default=os.environ.get('ATOLYE_API_TOKEN_FILE', '.control/automation.token'))
    p.add_argument('--file')
    p.add_argument('--preview-file', default='.control/assignment-preview.json')
    args = p.parse_args()
    if args.command == 'setup':
        raw = getpass.getpass('Web ekranından aldığın erişim anahtarı (gizli): ').strip()
        if len(raw) != 71 or not raw.startswith('atolye_'):
            raise ValueError('Geçersiz erişim anahtarı.')
        path = Path(args.token_file)
        path.parent.mkdir(mode=0o700, parents=True, exist_ok=True)
        fd = os.open(path, os.O_WRONLY | os.O_CREAT | os.O_EXCL | os.O_NOFOLLOW, 0o600)
        with os.fdopen(fd, 'w') as f:
            f.write(raw)
        print('Anahtar 0600 izinli dosyaya kaydedildi.'); return
    url = base_url(args.url)
    if args.command == 'preview':
        save(args.preview_file, {'url': url, 'preview': {'ready': False}, 'request_key': ''})
    secret = token_file(args.token_file)
    if args.command == 'preview':
        if not args.file:
            p.error('--file gerekli')
        path = Path(args.file)
        if path.suffix.lower() not in ('.xlsx', '.json'):
            raise ValueError('.xlsx veya .json gerekli.')
        with path.open('rb') as source:
            payload = source.read(2 * 1024 * 1024 + 1)
        if len(payload) > 2 * 1024 * 1024:
            raise ValueError('Dosya en fazla 2 MiB olabilir.')
        media = 'application/json' if path.suffix.lower() == '.json' else 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet'
        preview = call(url, secret, '/api/automation/imports', payload, media)
        save(args.preview_file, {'url': url, 'request_key': str(uuid.uuid4()), 'preview': preview})
        print(json.dumps(preview, ensure_ascii=False, indent=2))
        return 0 if preview['ready'] else 2
    if args.command == 'commit':
        with open(args.preview_file) as source:
            envelope = json.load(source)
        if envelope['url'] != url or not envelope['preview']['ready']:
            raise ValueError('Bu sunucu için başarılı önizleme gerekli.')
        preview_id = envelope['preview']['id']
        if len(preview_id) != 64 or not all(c in '0123456789abcdef' for c in preview_id):
            raise ValueError('Geçersiz önizleme numarası.')
        result = call(url, secret, '/api/automation/imports/' + preview_id + '/commit', b'{}', key=envelope['request_key'])
    else:
        result = call(url, secret, '/api/automation/results')
    print(json.dumps(result, ensure_ascii=False, indent=2))
    return 0

if __name__ == '__main__':
    try:
        sys.exit(main() or 0)
    except (OSError, ValueError, KeyError) as e:
        print('Hata: ' + str(e), file=sys.stderr)
        sys.exit(1)
