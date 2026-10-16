#!/usr/bin/env python3
"""
Lightweight zero-compilation utility for testing and debugging tvOS version
resolution in ipatool-webGUI. Works out of the box on both macOS and Windows
with standard Python 3 (no pip packages required) and automatically saves all
output to a text file (tvos_test_<app_id>.txt).

Usage (macOS / Linux):
    python3 scripts/test_tvos_versions.py 448868000

Usage (Windows):
    python scripts\\test_tvos_versions.py 448868000
"""

import argparse
import base64
import datetime
import hashlib
import io
import json
import os
import pathlib
import platform
import plistlib
import re
import ssl
import struct
import subprocess
import sys
import time
import urllib.error
import urllib.parse
import urllib.request
import uuid
import zipfile

# ============================================================================
# 1. Pure-Python AES-128/256 + JWE (PBES2-HS256+A128KW / A256GCM) for Windows
# ============================================================================

SBOX = [
    0x63,0x7c,0x77,0x7b,0xf2,0x6b,0x6f,0xc5,0x30,0x01,0x67,0x2b,0xfe,0xd7,0xab,0x76,
    0xca,0x82,0xc9,0x7d,0xfa,0x59,0x47,0xf0,0xad,0xd4,0xa2,0xaf,0x9c,0xa4,0x72,0xc0,
    0xb7,0xfd,0x93,0x26,0x36,0x3f,0xf7,0xcc,0x34,0xa5,0xe5,0xf1,0x71,0xd8,0x31,0x15,
    0x04,0xc7,0x23,0xc3,0x18,0x96,0x05,0x9a,0x07,0x12,0x80,0xe2,0xeb,0x27,0xb2,0x75,
    0x09,0x83,0x2c,0x1a,0x1b,0x6e,0x5a,0xa0,0x52,0x3b,0xd6,0xb3,0x29,0xe3,0x2f,0x84,
    0x53,0xd1,0x00,0xed,0x20,0xfc,0xb1,0x5b,0x6a,0xcb,0xbe,0x39,0x4a,0x4c,0x58,0xcf,
    0xd0,0xef,0xaa,0xfb,0x43,0x4d,0x33,0x85,0x45,0xf9,0x02,0x7f,0x50,0x3c,0x9f,0xa8,
    0x51,0xa3,0x40,0x8f,0x92,0x9d,0x38,0xf5,0xbc,0xb6,0xda,0x21,0x10,0xff,0xf3,0xd2,
    0xcd,0x0c,0x13,0xec,0x5f,0x97,0x44,0x17,0xc4,0xa7,0x7e,0x3d,0x64,0x5d,0x19,0x73,
    0x60,0x81,0x4f,0xdc,0x22,0x2a,0x90,0x88,0x46,0xee,0xb8,0x14,0xde,0x5e,0x0b,0xdb,
    0xe0,0x32,0x3a,0x0a,0x49,0x06,0x24,0x5c,0xc2,0xd3,0xac,0x62,0x91,0x95,0xe4,0x79,
    0xe7,0xc8,0x37,0x6d,0x8d,0xd5,0x4e,0xa9,0x6c,0x56,0xf4,0xea,0x65,0x7a,0xae,0x08,
    0xba,0x78,0x25,0x2e,0x1c,0xa6,0xb4,0xc6,0xe8,0xdd,0x74,0x1f,0x4b,0xbd,0x8b,0x8a,
    0x70,0x3e,0xb5,0x66,0x48,0x03,0xf6,0x0e,0x61,0x35,0x57,0xb9,0x86,0xc1,0x1d,0x9e,
    0xe1,0xf8,0x98,0x11,0x69,0xd9,0x8e,0x94,0x9b,0x1e,0x87,0xe9,0xce,0x55,0x28,0xdf,
    0x8c,0xa1,0x89,0x0d,0xbf,0xe6,0x42,0x68,0x41,0x99,0x2d,0x0f,0xb0,0x54,0xbb,0x16,
]
INV_SBOX = [0] * 256
for _i, _v in enumerate(SBOX):
    INV_SBOX[_v] = _i


def _xtime(a: int) -> int:
    return ((a << 1) ^ 0x1B) & 0xFF if (a & 0x80) else (a << 1)


def _gmul(a: int, b: int) -> int:
    p = 0
    for _ in range(8):
        if b & 1:
            p ^= a
        a = _xtime(a)
        b >>= 1
    return p


def _aes_key_expansion(key: bytes):
    nk = len(key) // 4
    nr = nk + 6
    w = [list(key[4 * i : 4 * i + 4]) for i in range(nk)]
    rcon = 1
    for i in range(nk, 4 * (nr + 1)):
        temp = list(w[i - 1])
        if i % nk == 0:
            temp = [SBOX[temp[1]] ^ rcon, SBOX[temp[2]], SBOX[temp[3]], SBOX[temp[0]]]
            rcon = _xtime(rcon)
        elif nk > 6 and i % nk == 4:
            temp = [SBOX[b] for b in temp]
        w.append([w[i - nk][j] ^ temp[j] for j in range(4)])
    return [sum(w[4 * r : 4 * r + 4], []) for r in range(nr + 1)]


def _aes_encrypt_block(block: bytes, rkeys) -> bytes:
    s = [block[i] ^ rkeys[0][i] for i in range(16)]
    for rnd in range(1, len(rkeys) - 1):
        s = [SBOX[b] for b in s]
        s = [s[0],s[5],s[10],s[15], s[4],s[9],s[14],s[3], s[8],s[13],s[2],s[7], s[12],s[1],s[6],s[11]]
        ns = [0] * 16
        for c in range(4):
            i = 4 * c
            a0, a1, a2, a3 = s[i], s[i + 1], s[i + 2], s[i + 3]
            ns[i]     = _gmul(a0, 2) ^ _gmul(a1, 3) ^ a2 ^ a3
            ns[i + 1] = a0 ^ _gmul(a1, 2) ^ _gmul(a2, 3) ^ a3
            ns[i + 2] = a0 ^ a1 ^ _gmul(a2, 2) ^ _gmul(a3, 3)
            ns[i + 3] = _gmul(a0, 3) ^ a1 ^ a2 ^ _gmul(a3, 2)
        s = [ns[i] ^ rkeys[rnd][i] for i in range(16)]
    s = [SBOX[b] for b in s]
    s = [s[0],s[5],s[10],s[15], s[4],s[9],s[14],s[3], s[8],s[13],s[2],s[7], s[12],s[1],s[6],s[11]]
    return bytes(s[i] ^ rkeys[-1][i] for i in range(16))


def _aes_decrypt_block(block: bytes, rkeys) -> bytes:
    s = [block[i] ^ rkeys[-1][i] for i in range(16)]
    for rnd in range(len(rkeys) - 2, 0, -1):
        s = [s[0],s[13],s[10],s[7], s[4],s[1],s[14],s[11], s[8],s[5],s[2],s[15], s[12],s[9],s[6],s[3]]
        s = [INV_SBOX[b] for b in s]
        s = [s[i] ^ rkeys[rnd][i] for i in range(16)]
        ns = [0] * 16
        for c in range(4):
            i = 4 * c
            a0, a1, a2, a3 = s[i], s[i + 1], s[i + 2], s[i + 3]
            ns[i]     = _gmul(a0, 14) ^ _gmul(a1, 11) ^ _gmul(a2, 13) ^ _gmul(a3, 9)
            ns[i + 1] = _gmul(a0, 9) ^ _gmul(a1, 14) ^ _gmul(a2, 11) ^ _gmul(a3, 13)
            ns[i + 2] = _gmul(a0, 13) ^ _gmul(a1, 9) ^ _gmul(a2, 14) ^ _gmul(a3, 11)
            ns[i + 3] = _gmul(a0, 11) ^ _gmul(a1, 13) ^ _gmul(a2, 9) ^ _gmul(a3, 14)
        s = ns
    s = [s[0],s[13],s[10],s[7], s[4],s[1],s[14],s[11], s[8],s[5],s[2],s[15], s[12],s[9],s[6],s[3]]
    s = [INV_SBOX[b] for b in s]
    return bytes(s[i] ^ rkeys[0][i] for i in range(16))


def _b64url_decode(s: str) -> bytes:
    pad = "=" * ((4 - len(s) % 4) % 4)
    return base64.urlsafe_b64decode(s + pad)


def _aes_key_unwrap(kek: bytes, wrapped: bytes) -> bytes:
    n = (len(wrapped) // 8) - 1
    rkeys = _aes_key_expansion(kek)
    a = wrapped[:8]
    r = [wrapped[8 * i : 8 * (i + 1)] for i in range(1, n + 1)]
    for j in range(5, -1, -1):
        for i in range(n, 0, -1):
            t = (n * j) + i
            a_int = struct.unpack(">Q", a)[0] ^ t
            b = _aes_decrypt_block(struct.pack(">Q", a_int) + r[i - 1], rkeys)
            a = b[:8]
            r[i - 1] = b[8:]
    if a != b"\xa6\xa6\xa6\xa6\xa6\xa6\xa6\xa6":
        raise ValueError("AES Key Unwrap integrity check failed")
    return b"".join(r)


def _aes_gcm_ctr_decrypt(cek: bytes, iv: bytes, ciphertext: bytes) -> bytes:
    rkeys = _aes_key_expansion(cek)
    out = bytearray()
    blocks = (len(ciphertext) + 15) // 16
    for blk in range(blocks):
        counter = iv + struct.pack(">I", blk + 2)
        ks = _aes_encrypt_block(counter, rkeys)
        chunk = ciphertext[16 * blk : 16 * (blk + 1)]
        out.extend(c ^ k for c, k in zip(chunk, ks))
    return bytes(out)


def decrypt_keyring_jwe(token: str, passphrase: str) -> bytes:
    parts = token.strip().split(".")
    if len(parts) != 5:
        raise ValueError("Not a 5-part JWE token")
    header = json.loads(_b64url_decode(parts[0]).decode("utf-8"))
    enc_key = _b64url_decode(parts[1])
    iv = _b64url_decode(parts[2])
    ciphertext = _b64url_decode(parts[3])

    alg = header.get("alg", "PBES2-HS256+A128KW")
    p2s = _b64url_decode(header["p2s"])
    p2c = int(header["p2c"])
    salt = alg.encode("utf-8") + b"\x00" + p2s
    kek = hashlib.pbkdf2_hmac("sha256", passphrase.encode("utf-8"), salt, p2c, 16)
    cek = _aes_key_unwrap(kek, enc_key)
    plaintext = _aes_gcm_ctr_decrypt(cek, iv, ciphertext)
    item = json.loads(plaintext.decode("utf-8"))
    if isinstance(item, dict) and "Data" in item:
        return base64.b64decode(item["Data"])
    return plaintext


# ============================================================================
# 2. Load Account, KBSync, GUID, and Cookies (macOS Keychain + Windows JWE)
# ============================================================================

def get_mac_guid(kbsync_entry: dict = None, dsid: str = "") -> str:
    # 1. If kbsync-v1 has {"dsid": ..., "guid": "...", "data": "..."}, reuse that exact GUID
    if isinstance(kbsync_entry, dict):
        entry_guid = str(kbsync_entry.get("guid", "")).strip().upper().replace(":", "")
        if len(entry_guid) == 12:
            return entry_guid
        for k in kbsync_entry.keys():
            if ":" in k:
                k_dsid, k_guid = k.split(":", 1)
                k_guid = k_guid.strip().upper().replace(":", "")
                if len(k_guid) == 12 and (not dsid or k_dsid == dsid):
                    return k_guid

    # 2. On macOS, query en0 hardware MAC address (matching mac_address_darwin.go)
    if platform.system() == "Darwin":
        for cmd in (
            ["networksetup", "-getmacaddress", "en0"],
            ["ifconfig", "en0"],
        ):
            try:
                res = subprocess.run(cmd, capture_output=True, text=True, timeout=3)
                if res.returncode == 0 and res.stdout:
                    m = re.search(r"([0-9A-Fa-f]{2}(?::[0-9A-Fa-f]{2}){5})", res.stdout)
                    if m:
                        mac_hex = m.group(1).replace(":", "").upper()
                        if mac_hex != "000000000000":
                            return mac_hex
            except Exception:
                pass

    # 3. Fallback to uuid.getnode()
    node = uuid.getnode()
    return f"{node:012X}"


def _parse_raw_keychain_secret(raw: str):
    raw = raw.strip()
    if not raw:
        return None
    for candidate in (raw,):
        try:
            return json.loads(candidate)
        except Exception:
            pass
    try:
        decoded = bytes.fromhex(raw).decode("utf-8", errors="replace").strip()
        return json.loads(decoded)
    except Exception:
        pass
    return None


def load_keychain_item(state_dir: pathlib.Path, key: str):
    # 1. On macOS, check macOS Keychain via `security find-generic-password`
    if platform.system() == "Darwin":
        for service in ("ipatool-webgui-auth.service", "ipatool-auth.service"):
            try:
                res = subprocess.run(
                    ["security", "find-generic-password", "-s", service, "-a", key, "-w"],
                    capture_output=True,
                    text=True,
                    timeout=10,
                )
                if res.returncode == 0 and res.stdout.strip():
                    parsed = _parse_raw_keychain_secret(res.stdout)
                    if parsed is not None:
                        return parsed
            except Exception:
                pass
            # Fallback with -g (which prints password to stderr in `password: "..."` or `0x...` format)
            try:
                res = subprocess.run(
                    ["security", "find-generic-password", "-s", service, "-a", key, "-g"],
                    capture_output=True,
                    text=True,
                    timeout=10,
                )
                combined = (res.stderr or "") + "\n" + (res.stdout or "")
                m_hex = re.search(r"password:\s*0x([0-9A-Fa-f]+)", combined)
                if m_hex:
                    parsed = _parse_raw_keychain_secret(m_hex.group(1))
                    if parsed is not None:
                        return parsed
                m_str = re.search(r'password:\s*"(.*)"', combined)
                if m_str:
                    parsed = _parse_raw_keychain_secret(m_str.group(1))
                    if parsed is not None:
                        return parsed
            except Exception:
                pass

    # 2. Check FileBackend in ~/.ipatool-webgui/<key> (Windows / Linux / macOS fallback)
    file_path = state_dir / key
    pass_path = state_dir / "webgui-keychain-passphrase"
    if file_path.exists() and pass_path.exists():
        try:
            passphrase = pass_path.read_text(encoding="utf-8").strip()
            raw_token = file_path.read_text(encoding="utf-8").strip()
            data = decrypt_keyring_jwe(raw_token, passphrase)
            return json.loads(data.decode("utf-8"))
        except Exception:
            pass

    return None


def load_cookies_and_fallback_account(state_dir: pathlib.Path):
    cookie_file = state_dir / "cookies"
    fallback_acc = {}
    if not cookie_file.exists():
        return "", fallback_acc
    try:
        entries = json.loads(cookie_file.read_text(encoding="utf-8"))
        pairs = []
        seen = set()
        for e in entries:
            name = e.get("Name")
            val = e.get("Value")
            if name and val and name not in seen:
                seen.add(name)
                pairs.append(f"{name}={val}")
                if name.startswith("mz_at0-"):
                    dsid = name.split("mz_at0-", 1)[1]
                    if dsid.isdigit():
                        fallback_acc["directoryServicesIdentifier"] = dsid
                        fallback_acc["passwordToken"] = val
                elif name == "itspod" and val.isdigit():
                    fallback_acc["pod"] = val
        return "; ".join(pairs), fallback_acc
    except Exception:
        return "", fallback_acc


# ============================================================================
# 3. DMAP / DAAP Binary Encoder & Decoder
# ============================================================================

CONTAINER_TAGS = {
    "adbs", "agal", "agar", "aply", "apso", "arli", "avdb", "cmgt", "caci",
    "fags", "loci", "mbcl", "mccr", "mdcl", "mcon", "mlcl", "mlid", "mlit",
    "mlog", "mpco", "mshl", "msrv", "mstt", "msup", "mtco", "mudl", "mupd",
    "avai", "abro", "abar", "abcp", "abgn", "adbs", "aefl", "aeif", "ajrl",
    "alhf", "fhla", "alif", "aelf", "adsr",
}

DAAP_EXTENDED_META_FIELDS = (
    "dmap.itemname,dmap.itemid,dmap.longitemid,dmap.downloadstatus,dmap.itemhidden,"
    "daap.songartist,daap.songdatepurchased,daap.songgenre,"
    "com.apple.itunes.item-flavor-listing,com.apple.itunes.item-flavor,com.apple.itunes.item-flavors,"
    "com.apple.itunes.item-redownload-param-2,com.apple.itunes.item-redownload-param,"
    "com.apple.itunes.itms-artistid,com.apple.itunes.itms-genreid,com.apple.itunes.itms-playlistid,"
    "com.apple.itunes.itms-songid,com.apple.itunes.itms-storefrontid,"
    "com.apple.itunes.cloud-id,com.apple.itunes.cloud-purchased-token,"
    "com.apple.itunes.extended-media-kind,com.apple.itunes.supported-devices,"
    "com.apple.itunes.supports-layered-image,com.apple.itunes.bundle-id,com.apple.itunes.version,"
    "com.apple.itunes.display-publication-version,com.apple.itunes.store.preflight-package-url,"
    "com.apple.itunes.store.redownload-action-params,com.apple.itunes.store.software-bundle-id,"
    "com.apple.itunes.store.software-device-families,"
    "com.apple.itunes.store.software-version-external-identifier,com.apple.itunes.store.version-string"
)


def dmap_tag(tag: str, payload: bytes = b"") -> bytes:
    return tag.encode("ascii") + struct.pack(">I", len(payload)) + payload


def dmap_uint8(tag: str, val: int) -> bytes:
    return dmap_tag(tag, struct.pack(">B", val))


def dmap_uint32(tag: str, val: int) -> bytes:
    return dmap_tag(tag, struct.pack(">I", val))


def dmap_string(tag: str, val: str) -> bytes:
    return dmap_tag(tag, val.encode("utf-8"))


def dmap_int(payload: bytes) -> int:
    if len(payload) == 1:
        return payload[0]
    if len(payload) == 2:
        return struct.unpack(">H", payload)[0]
    if len(payload) == 4:
        return struct.unpack(">I", payload)[0]
    if len(payload) == 8:
        return struct.unpack(">Q", payload)[0]
    return 0


def first_dmap_uint(data: bytes, target: str):
    offset = 0
    while offset + 8 <= len(data):
        tag = data[offset : offset + 4].decode("latin1", errors="ignore")
        size = struct.unpack(">I", data[offset + 4 : offset + 8])[0]
        offset += 8
        if offset + size > len(data):
            break
        payload = data[offset : offset + size]
        offset += size
        if tag == target:
            return dmap_int(payload)
        if tag in CONTAINER_TAGS and tag != "mlid":
            sub = first_dmap_uint(payload, target)
            if sub is not None:
                return sub
    return None


def parse_dmap_mlit_items(data: bytes):
    items = []

    def walk(buf: bytes, depth: int = 0):
        offset = 0
        while offset + 8 <= len(buf):
            tag = buf[offset : offset + 4].decode("latin1", errors="ignore")
            size = struct.unpack(">I", buf[offset + 4 : offset + 8])[0]
            offset += 8
            if offset + size > len(buf):
                break
            payload = buf[offset : offset + size]
            offset += size
            if tag == "mlit":
                items.append(decode_mlit(payload))
            elif tag in CONTAINER_TAGS and depth < 12:
                walk(payload, depth + 1)

    walk(data, 0)
    return items


def decode_mlit(buf: bytes, depth: int = 0) -> dict:
    out = {"_tags": [], "_flavors": [], "_strings": {}}
    offset = 0
    while offset + 8 <= len(buf):
        tag = buf[offset : offset + 4].decode("latin1", errors="ignore")
        size = struct.unpack(">I", buf[offset + 4 : offset + 8])[0]
        offset += 8
        if offset + size > len(buf):
            break
        payload = buf[offset : offset + size]
        offset += size
        out["_tags"].append(tag)

        if tag in ("aeSI", "aeAi", "miid", "mild", "aeMk", "aeSS", "mihi", "mihd", "ajhj", "aeli", "asdp"):
            out[tag] = dmap_int(payload)
        elif tag in ("aeBI", "aeLN", "minm", "asar", "aePd", "aeRp", "aeRP", "ajpu", "ajPU", "aeif"):
            try:
                s = payload.decode("utf-8", errors="replace").strip()
                out[tag] = s
                out["_strings"][tag] = s
            except Exception:
                pass
            if tag == "aeif" and len(payload) >= 8:
                out["_flavors"].append(decode_mlit(payload, depth + 1))
        elif tag in ("aefl", "alhf", "fhla", "alif", "aelf", "mlcl", "mlit", "mdcl") and depth < 6:
            out["_flavors"].append(decode_mlit(payload, depth + 1))
        else:
            if 4 <= len(payload) <= 512:
                try:
                    txt = payload.decode("utf-8")
                    if txt.isprintable():
                        out["_strings"][tag] = txt
                except Exception:
                    pass
    return out


# ============================================================================
# 4. HTTP & Remote IPA Info.plist Inspector (with macOS SSL support)
# ============================================================================

CONFIGURATOR_DL_UA = "Configurator/2.17 (Macintosh; OS X 15.2; 24C5089c) AppleWebKit/0620.1.16.11.6"
CONFIGURATOR_ENT_UA = "Configurator/2.18 (Macintosh; OS X 15.3.2; 24D81) AppleWebKit/0620.2.4.11.6"
TVOS_TA_DEVICES = "AppleTV14,1 AppleTV6,2 AppleTV5,3"
DAAP_BASE_URL = "https://pd.itunes.apple.com/WebObjects/MZPurchaseDaap.woa/purchase"
ENT_DOWNLOAD_URL = "https://downloaddispatch.itunes.apple.com/WebObjects/DownloadDispatch.woa/wa/ent/download"


def _build_ssl_context():
    ctx = ssl.create_default_context()
    # On macOS, Python.org builds may not load /etc/ssl/cert.pem by default
    if platform.system() == "Darwin" and os.path.exists("/etc/ssl/cert.pem"):
        try:
            ctx.load_verify_locations(cafile="/etc/ssl/cert.pem")
        except Exception:
            pass
    return ctx


_SSL_CTX = _build_ssl_context()
_UNVERIFIED_SSL_CTX = ssl._create_unverified_context()


def http_send(url: str, method: str = "GET", headers: dict = None, body: bytes = None, timeout: int = 20):
    req = urllib.request.Request(url, data=body, headers=headers or {}, method=method)
    for ctx in (_SSL_CTX, _UNVERIFIED_SSL_CTX):
        try:
            with urllib.request.urlopen(req, timeout=timeout, context=ctx) as resp:
                return resp.status, resp.read(), dict(resp.headers)
        except urllib.error.HTTPError as e:
            return e.code, e.read(), dict(e.headers)
        except urllib.error.URLError as e:
            if "CERTIFICATE_VERIFY_FAILED" in str(e) and ctx is _SSL_CTX:
                continue
            return 0, str(e).encode("utf-8"), {}
        except Exception as e:
            return 0, str(e).encode("utf-8"), {}
    return 0, b"", {}


class HTTPRangeFile(io.RawIOBase):
    def __init__(self, url: str):
        self.url = url
        self.pos = 0
        _, _, hdrs = http_send(url, method="HEAD", headers={"User-Agent": CONFIGURATOR_DL_UA})
        cl = hdrs.get("Content-Length") or hdrs.get("content-length")
        if not cl:
            _, _, hdrs = http_send(url, method="GET", headers={"Range": "bytes=0-0", "User-Agent": CONFIGURATOR_DL_UA})
            cr = hdrs.get("Content-Range") or hdrs.get("content-range") or ""
            if "/" in cr:
                cl = cr.split("/")[-1]
        self.size = int(cl) if cl else 0
        self._tail_cache = None
        self._tail_start = 0

    def seekable(self):
        return True

    def readable(self):
        return True

    def seek(self, offset, whence=io.SEEK_SET):
        if whence == io.SEEK_SET:
            self.pos = offset
        elif whence == io.SEEK_CUR:
            self.pos += offset
        elif whence == io.SEEK_END:
            self.pos = self.size + offset
        return self.pos

    def tell(self):
        return self.pos

    def readinto(self, b):
        if self.pos >= self.size:
            return 0
        n = min(len(b), self.size - self.pos)
        end = self.pos + n - 1
        if self._tail_cache is None and self.size > 0 and self.pos >= max(0, self.size - 98304):
            self._tail_start = max(0, self.size - 98304)
            _, data, _ = http_send(
                self.url,
                headers={"Range": f"bytes={self._tail_start}-{self.size - 1}", "User-Agent": CONFIGURATOR_DL_UA},
            )
            self._tail_cache = data
        if self._tail_cache and self.pos >= self._tail_start and end < self._tail_start + len(self._tail_cache):
            start_off = self.pos - self._tail_start
            chunk = self._tail_cache[start_off : start_off + n]
            b[: len(chunk)] = chunk
            self.pos += len(chunk)
            return len(chunk)

        _, data, _ = http_send(
            self.url,
            headers={"Range": f"bytes={self.pos}-{end}", "User-Agent": CONFIGURATOR_DL_UA},
        )
        b[: len(data)] = data
        self.pos += len(data)
        return len(data)


def inspect_remote_ipa_info_plist(package_url: str) -> dict:
    rf = HTTPRangeFile(package_url)
    with zipfile.ZipFile(io.BufferedReader(rf)) as zf:
        for name in zf.namelist():
            parts = name.split("/")
            if len(parts) == 3 and parts[0] == "Payload" and parts[1].endswith(".app") and parts[2] == "Info.plist":
                raw = zf.read(name)
                info = plistlib.loads(raw)
                return {
                    "CFBundleSupportedPlatforms": info.get("CFBundleSupportedPlatforms", []),
                    "DTPlatformName": info.get("DTPlatformName", ""),
                    "CFBundleShortVersionString": info.get("CFBundleShortVersionString", ""),
                    "MinimumOSVersion": info.get("MinimumOSVersion") or info.get("TVOSMinimumOSVersion", ""),
                }
    return {}


# ============================================================================
# 5. Main Diagnostic Pipeline + Automatic Text File Logger
# ============================================================================

def extract_version_ids_from_text(text: str):
    out = []
    for m in re.finditer(r'(?:appExtVrsId|externalVersionId|softwareVersionExternalIdentifier|externalId|versionId|evid)[=:]\s*"?([0-9]{5,})', text):
        v = m.group(1)
        if v not in out:
            out.append(v)
    return out


class TeeWriter:
    def __init__(self, file_path: pathlib.Path):
        self.file_path = file_path
        self.f = open(file_path, "w", encoding="utf-8")

    def log(self, msg: str = ""):
        print(msg)
        self.f.write(msg + "\n")
        self.f.flush()

    def close(self):
        try:
            self.f.close()
        except Exception:
            pass


def main():
    parser = argparse.ArgumentParser(description="Lightweight tvOS version resolution tester (macOS & Windows, no compilation needed)")
    parser.add_argument("app_id", type=int, help="App Store Adam ID (e.g. 448868000)")
    parser.add_argument("--candidate", action="append", default=[], help="Extra externalVersionId candidate(s) to test")
    parser.add_argument("-o", "--output", default="", help="Output text file path (default: ./tvos_test_<app_id>.txt)")
    args = parser.parse_args()

    app_id = args.app_id
    out_path = pathlib.Path(args.output) if args.output else (pathlib.Path.cwd() / f"tvos_test_{app_id}.txt")
    tee = TeeWriter(out_path)
    log = tee.log

    try:
        state_dir = pathlib.Path.home() / ".ipatool-webgui"
        log(f"=== tvOS Version Tester for App ID {app_id} ===")
        log(f"Timestamp: {datetime.datetime.now().isoformat(timespec='seconds')}")
        log(f"OS: {platform.system()} {platform.release()} ({platform.machine()}) | Python {platform.python_version()}")
        log(f"State directory: {state_dir}")
        log(f"Output file: {out_path.resolve()}")

        cookie_hdr, fallback_acc = load_cookies_and_fallback_account(state_dir)
        acc = load_keychain_item(state_dir, "account") or fallback_acc
        if not acc or not acc.get("directoryServicesIdentifier") or not acc.get("passwordToken"):
            log("ERROR: Could not load account from macOS Keychain, ~/.ipatool-webgui/account, or ~/.ipatool-webgui/cookies.")
            log("Make sure you are logged in to ipatool-webGUI (and on macOS click 'Allow' if Keychain prompts).")
            sys.exit(1)

        dsid = str(acc.get("directoryServicesIdentifier", "")).strip()
        token = str(acc.get("passwordToken", "")).strip()
        store_front = str(acc.get("storeFront") or "143469-24,29").strip()
        pod = str(acc.get("pod") or fallback_acc.get("pod") or "").strip()
        email = str(acc.get("email", "loaded-from-session")).strip()

        kbsync_entry = load_keychain_item(state_dir, "kbsync-v1") or load_keychain_item(state_dir, "kbsync_cache") or {}
        guid = get_mac_guid(kbsync_entry if isinstance(kbsync_entry, dict) else None, dsid)

        sf_base = store_front.split(",")[0] if "," in store_front else store_front
        sf_numeric = sf_base.split("-")[0] if "-" in sf_base else sf_base

        kbsync_b64 = ""
        if isinstance(kbsync_entry, dict):
            kbsync_b64 = str(kbsync_entry.get("data", "")).strip()
            if not kbsync_b64:
                kbsync_b64 = kbsync_entry.get(f"{dsid}:{guid}", "") or next((v for v in kbsync_entry.values() if isinstance(v, str) and len(v) > 20), "")

        hw_bytes = bytes.fromhex(guid) if len(guid) == 12 else b"\x00" * 6
        serial_b64 = base64.b64encode(b"\x54\xc8\xb0\xa9\x88" + hw_bytes[2:]).decode("ascii")

        log(f"Account: {email} | DSID: {dsid} | StoreFront: {store_front} | Pod: {pod} | GUID: {guid}")
        log(f"Cookies loaded: {'yes' if cookie_hdr else 'no'} | Cached kbsync: {'yes (' + str(len(kbsync_b64)) + ' chars)' if kbsync_b64 else 'no'}")

        candidates = list(args.candidate)
        ios_train_set = set()
        unpinned_first = ""
        unpinned_latest = ""

        def common_headers(ua=CONFIGURATOR_DL_UA, sf=store_front, ta_device=None, content_type="application/x-apple-plist"):
            h = {
                "Content-Type": content_type,
                "User-Agent": ua,
                "X-Apple-Store-Front": sf,
                "X-Dsid": dsid,
                "iCloud-DSID": dsid,
                "X-Token": token,
            }
            if cookie_hdr:
                h["Cookie"] = cookie_hdr
            if ta_device:
                h["X-Apple-TA-Device"] = ta_device
            return h

        # --------------------------------------------------------------------
        # Step 1: Unpinned Download Check (volumeStoreDownloadProduct + ent/download)
        # --------------------------------------------------------------------
        log("\n--- [Step 1] Unpinned Download & Remote IPA Platform Check ---")
        vs_host = f"p{pod}-buy.itunes.apple.com" if pod else "buy.itunes.apple.com"
        vs_url = f"https://{vs_host}/WebObjects/MZFinance.woa/wa/volumeStoreDownloadProduct?guid={guid}"
        ent_url = f"{ENT_DOWNLOAD_URL}?guid={guid}"

        step1_probes = [
            (
                "volumeStore",
                vs_url,
                {
                    "Content-Type": "application/x-apple-plist",
                    "User-Agent": CONFIGURATOR_DL_UA,
                    "iCloud-DSID": dsid,
                    "X-Dsid": dsid,
                    **({"Cookie": cookie_hdr} if cookie_hdr else {}),
                },
                {
                    "creditDisplay": "",
                    "guid": guid,
                    "salableAdamId": app_id,
                    "serialNumber": "0",
                },
            ),
        ]
        if kbsync_b64:
            step1_probes.append((
                "ent/download",
                ent_url,
                common_headers(ua=CONFIGURATOR_ENT_UA, sf=store_front),
                {
                    "creditDisplay": "",
                    "guid": guid,
                    "kbsync": kbsync_b64,
                    "salableAdamId": str(app_id),
                    "serialNumber": serial_b64,
                },
            ))

        for p_name, p_url, p_hdrs, p_dict in step1_probes:
            st, raw, _ = http_send(p_url, method="POST", headers=p_hdrs, body=plistlib.dumps(p_dict))
            try:
                pl = plistlib.loads(raw)
                songs = pl.get("songList", [])
                if songs:
                    meta = songs[0].get("metadata", {})
                    train = [str(x) for x in meta.get("softwareVersionExternalIdentifiers", [])]
                    lat = str(meta.get("softwareVersionExternalIdentifier", "")) or (train[-1] if train else "")
                    if train and not unpinned_first:
                        unpinned_first = train[0]
                    if lat and not unpinned_latest:
                        unpinned_latest = lat
                    pkg_url = songs[0].get("URL", "")
                    info = inspect_remote_ipa_info_plist(pkg_url) if pkg_url else {}
                    plats = info.get("CFBundleSupportedPlatforms", [])
                    log(f"  {p_name} unpinned -> status={st}, latest={lat}, train_len={len(train)}, platforms={plats}, ver={info.get('CFBundleShortVersionString')}")
                    if "AppleTVOS" in plats:
                        log(f"\n[SUCCESS] Unpinned {p_name} response is already AppleTVOS! Latest externalVersionId={lat}")
                        return
                    ios_train_set.update(train)
                    if lat:
                        ios_train_set.add(lat)
                else:
                    log(f"  {p_name} unpinned -> status={st}, empty songList (failureType={pl.get('failureType')}, msg={pl.get('customerMessage')})")
            except Exception as e:
                log(f"  {p_name} parse error: {e}")

        # --------------------------------------------------------------------
        # Step 2: Extended DAAP Purchase History (pd.itunes.apple.com, meta without spaces)
        # --------------------------------------------------------------------
        log("\n--- [Step 2] Extended DAAP Purchase History (pd.itunes.apple.com) ---")
        all_query = (
            "('com.apple.itunes.extended\\-media\\-kind:131072',"
            "'com.apple.itunes.extended\\-media\\-kind:262144',"
            "'com.apple.itunes.extended\\-media\\-kind:33554432',"
            "'com.apple.itunes.extended\\-media\\-kind:67108864')"
        )
        tv_query = (
            "('com.apple.itunes.extended\\-media\\-kind:32',"
            "'com.apple.itunes.extended\\-media\\-kind:536870912',"
            "'com.apple.itunes.extended\\-media\\-kind:33554432',"
            "'com.apple.itunes.extended\\-media\\-kind:131072')"
        )
        single_query = f"('com.apple.itunes.itms\\-songid:{app_id}')"

        def daap_headers(sf_val: str, content_type="application/x-dmap-tagged"):
            now_utc = datetime.datetime.now(datetime.timezone.utc)
            h = {
                "Accept": "*/*",
                "Accept-Language": "en-us",
                "User-Agent": CONFIGURATOR_DL_UA,
                "Client-Cloud-DAAP-Request-Reason": "5",
                "Client-Cloud-Daap-Version": "1.3/24D81",
                "Client-Cloud-Purchase-Daap-Version": "1.1/Configurator-2.0",
                "Client-DAAP-Version": "3.12",
                "Date": now_utc.strftime("%a, %d %b %Y %H:%M:%S GMT"),
                "iCloud-DSID": dsid,
                "X-Apple-I-Client-Time": now_utc.strftime("%Y-%m-%dT%H:%M:%SZ"),
                "X-Apple-I-Locale": "en_US",
                "X-Apple-Store-Front": sf_val,
                "X-Dsid": dsid,
                "X-Guid": guid,
                "X-Token": token,
                "X-DAAP-Supported-Features": "16",
            }
            if content_type:
                h["Content-Type"] = content_type
            if cookie_hdr:
                h["Cookie"] = cookie_hdr
            return h

        storefronts_to_test = [f"{sf_base},29", f"{sf_base},34"]
        if sf_numeric != "143441":
            storefronts_to_test.append("143441-1,29")

        d101_url = f"{DAAP_BASE_URL}/databases/101/items?includeApplePubBooks=1&includeItemFlavors=1"

        for sf_val in storefronts_to_test:
            # 2a. Direct databases/101/items (PurchaseHistoryItemsDAAPRequestEncoder.m)
            for d_q_label, d_q in [("all", all_query), ("tv", tv_query), ("single", single_query)]:
                direct_tags = (
                    dmap_string("meta", DAAP_EXTENDED_META_FIELDS)
                    + dmap_uint32("musr", 0)
                    + dmap_uint8("mhid", 1)
                    + dmap_uint8("ajIp", 1)
                    + dmap_string("mque", d_q)
                    + dmap_tag("aetl", b"")
                )
                st, raw, _ = http_send(d101_url, method="POST", headers=daap_headers(sf_val), body=dmap_tag("adsr", direct_tags))
                mstt = first_dmap_uint(raw, "mstt") if st == 200 else None
                items = parse_dmap_mlit_items(raw) if st == 200 else []
                matched = [it for it in items if it.get("aeSI") == app_id or it.get("miid") == app_id or it.get("mild") == app_id]
                if d_q_label == "all" or matched or len(items) > 0:
                    log(f"  [{sf_val}] direct db101 ({d_q_label}, m=True) -> HTTP {st}, mstt={mstt}, total_items={len(items)}, matched={len(matched)}")
                for m_it in matched:
                    log(f"    -> MATCH in direct db101 ({d_q_label}): {json.dumps(m_it, ensure_ascii=False)}")
                    for s_val in m_it.get("_strings", {}).values():
                        for cid in extract_version_ids_from_text(s_val):
                            if cid not in candidates:
                                candidates.append(cid)

            # 2b. Session /login (empty body) -> /update (form-urlencoded) -> db101 & dbRev
            st_l, raw_l, _ = http_send(f"{DAAP_BASE_URL}/login", method="POST", headers=daap_headers(sf_val, content_type=None), body=b"")
            mlid = first_dmap_uint(raw_l, "mlid") if st_l == 200 else None
            if not mlid:
                log(f"  [{sf_val}] /login failed (HTTP {st_l})")
                continue

            for q_label, q_str in [("all", all_query), ("tv", tv_query), ("single", single_query)]:
                upd_body = f"session-id={mlid}&revision-number=(null)&query={q_str}".encode("utf-8")
                st_u, raw_u, _ = http_send(
                    f"{DAAP_BASE_URL}/update",
                    method="POST",
                    headers=daap_headers(sf_val, content_type="application/x-www-form-urlencoded"),
                    body=upd_body,
                )
                rev = first_dmap_uint(raw_u, "musr") if st_u == 200 else None
                if not rev:
                    continue

                for db_label, db_url, local_v, delta_v in [
                    ("db101", d101_url, 0, rev),
                    ("dbRev", f"{DAAP_BASE_URL}/databases/{rev}/items?includeApplePubBooks=1&includeItemFlavors=1", rev, 0),
                ]:
                    for inc_meta in (True, False):
                        req_tags = dmap_uint32("mstc", int(time.time()))
                        if inc_meta:
                            req_tags += dmap_string("meta", DAAP_EXTENDED_META_FIELDS)
                        req_tags += (
                            dmap_string("mque", q_str)
                            + dmap_uint32("mlid", mlid)
                            + dmap_uint8("mikd", 2)
                            + dmap_uint32("musr", local_v)
                            + dmap_uint32("mder", delta_v)
                            + dmap_uint8("mhid", 1)
                            + dmap_uint8("ajIp", 1)
                            + dmap_tag("aetl", b"")
                        )
                        st_i, raw_i, _ = http_send(db_url, method="POST", headers=daap_headers(sf_val), body=dmap_tag("adsr", req_tags))
                        mstt_i = first_dmap_uint(raw_i, "mstt") if st_i == 200 else None
                        items_i = parse_dmap_mlit_items(raw_i) if st_i == 200 else []
                        hid_count = sum(1 for it in items_i if it.get("mihi") or it.get("mihd") or it.get("ajhj"))
                        matched_i = [
                            it for it in items_i
                            if it.get("aeSI") == app_id or it.get("miid") == app_id or it.get("mild") == app_id
                            or (q_label == "single" and len(items_i) == 1)
                        ]
                        if q_label == "all" or matched_i:
                            log(
                                f"  [{sf_val}] {db_label} ({q_label}, m={inc_meta}) -> HTTP {st_i}, mstt={mstt_i}, "
                                f"items={len(items_i)}, hidden={hid_count}, matched={len(matched_i)}"
                            )
                        for m_it in matched_i:
                            log(f"    -> MATCH ({db_label}, {q_label}, m={inc_meta}): {json.dumps(m_it, ensure_ascii=False)}")
                            for s_val in m_it.get("_strings", {}).values():
                                for cid in extract_version_ids_from_text(s_val):
                                    if cid not in candidates:
                                        candidates.append(cid)
                            for fl in m_it.get("_flavors", []):
                                for s_val in fl.get("_strings", {}).values():
                                    for cid in extract_version_ids_from_text(s_val):
                                        if cid not in candidates:
                                            candidates.append(cid)

        # --------------------------------------------------------------------
        # Step 3: MZSoftwareUpdate (availableSoftwareUpdatesExtended / Standard)
        # --------------------------------------------------------------------
        log("\n--- [Step 3] MZSoftwareUpdate Probes ---")
        if kbsync_b64:
            local_sw = [
                {"adam-id": app_id, "installed-version-identifier": 0},
                {"adam-id": app_id, "installed-version-identifier": 1},
            ]
            if unpinned_first.isdigit():
                local_sw.append({"adam-id": app_id, "installed-version-identifier": int(unpinned_first)})
            if unpinned_latest.isdigit():
                local_sw.append({"adam-id": app_id, "installed-version-identifier": int(unpinned_latest)})

            for su_name, su_url in [
                ("suExt", "https://su.itunes.apple.com/WebObjects/MZSoftwareUpdate.woa/wa/availableSoftwareUpdatesExtended"),
                ("suStd", "https://su.itunes.apple.com/WebObjects/MZSoftwareUpdate.woa/wa/availableSoftwareUpdates"),
            ]:
                su_body = plistlib.dumps({
                    "autoUpdatesEnabled": True,
                    "guid": guid,
                    "kbsync": kbsync_b64,
                    "local-software": local_sw,
                })
                st_su, raw_su, _ = http_send(
                    su_url,
                    method="POST",
                    headers=common_headers(ua=CONFIGURATOR_ENT_UA, sf=f"{sf_base},29", ta_device=TVOS_TA_DEVICES),
                    body=su_body,
                )
                txt_su = raw_su.decode("utf-8", errors="replace")
                found_su = extract_version_ids_from_text(txt_su)
                non_ios_su = [x for x in found_su if x not in ios_train_set]
                log(f"  {su_name} -> HTTP {st_su}, bytes={len(raw_su)}, found_ids={found_su}, non_ios={non_ios_su}")
                if len(txt_su) < 4000:
                    log(f"    raw {su_name}: {txt_su[:1000]}")
                for cid in non_ios_su:
                    if cid not in candidates:
                        candidates.append(cid)
        else:
            log("  Skipped (no cached kbsync-v1 found in keychain)")

        # --------------------------------------------------------------------
        # Step 4: Test Candidates via Pinned Download + Remote Info.plist Check
        # --------------------------------------------------------------------
        log(f"\n--- [Step 4] Testing Candidates: {candidates} ---")
        for cand in candidates:
            if cand in ios_train_set:
                log(f"  Candidate {cand}: skipped (already in iPhoneOS train)")
                continue

            cand_probes = [
                (
                    "volumeStore",
                    vs_url,
                    {
                        "Content-Type": "application/x-apple-plist",
                        "User-Agent": CONFIGURATOR_DL_UA,
                        "iCloud-DSID": dsid,
                        "X-Dsid": dsid,
                        **({"Cookie": cookie_hdr} if cookie_hdr else {}),
                    },
                    {
                        "creditDisplay": "",
                        "guid": guid,
                        "salableAdamId": app_id,
                        "externalVersionId": cand,
                        "serialNumber": "0",
                    },
                ),
            ]
            if kbsync_b64:
                cand_probes.append((
                    "ent/download",
                    ent_url,
                    common_headers(ua=CONFIGURATOR_ENT_UA, sf=store_front),
                    {
                        "creditDisplay": "",
                        "guid": guid,
                        "kbsync": kbsync_b64,
                        "salableAdamId": str(app_id),
                        "externalVersionId": cand,
                        "serialNumber": serial_b64,
                    },
                ))

            for cp_name, cp_url, cp_hdrs, cp_dict in cand_probes:
                st_c, raw_c, _ = http_send(cp_url, method="POST", headers=cp_hdrs, body=plistlib.dumps(cp_dict))
                try:
                    pl_c = plistlib.loads(raw_c)
                    songs_c = pl_c.get("songList", [])
                    if not songs_c:
                        log(f"  Candidate {cand} ({cp_name}) -> empty songList (failureType={pl_c.get('failureType')}, msg={pl_c.get('customerMessage')})")
                        continue
                    meta_c = songs_c[0].get("metadata", {})
                    train_c = [str(x) for x in meta_c.get("softwareVersionExternalIdentifiers", [])]
                    lat_c = train_c[-1] if train_c else str(meta_c.get("softwareVersionExternalIdentifier", cand))
                    pkg_url_c = songs_c[0].get("URL", "")
                    info_c = inspect_remote_ipa_info_plist(pkg_url_c) if pkg_url_c else {}
                    plats_c = info_c.get("CFBundleSupportedPlatforms", [])
                    log(f"  Candidate {cand} ({cp_name}) -> platforms={plats_c}, shortVer={info_c.get('CFBundleShortVersionString')}, train_len={len(train_c)}, latest={lat_c}")
                    if "AppleTVOS" in plats_c:
                        log(f"\n[SUCCESS] Resolved tvOS train via candidate {cand} ({cp_name})!")
                        log(f"  tvOS version train ({len(train_c)} versions): {train_c}")
                        log(f"  Latest tvOS externalVersionId: {lat_c}")
                        return
                except Exception as e:
                    log(f"  Candidate {cand} ({cp_name}) error: {e}")

        log("\n[RESULT] No tvOS externalVersionId resolved yet. Check the Step 2 / Step 3 output above!")
    finally:
        log(f"\n[SAVED] Full output saved to: {out_path.resolve()}")
        tee.close()


if __name__ == "__main__":
    main()
