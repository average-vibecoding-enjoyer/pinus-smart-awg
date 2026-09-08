"""Optional isolated Pinus import service. Importing this module does no I/O.

Only the bot's authenticated owner flow may issue links. This module does not
import the production bot, modify VPN profiles or run an AWG service.
"""
from __future__ import annotations

import base64
import hashlib
import json
import re
import secrets
import sqlite3
import time
from contextlib import contextmanager
from pathlib import Path
from urllib.parse import urlsplit

from cryptography.hazmat.primitives.ciphers.aead import AESGCM
from cryptography.exceptions import InvalidTag

TOKEN = re.compile(r"[A-Za-z0-9_-]{43}\Z")
MAX_CONFIG = 1024 * 1024


def checked_origin(origin: str) -> str:
    url = urlsplit(origin)
    if (url.scheme != "https" or not url.hostname or url.username or url.password
            or url.port not in (None, 443) or url.path not in ("", "/")
            or url.query or url.fragment):
        raise ValueError("Expected an HTTPS origin")
    return "https://" + url.netloc


class TokenStore:
    def __init__(self, db_path: Path, key: bytes, origin: str, clock=time.time):
        if len(key) != 32:
            raise ValueError("Expected a dedicated 32-byte import encryption key")
        self.path, self.cipher = Path(db_path), AESGCM(key)
        self.origin, self.clock = checked_origin(origin), clock

    def initialize(self):
        self.path.parent.mkdir(parents=True, exist_ok=True, mode=0o700)
        with self.connect() as conn:
            conn.execute("PRAGMA journal_mode=WAL")
            conn.execute("""CREATE TABLE IF NOT EXISTS import_tokens (
                digest BLOB PRIMARY KEY, owner_id INTEGER NOT NULL,
                profile_id INTEGER NOT NULL, expires INTEGER NOT NULL,
                nonce BLOB NOT NULL, payload BLOB NOT NULL)""")
        self.path.chmod(0o600)

    @contextmanager
    def connect(self):
        connection = sqlite3.connect(self.path, timeout=5)
        try:
            with connection:
                yield connection
        finally:
            connection.close()

    def issue(self, owner_id: int, profile_id: int, name: str, config: str,
              ttl: int = 300) -> str:
        if not 1 <= ttl <= 300 or len(config.encode("utf-8")) > MAX_CONFIG:
            raise ValueError("Invalid import size or expiration")
        if not name or len(name) > 32 or any(c in name for c in '\\/:*?"<>|\x00\r\n'):
            raise ValueError("Invalid profile name")
        token = secrets.token_urlsafe(32)
        digest = hashlib.sha256(token.encode("ascii")).digest()
        nonce = secrets.token_bytes(12)
        plain = json.dumps({"version": 1, "name": name, "config": config},
                           ensure_ascii=False, separators=(",", ":")).encode("utf-8")
        encrypted = self.cipher.encrypt(nonce, plain, digest)
        now = int(self.clock())
        with self.connect() as conn:
            conn.execute("DELETE FROM import_tokens WHERE expires <= ?", (now,))
            # Issuing a fresh link invalidates older links to the same profile.
            conn.execute("DELETE FROM import_tokens WHERE owner_id=? AND profile_id=?",
                         (owner_id, profile_id))
            conn.execute("INSERT INTO import_tokens VALUES (?,?,?,?,?,?)",
                         (digest, owner_id, profile_id, now + ttl, nonce, encrypted))
        return self.origin + "/pinus/import#token=" + token

    def revoke(self, owner_id: int, profile_id: int | None = None):
        with self.connect() as conn:
            if profile_id is None:
                conn.execute("DELETE FROM import_tokens WHERE owner_id=?", (owner_id,))
            else:
                conn.execute("DELETE FROM import_tokens WHERE owner_id=? AND profile_id=?",
                             (owner_id, profile_id))

    def claim(self, token: str) -> bytes | None:
        if not TOKEN.fullmatch(token):
            return None
        digest = hashlib.sha256(token.encode("ascii")).digest()
        # A write transaction provides exactly one winner across processes.
        with self.connect() as conn:
            conn.execute("BEGIN IMMEDIATE")
            row = conn.execute("SELECT expires,nonce,payload FROM import_tokens WHERE digest=?",
                               (digest,)).fetchone()
            conn.execute("DELETE FROM import_tokens WHERE digest=?", (digest,))
        if row is None or row[0] <= self.clock():
            return None
        return self.cipher.decrypt(row[1], row[2], digest)


def issue_owned_awg_link(store: TokenStore, requester_id: int, profile,
                         profile_root: Path, is_authorized) -> str:
    """Call in send_profile only after the bot has selected the owned profile."""
    if int(profile["tg_id"]) != int(requester_id) or not is_authorized(requester_id):
        raise PermissionError("Profile owner has no current access")
    root = Path(profile_root).resolve(strict=True)
    path = Path(profile["config_path"]).resolve(strict=True)
    if not path.is_relative_to(root) or path.suffix.lower() != ".conf":
        raise ValueError("Profile is outside the configured profile directory")
    with path.open("rb") as handle:
        raw = handle.read(MAX_CONFIG + 1)
    if len(raw) > MAX_CONFIG:
        raise ValueError("Profile is too large")
    config = raw.decode("utf-8-sig")
    return store.issue(requester_id, int(profile["id"]), str(profile["client_name"]), config)


def make_handler(store: TokenStore):
    from http.server import BaseHTTPRequestHandler

    class Handler(BaseHTTPRequestHandler):
        protocol_version = "HTTP/1.0"

        def setup(self):
            super().setup()
            self.connection.settimeout(5)

        def log_message(self, *args):
            pass  # Never log bearer tokens, paths, profiles or request headers.

        def respond(self, status, body=b""):
            self.send_response(status)
            self.send_header("Content-Type", "application/json; charset=utf-8")
            self.send_header("Cache-Control", "no-store")
            self.send_header("X-Content-Type-Options", "nosniff")
            self.send_header("Content-Length", str(len(body)))
            self.end_headers()
            self.wfile.write(body)

        def do_POST(self):
            if self.path != "/api/pinus/import":
                self.respond(404)
                return
            if self.headers.get("Transfer-Encoding") or self.headers.get("Content-Length", "0") != "0":
                self.respond(400)
                return
            auth = self.headers.get("Authorization", "")
            if not auth.startswith("Bearer ") or len(auth) != 50:
                self.respond(401)
                return
            try:
                payload = store.claim(auth[7:])
            except (sqlite3.Error, ValueError, InvalidTag):
                self.respond(503)
                return
            self.respond(200, payload) if payload is not None else self.respond(410)

        def do_GET(self):
            self.respond(405)

    return Handler


def main():
    import argparse
    import os
    from http.server import ThreadingHTTPServer

    parser = argparse.ArgumentParser()
    parser.add_argument("--db", type=Path, required=True)
    parser.add_argument("--key-file", type=Path, required=True)
    parser.add_argument("--origin", required=True)
    parser.add_argument("--port", type=int, default=9073)
    args = parser.parse_args()
    os.umask(0o077)
    store = TokenStore(args.db, args.key_file.read_bytes(), args.origin)
    store.initialize()
    # Public HTTPS terminates at the separately configured reverse proxy.
    ThreadingHTTPServer(("127.0.0.1", args.port), make_handler(store)).serve_forever()


if __name__ == "__main__":
    main()
