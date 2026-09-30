"""Operator-only demo credentials. Never imported by the server or exposed over HTTP."""
import base64
import hashlib
import hmac
import json
import os
import struct
import time

with open(os.environ['DEMO_ACCOUNTS_FILE']) as source:
    accounts = json.load(source)
print('Yalnızca yerel demo hesapları. Bu bilgileri paylaşma.')
for account in accounts:
    print('\n' + account['email'] + '\nParola: ' + account['password'])
    if secret := account.get('totp_secret'):
        now = int(time.time())
        value = hmac.new(base64.b32decode(secret), struct.pack('>Q', now // 30), hashlib.sha1).digest()
        offset = value[-1] & 15
        code = (int.from_bytes(value[offset:offset + 4], 'big') & 0x7fffffff) % 1000000
        print(f'Demo MFA kodu: {code:06d} ({30 - now % 30} saniye sonra yenilenir)')
        print('İstersen doğrulama uygulamana ekle / TOTP anahtarı: ' + secret)
