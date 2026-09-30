"""Linux local operator CLI/TUI. No HTTP listener, shell evaluation or sudo."""
import argparse
import concurrent.futures
import contextlib
import curses
import errno
import fcntl
import json
import os
from pathlib import Path
import re
import signal
import socket
import subprocess
import sys
import time
import unicodedata

ROOT = Path(__file__).resolve().parents[1]
STATE = Path(os.environ.get('ATOLYE_CONTROL_DIR', ROOT / '.control'))


def clean(value):
    value = re.sub(r'\x1b\][^\x07\x1b]*(?:\x07|\x1b\\)', '', str(value))
    value = re.sub(r'\x1b\[[0-?]*[ -/]*[@-~]', '', value)
    value = ''.join(c for c in value if c == '\n' or not unicodedata.category(c).startswith('C'))
    value = re.sub(r'(?i)(bearer\s+)\S+', r'\1***', value)
    value = re.sub(r'(?i)((?:password|token|secret|authorization|database_url)["\s]*[:=]\s*)[^\s,]+', r'\1***', value)
    value = re.sub(r'://[^\s/@]+:[^\s/@]+@', '://***@', value)
    for key, secret in os.environ.items():
        if len(secret) >= 4 and any(word in key.upper() for word in ('TOKEN', 'PASSWORD', 'SECRET', 'DATABASE_URL')):
            value = value.replace(secret, '***')
    return value


def run(args, timeout=30, env=None):
    result = subprocess.run(args, cwd=ROOT, stdin=subprocess.DEVNULL,
                            stdout=subprocess.PIPE, stderr=subprocess.STDOUT, timeout=timeout, env=env)
    output = clean(result.stdout.decode(errors='replace'))
    if result.returncode:
        raise RuntimeError(output[-6000:] or f'İşlem başarısız: {args[0]}')
    return output


@contextlib.contextmanager
def locked(profile):
    STATE.mkdir(mode=0o700, exist_ok=True)
    with (STATE / (profile + '.lock')).open('a') as lock:
        try:
            fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
        except BlockingIOError:
            raise RuntimeError('Bu profilde başka bir işlem sürüyor.')
        yield


def identity(pid):
    try:
        proc = Path('/proc') / str(pid)
        fields = (proc / 'stat').read_text().rsplit(')', 1)[1].split()
        if fields[0] == 'Z':
            return None
        return {'start': fields[19], 'exe': str((proc / 'exe').resolve(strict=True)),
                'uid': proc.stat().st_uid}
    except (OSError, ValueError):
        return None


def managed():
    try:
        record = json.loads((STATE / 'local.json').read_text())
        if record['identity'] == identity(record['pid']):
            return record
    except (OSError, ValueError, KeyError):
        pass
    return None


def kube(*args):
    return ['kubectl', '--kubeconfig', str(ROOT / '.kubeconfig'), '--context', 'kind-go-odev',
            '--request-timeout=10s', '-n', 'gorev-atolyesi', *args]


def status(profile):
    if profile == 'local':
        record = managed()
        return {'profile': profile, 'state': 'Çalışıyor' if record else 'Yönetilen süreç yok',
                'pid': record['pid'] if record else None,
                'url': record['url'] if record else None}
    data = json.loads(run(kube('get', 'deployment', 'gorev-atolyesi', '-o', 'json')))
    return {'profile': profile, 'state': 'Çalışıyor' if data.get('status', {}).get('readyReplicas', 0) else 'Hazır pod yok',
            'desired': data['spec'].get('replicas', 0), 'ready': data.get('status', {}).get('readyReplicas', 0)}


def check_port(host, port):
    # Match Go's listener reuse semantics: TIME_WAIT is not a live server.
    # Never use SO_REUSEPORT, which could allow sharing an active listener.
    with socket.socket() as sock:
        sock.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
        try:
            sock.bind((host, port))
            sock.listen(1)
        except OSError as exc:
            if exc.errno == errno.EADDRINUSE:
                raise RuntimeError(f'{host}:{port} başka bir süreç tarafından kullanılıyor. '
                                   'O süreci kendi terminalinden durdurun veya ADDR ile farklı port seçin.') from exc
            raise


def start_local():
    if managed():
        return 'Yerel sunucu zaten çalışıyor.'
    addr = os.environ.get('ADDR', '127.0.0.1:8090')
    host, port = addr.rsplit(':', 1)
    if host != '127.0.0.1' or not 1 <= int(port) <= 65535:
        raise RuntimeError('Yerel kontrol için ADDR=127.0.0.1:<port> kullanın.')
    check_port(host, int(port))
    binary = STATE / 'server'
    run(['go', 'build', '-o', str(binary), './cmd/gorev-atolyesi'], timeout=180)
    with (STATE / 'local.log').open('ab') as log:
        child = subprocess.Popen([str(binary)], cwd=ROOT, stdin=subprocess.DEVNULL,
                                 stdout=log, stderr=log, start_new_session=True, env=local_env())
    record = {'pid': child.pid, 'identity': identity(child.pid), 'url': 'http://' + addr}
    (STATE / 'local.json').write_text(json.dumps(record))
    for _ in range(100):
        if child.poll() is not None:
            raise RuntimeError('Sunucu başlatılamadı. Logları görüntüleyin.\n' + logs('local'))
        try:
            with socket.create_connection((host, int(port)), timeout=0.1):
                return 'Yerel sunucu hazır: ' + record['url']
        except OSError:
            time.sleep(0.1)
    # If initialization takes longer, preserve the tracked process for explicit stop.
    raise RuntimeError('Süreç başlatıldı fakat bağlantı henüz hazır değil. Durumu/logları kontrol edin.')


def stop_local():
    record = managed()
    if not record:
        return 'Yönetilen yerel süreç yok; başka süreçlere dokunulmadı.'
    # pidfd binds the signal to this process even if its PID is later reused.
    fd = os.pidfd_open(record['pid'])
    try:
        if identity(record['pid']) != record['identity']:
            raise RuntimeError('Süreç kimliği değişti; kapatma iptal edildi.')
        signal.pidfd_send_signal(fd, signal.SIGTERM)
        for _ in range(150):
            if not managed():
                try:
                    os.waitpid(record['pid'], os.WNOHANG)
                except ChildProcessError:
                    pass  # A different controller process started this server.
                return 'Yerel sunucu durduruldu. Veriler korundu.'
            time.sleep(0.1)
        raise RuntimeError('Durdurma istendi fakat süreç hâlâ açık; zorla öldürülmedi.')
    finally:
        os.close(fd)


def logs(profile):
    if profile == 'kind':
        return run(kube('logs', 'deployment/gorev-atolyesi', '--tail=80'))
    try:
        with (STATE / 'local.log').open('rb') as log:
            log.seek(0, 2)
            log.seek(max(0, log.tell() - 16000))
            return clean(log.read().decode(errors='replace'))
    except FileNotFoundError:
        return 'Henüz yerel log yok.'


def local_env():
    env = dict(os.environ)
    env.setdefault('MFA_KEY_FILE', str(STATE / 'mfa.key'))
    if (STATE / 'accounts-enabled').exists():
        env.update(AUTH_ENABLED='true', ALLOW_SIGNUP='true', ACCOUNT_APPROVAL='true')
    return env


def admin_command(command, email='', after=''):
    env = local_env()
    env['AUTH_ENABLED'] = 'true'
    env['USER_EMAIL'] = email
    env['USER_AFTER'] = after
    return run(['go', 'run', './cmd/gorev-atolyesi', command], timeout=180, env=env)


def mailbox_text(raw):
    messages = json.loads(raw)
    lines = ['Geliştirme posta kutusu / gerçek e-posta gönderilmedi.']
    for item in messages:
        lines.extend(['', item['email'], 'Doğrulama kodu:', item['code'],
                      'Son geçerlilik: ' + time.strftime('%d.%m.%Y %H:%M', time.localtime(item['expires_at']))])
    if not messages:
        lines.append('Geçerli ileti yok. Web ekranından Yeni kod iste seçeneğini kullanın.')
    return '\n'.join(lines)


def action(name, profile):
    if os.environ.get('ATOLYE_LOCAL_ONLY') == 'true' and profile != 'local':
        raise RuntimeError('Demo yalnızca kendi yerel profilini yönetir.')
    if name.startswith('assignment-'):
        if profile != 'local':
            raise RuntimeError('API atama işlemleri yerel panelden yapılandırılır.')
        if name == 'assignment-files':
            folder = ROOT / 'data' / 'imports'
            folder.mkdir(parents=True, exist_ok=True)
            return json.dumps([str(p) for p in sorted(folder.iterdir()) if p.is_file() and not p.is_symlink() and p.suffix.lower() in ('.json', '.xlsx')])
        args = ['python3', 'scripts/atolye_assign.py']
        if name.startswith('assignment-preview:'):
            args += ['preview', '--file', name.split(':', 1)[1]]
        elif name == 'assignment-commit':
            args += ['commit']
        elif name == 'assignment-results':
            args += ['results']
        else:
            raise RuntimeError('Bilinmeyen atama işlemi.')
        args += ['--url', os.environ.get('ATOLYE_API_URL', 'http://' + os.environ.get('ADDR', '127.0.0.1:8090')),
                 '--token-file', os.environ.get('ATOLYE_API_TOKEN_FILE', str(STATE / 'automation.token')),
                 '--preview-file', str(STATE / 'assignment-preview.json')]
        with locked(profile):
            return run(args, timeout=45)
    if name in ('account-mode', 'admin-list', 'mailbox', 'mfa-reset-list') or name.startswith(('bootstrap:', 'mfa-reset:')):
        if profile != 'local':
            raise RuntimeError('Hesap kurulumu yerel profil içindir. Kubernetes DB yönetimi ayrı dağıtım işiyle yapılmalıdır.')
        with locked(profile):
            if name in ('admin-list', 'mfa-reset-list'):
                return admin_command('admin-list')
            if name.startswith('mfa-reset:'):
                admin_command('admin-mfa-reset', name.split(':', 1)[1])
                return 'MFA sıfırlandı, tüm oturumlar kapatıldı. Yönetici web üzerinden MFA kurmadan erişemez.'
            if name == 'mailbox':
                return mailbox_text(admin_command('admin-mailbox'))
            if name.startswith('bootstrap:'):
                return admin_command('admin-bootstrap', name.split(':', 1)[1])
            if managed():
                raise RuntimeError('Önce yerel sunucuyu Durdur ile kapatın; ardından hesaplı modu etkinleştirin.')
            env = dict(os.environ, AUTH_ENABLED='true', ACCOUNT_APPROVAL='true')
            result = run(['go', 'run', './cmd/gorev-atolyesi', 'identity-enable'], timeout=180, env=env)
            (STATE / 'accounts-enabled').write_text('enabled\n')
            return result + '\nŞimdi Başlat seçeneğini kullanıp web üzerinden kayıt olun. Eski kişisel görevler DB içinde korunur; otomatik bir hesaba aktarılmaz.'
    if name == 'status':
        return json.dumps(status(profile), ensure_ascii=False, indent=2)
    if name == 'logs':
        return logs(profile)
    if name == 'backups':
        files = sorted((ROOT / 'backups').glob('*'), key=lambda p: p.stat().st_mtime, reverse=True)
        return '\n'.join(clean(f'{p.name}  {p.stat().st_size} bayt') for p in files[:20] if p.is_file()) or 'backups/ içinde yerel yedek yok. Uzak yedekler sorgulanmadı.'
    if name == 'open':
        record = managed() if profile == 'local' else None
        if not record:
            raise RuntimeError('Tarayıcıda açmak için yönetilen yerel sunucuyu başlatın. Kind için proje-ac.sh port yönlendirmesini kullanın.')
        return run(['xdg-open', record['url']]) or 'Tarayıcıya gönderildi.'
    with locked(profile):
        if profile == 'local':
            if name in ('down', 'restart'):
                result = stop_local()
            if name in ('up', 'restart'):
                result = start_local()
            return result
        # These actions never create/delete clusters or volumes.
        if name in ('down', 'restart'):
            run(['bash', 'scripts/proje-kapat.sh'], timeout=130)
        if name in ('up', 'restart'):
            run(['kind', 'export', 'kubeconfig', '--name', 'go-odev', '--kubeconfig', str(ROOT / '.kubeconfig')])
            run(kube('scale', 'deployment/gorev-atolyesi', '--replicas=1'))
            run(kube('rollout', 'status', 'deployment/gorev-atolyesi', '--timeout=180s'), timeout=195)
        return action('status', profile)


MENU = [('Başlat / yerel kodu derle', 'up'), ('Durdur', 'down'),
        ('Yeniden başlat / güncelle', 'restart'), ('Durumu göster', 'status'),
        ('Logları göster', 'logs'), ('Yedekleri listele', 'backups'), ('Siteyi tarayıcıda aç', 'open'),
        ('Hesaplı kaydı etkinleştir', 'account-mode'), ('İlk yöneticiyi seç', 'admin-list'),
        ('Doğrulama iletilerini göster', 'mailbox'), ('Kayıp MFA / operatör kurtarması', 'mfa-reset-list'),
        ('Dosyadan görev / önizle', 'assignment-files'), ('Son önizlemeyi onayla / ata', 'assignment-commit'),
        ('Toplu atama sonuçları', 'assignment-results')]


def tui(screen, initial_profile):
    curses.curs_set(0)
    screen.timeout(100)
    accent = curses.A_BOLD
    if curses.has_colors() and 'NO_COLOR' not in os.environ:
        curses.start_color()
        curses.use_default_colors()
        curses.init_pair(1, 37 if curses.COLORS >= 256 else curses.COLOR_CYAN, -1)
        accent |= curses.color_pair(1)
    profile, selected, confirm = initial_profile, 0, False
    output, offset = 'Bir işlem seçin. Tab: local / kind profili.', 0
    menu, pending_action = MENU, 'status'
    with concurrent.futures.ThreadPoolExecutor(max_workers=1) as pool:
        future = pool.submit(action, 'status', profile)
        while True:
            if future and future.done():
                try:
                    output = future.result()
                    if pending_action == 'assignment-files':
                        paths = json.loads(output)
                        menu = [(Path(p).name, 'assignment-preview:' + p) for p in paths] + [('Ana menüye dön', 'back')]
                        selected = 0
                        output = 'data/imports klasöründeki dosyayı seç. Önizleme görev oluşturmaz. Sonuçları inceledikten sonra ana menüden Son önizlemeyi onayla / ata seç.'
                    if pending_action in ('admin-list', 'mfa-reset-list'):
                        users = json.loads(output)
                        resetting = pending_action == 'mfa-reset-list'
                        menu = [(u['email'], ('mfa-reset:' if resetting else 'bootstrap:') + u['email']) for u in users if u.get('mfa_enabled') and (resetting or (u['role'] == 'member' and u['status'] != 'suspended' and u.get('email_verified')))]
                        menu.append(('Ana menüye dön', 'back'))
                        selected = 0
                        output = 'Yalnızca MFA cihazı ve kurtarma kodları kaybolduğunda kullan. Seçili hesabın tüm oturumları kapatılır ve MFA yeniden kurulur.' if resetting else 'E-postası doğrulanmış ve MFA kurulmuş hesabını seç. İlk yönetici yalnızca bir kez belirlenir.'
                except Exception as exc:
                    output = 'Hata: ' + clean(exc)
                future, offset = None, 0
            screen.erase()
            height, width = screen.getmaxyx()

            def put(row, text, attr=0):
                if 0 <= row < height:
                    try:
                        screen.addstr(row, 1, clean(text)[:max(0, width - 3)], attr)
                    except curses.error:
                        pass

            put(0, 'Görev Atölyesi / İşletim tezgâhı', accent)
            put(1, f'Profil: {profile}   ' + ('İşlem sürüyor…' if future else 'Hazır'))
            if height < 19 or width < 40:
                put(3, 'Terminali en az 40 × 19 boyutuna büyütün.')
            else:
                start = max(0, selected - 8)
                for i in range(start, min(len(menu), start + 9)):
                    label = menu[i][0]
                    put(3 + i - start, ('› ' if i == selected else '  ') + label,
                        curses.A_REVERSE if i == selected else 0)
                put(13, 'Sonuç / PgUp ve PgDn ile kaydır', accent)
                lines = output.splitlines()
                offset = min(offset, max(0, len(lines) - max(1, height - 17)))
                for i, line in enumerate(lines[offset:offset + max(1, height - 17)]):
                    put(14 + i, line)
            put(height - 2, f'{profile}: {menu[selected][0]}? Enter: onay / Esc: vazgeç' if confirm else
                '↑↓ Seç  Enter Çalıştır  Tab Profil  q Çık')
            put(height - 1, 'Panelden çıkmak sunucuyu durdurmaz.')
            screen.refresh()
            key = screen.getch()
            if key in (ord('q'), 3):
                if future:
                    output = 'İşlem sürüyor. Sonuçlandıktan sonra q ile çıkın.'
                else:
                    return
            if future:
                continue
            if key == 27:
                if confirm:
                    confirm = False
                elif menu is not MENU:
                    menu, selected = MENU, 0
            elif key in (curses.KEY_UP, curses.KEY_DOWN):
                selected = (selected + (1 if key == curses.KEY_DOWN else -1)) % len(menu)
                confirm = False
            elif key == 9:
                if os.environ.get('ATOLYE_LOCAL_ONLY') == 'true':
                    output = 'Demo kendi yerel profilinde kalır; Kubernetes profili bu panelde kapalıdır.'
                    continue
                profile = 'kind' if profile == 'local' else 'local'
                menu, selected = MENU, 0
                confirm = False
                output = 'Profil değişti. Durumu göster ile sorgulayın.'
            elif key in (curses.KEY_NPAGE, curses.KEY_PPAGE):
                offset = max(0, offset + (5 if key == curses.KEY_NPAGE else -5))
            elif key in (10, 13, curses.KEY_ENTER):
                name = menu[selected][1]
                if name == 'back':
                    menu, selected = MENU, 0
                    continue
                if (name in ('down', 'restart', 'account-mode', 'assignment-commit') or name.startswith(('bootstrap:', 'mfa-reset:'))) and not confirm:
                    confirm = True
                    if name == 'account-mode':
                        output = 'Bu DB için hesap zorunluluğu kalıcı olur. Eski kişisel görevler korunur; oturumlarda otomatik görünmez.'
                else:
                    confirm = False
                    output = 'İşlem sürüyor…'
                    pending_action = name
                    future = pool.submit(action, name, profile)


def main():
    os.umask(0o077)
    parser = argparse.ArgumentParser(description='Görev Atölyesi yerel TUI ve işletim komutları')
    parser.add_argument('area', choices=['control', 'system', 'admin'])
    parser.add_argument('command', choices=['tui', 'up', 'down', 'restart', 'status', 'logs', 'backups', 'open', 'account-mode', 'list', 'bootstrap', 'mailbox', 'mfa-reset'])
    parser.add_argument('--email', default='')
    parser.add_argument('--after', default='')
    parser.add_argument('--profile', choices=['local', 'kind'], default='local')
    parser.add_argument('--json', action='store_true', help='Sonucu JSON olarak yazdır')
    args = parser.parse_args()
    try:
        if args.area == 'admin':
            if args.command not in ('list', 'bootstrap', 'mailbox','mfa-reset') or args.profile != 'local':
                parser.error('admin list, admin mailbox veya admin bootstrap --email adres kullanın (local profil).')
            if args.command in ('bootstrap','mfa-reset') and not args.email:
                parser.error('--email gerekli; önce web üzerinden kayıt olun.')
            with locked('local'):
                result = admin_command('admin-' + args.command, args.email, args.after)
            print(json.dumps({'ok': True, 'result': json.loads(result) if args.command in ('list','mailbox') else result}, ensure_ascii=False) if args.json else mailbox_text(result) if args.command == 'mailbox' else result)
            return 0
        if (args.area == 'control') != (args.command == 'tui') or args.command in ('list', 'bootstrap','mfa-reset'):
            parser.error('control tui veya system <işlem> kullanın.')
        if args.command == 'tui':
            if not sys.stdin.isatty() or not sys.stdout.isatty() or os.environ.get('TERM', 'dumb') == 'dumb':
                parser.error('TUI etkileşimli terminal gerektirir. system status --json kullanın.')
            # Read Ctrl+C as a key so active work is not abandoned halfway.
            def start(screen):
                curses.raw()
                screen.keypad(True)
                tui(screen, args.profile)
            curses.wrapper(start)
        else:
            result = action(args.command, args.profile)
            print(json.dumps({'ok': True, 'result': json.loads(result) if args.command == 'status' else result}, ensure_ascii=False)
                  if args.json else result)
        return 0
    except (Exception, KeyboardInterrupt) as exc:
        message = clean(exc) or 'İşlem kesildi; durumu kontrol edin.'
        print(json.dumps({'ok': False, 'error': message}, ensure_ascii=False) if args.json else 'Hata: ' + message, file=sys.stderr)
        return 1


if __name__ == '__main__':
    sys.exit(main())
