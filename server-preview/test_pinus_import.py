import json
import tempfile
import unittest
from concurrent.futures import ThreadPoolExecutor
from pathlib import Path

from pinus_import import TokenStore, issue_owned_awg_link


class TokenTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.now = 1000
        self.store = TokenStore(Path(self.temp.name)/"tokens.sqlite", b"x"*32,
                                "https://example.test", lambda: self.now)
        self.store.initialize()

    def issue(self, profile=1):
        return self.store.issue(1, profile, "Synthetic", "SYNTHETIC-SECRET").split("token=")[1]

    def test_single_claim_across_threads(self):
        token = self.issue()
        with ThreadPoolExecutor(max_workers=8) as pool:
            results = list(pool.map(self.store.claim, [token]*16))
        self.assertEqual(sum(x is not None for x in results), 1)
        value = next(x for x in results if x is not None)
        self.assertEqual(json.loads(value)["config"], "SYNTHETIC-SECRET")

    def test_expired_rotated_and_revoked(self):
        old = self.issue()
        new = self.issue()
        self.assertIsNone(self.store.claim(old))
        self.now += 301
        self.assertIsNone(self.store.claim(new))
        revoked = self.issue()
        self.store.revoke(1)
        self.assertIsNone(self.store.claim(revoked))

    def test_token_and_config_absent_from_database(self):
        token = self.issue()
        with self.store.connect() as db:
            row = db.execute("SELECT digest,payload FROM import_tokens").fetchone()
        self.assertNotIn(token.encode(), b"".join(row))
        self.assertNotIn(b"SYNTHETIC-SECRET", b"".join(row))

    def test_owner_and_authorization_checked_before_read(self):
        profile = {"tg_id": 1, "config_path": "must-never-read", "id": 1}
        for requester, authorized in [(2, True), (1, False)]:
            with self.assertRaises(PermissionError):
                issue_owned_awg_link(self.store, requester, profile, Path(self.temp.name),
                                     lambda _: authorized)

    def test_malformed_and_unknown_tokens(self):
        for token in ["", "a"*43, "../secret", "a"*10000]:
            self.assertIsNone(self.store.claim(token))


if __name__ == "__main__":
    unittest.main()
