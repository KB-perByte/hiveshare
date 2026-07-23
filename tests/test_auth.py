"""Integration tests for auth endpoints."""

import requests
import time

from conftest import auth_header, API

TIMEOUT = 10


class TestRegister:
    def test_register_returns_api_key(self):
        resp = requests.post(f"{API}/auth/register", timeout=TIMEOUT, json={
            "email": f"reg-{int(time.time())}@test.local",
            "name": "Register Test",
        })
        assert resp.status_code == 201
        data = resp.json()
        assert data["api_key"].startswith("hvs_")
        assert "id" in data

    def test_register_duplicate_email_409(self):
        email = f"dup-{int(time.time())}@test.local"
        requests.post(f"{API}/auth/register", timeout=TIMEOUT, json={
            "email": email, "name": "First",
        }).raise_for_status()

        resp = requests.post(f"{API}/auth/register", timeout=TIMEOUT, json={
            "email": email, "name": "Second",
        })
        assert resp.status_code == 409

    def test_register_missing_fields_400(self):
        resp = requests.post(f"{API}/auth/register", timeout=TIMEOUT, json={
            "email": "no-name@test.local",
        })
        assert resp.status_code == 400


class TestWhoami:
    def test_whoami_returns_user(self, api_url, user_a):
        resp = requests.get(
            f"{api_url}/auth/whoami",
            headers=auth_header(user_a), timeout=TIMEOUT,
        )
        assert resp.status_code == 200
        assert resp.json()["email"] == user_a["email"]
        assert resp.json()["name"] == "Test User A"

    def test_whoami_no_auth_401(self, api_url):
        resp = requests.get(f"{api_url}/auth/whoami", timeout=TIMEOUT)
        assert resp.status_code == 401

    def test_whoami_bad_key_401(self, api_url):
        resp = requests.get(
            f"{api_url}/auth/whoami",
            headers={"Authorization": "Bearer hvs_bogus"}, timeout=TIMEOUT,
        )
        assert resp.status_code == 401
