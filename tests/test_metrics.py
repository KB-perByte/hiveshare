"""Integration tests for metrics endpoints."""

import requests

from conftest import auth_header

TIMEOUT = 10


class TestHiveshareMetrics:
    def test_hiveshare_metrics(self, api_url, user_a, hiveshare_id, hive_entry):
        resp = requests.get(
            f"{api_url}/hiveshares/{hiveshare_id}/metrics",
            headers=auth_header(user_a), timeout=TIMEOUT,
        )
        assert resp.status_code == 200
        data = resp.json()
        assert "hiveshare" in data
        assert "memory" in data
        assert "collaboration" in data
        assert "coverage" in data
        assert "activity" in data
        assert data["memory"]["total_entries"] >= 1


class TestUserMetrics:
    def test_user_metrics(self, api_url, user_a):
        resp = requests.get(
            f"{api_url}/metrics/me",
            headers=auth_header(user_a), timeout=TIMEOUT,
        )
        assert resp.status_code == 200
        data = resp.json()
        assert "total_entries" in data
        assert "total_searches" in data
        assert "hiveshares_owned" in data
