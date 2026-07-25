"""Integration tests for infrastructure endpoints (health, routing, auth enforcement)."""

import requests

from conftest import BASE_URL, API

TIMEOUT = 10


class TestHealth:
    def test_health_returns_200(self):
        resp = requests.get(f"{BASE_URL}/health", timeout=TIMEOUT)
        assert resp.status_code == 200

    def test_health_response_format(self):
        resp = requests.get(f"{BASE_URL}/health", timeout=TIMEOUT)
        data = resp.json()
        assert data["status"] == "ok"
        assert data["db"] == "ok"
        assert data["redis"] == "ok"
        assert "commit" in data
        assert "build_time" in data


class TestRouting:
    def test_unknown_route_returns_404_or_405(self):
        resp = requests.get(f"{API}/nonexistent", timeout=TIMEOUT)
        assert resp.status_code in (404, 405)


class TestAuthEnforcement:
    def test_hiveshares_without_auth_returns_401(self):
        resp = requests.get(f"{API}/hiveshares", timeout=TIMEOUT)
        assert resp.status_code == 401

    def test_hiveshares_invalid_key_returns_401(self):
        resp = requests.get(
            f"{API}/hiveshares",
            headers={"Authorization": "Bearer hvs_invalid"}, timeout=TIMEOUT,
        )
        assert resp.status_code == 401
