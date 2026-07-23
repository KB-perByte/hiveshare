"""Shared fixtures for HiveShare integration tests.

Requires a running server (make dev) at BASE_URL.
"""

import os
import time
import requests


BASE_URL = os.environ.get("HIVESHARE_TEST_URL", "http://localhost:8080")
API = f"{BASE_URL}/api/v1"

_SESSION_TS = str(int(time.time() * 1000))


def _register(suffix, name):
    email = f"pytest-{suffix}-{_SESSION_TS}@test.local"
    resp = requests.post(f"{API}/auth/register", timeout=10, json={
        "email": email,
        "name": name,
    })
    resp.raise_for_status()
    data = resp.json()
    return {"api_key": data["api_key"], "id": data["id"], "email": email}


import pytest


@pytest.fixture(scope="session")
def api_url():
    return API


@pytest.fixture(scope="session")
def user_a():
    """Register user A with a unique email."""
    return _register("a", "Test User A")


@pytest.fixture(scope="session")
def user_b():
    """Register user B with a unique email."""
    return _register("b", "Test User B")


def auth_header(user):
    return {"Authorization": f"Bearer {user['api_key']}"}


@pytest.fixture(scope="session")
def hiveshare_id(api_url, user_a):
    """Create a hiveshare owned by user A."""
    resp = requests.post(f"{api_url}/hiveshares", timeout=10, json={
        "name": "Test Hiveshare",
        "description": "Integration test hiveshare",
    }, headers=auth_header(user_a))
    resp.raise_for_status()
    return resp.json()["id"]


@pytest.fixture(scope="session")
def memory_entry(api_url, user_a, hiveshare_id):
    """Create a memory entry in the test hiveshare."""
    resp = requests.post(f"{api_url}/hiveshares/{hiveshare_id}/memory", timeout=10, json={
        "source_type": "manual",
        "source_ref": "test-entry-1",
        "content": "Original content for testing history",
        "summary": "Test entry",
        "tool": "manual",
        "tags": ["test"],
    }, headers=auth_header(user_a))
    resp.raise_for_status()
    return resp.json()
