"""Integration tests for memory CRUD and search."""

import requests

from conftest import auth_header, API

TIMEOUT = 10


class TestMemoryCRUD:
    def test_create_entry(self, api_url, user_a, hiveshare_id):
        resp = requests.post(
            f"{api_url}/hiveshares/{hiveshare_id}/hives",
            json={
                "source_type": "jira",
                "source_ref": "TEST-100",
                "content": "Test hive content",
                "summary": "Test summary",
                "tool": "claude",
                "tags": ["test"],
            },
            headers=auth_header(user_a), timeout=TIMEOUT,
        )
        assert resp.status_code == 201
        data = resp.json()
        assert data["source_type"] == "jira"
        assert data["source_ref"] == "TEST-100"
        assert data["tool"] == "claude"

    def test_create_missing_fields_400(self, api_url, user_a, hiveshare_id):
        resp = requests.post(
            f"{api_url}/hiveshares/{hiveshare_id}/hives",
            json={"content": "no source type"},
            headers=auth_header(user_a), timeout=TIMEOUT,
        )
        assert resp.status_code == 400

    def test_list_entries(self, api_url, user_a, hiveshare_id, hive_entry):
        resp = requests.get(
            f"{api_url}/hiveshares/{hiveshare_id}/hives",
            headers=auth_header(user_a), timeout=TIMEOUT,
        )
        assert resp.status_code == 200
        entries = resp.json()
        assert len(entries) >= 1

    def test_list_filter_by_source_type(self, api_url, user_a, hiveshare_id):
        resp = requests.get(
            f"{api_url}/hiveshares/{hiveshare_id}/hives?source_type=manual",
            headers=auth_header(user_a), timeout=TIMEOUT,
        )
        assert resp.status_code == 200
        for e in resp.json():
            assert e["source_type"] == "manual"

    def test_get_entry(self, api_url, user_a, hiveshare_id, hive_entry):
        resp = requests.get(
            f"{api_url}/hiveshares/{hiveshare_id}/hives/{hive_entry['id']}",
            headers=auth_header(user_a), timeout=TIMEOUT,
        )
        assert resp.status_code == 200
        assert resp.json()["id"] == hive_entry["id"]
        assert "content" in resp.json()

    def test_update_entry(self, api_url, user_a, hiveshare_id, hive_entry):
        resp = requests.put(
            f"{api_url}/hiveshares/{hiveshare_id}/hives/{hive_entry['id']}",
            json={"content": "Updated via test", "summary": "Updated", "tags": ["updated"]},
            headers=auth_header(user_a), timeout=TIMEOUT,
        )
        assert resp.status_code == 200
        assert resp.json()["content"] == "Updated via test"

    def test_view_only_cannot_write_403(self, api_url, user_a, user_b, hiveshare_id):
        invite_resp = requests.post(
            f"{api_url}/hiveshares/{hiveshare_id}/invite",
            json={"email": user_b["email"], "role": "view"},
            headers=auth_header(user_a), timeout=TIMEOUT,
        )
        if invite_resp.status_code == 201:
            token = invite_resp.json()["token"]
            requests.post(f"{api_url}/invitations/{token}/accept",
                          json={}, timeout=TIMEOUT)

        resp = requests.post(
            f"{api_url}/hiveshares/{hiveshare_id}/hives",
            json={
                "source_type": "manual",
                "source_ref": "view-write-test",
                "content": "should be rejected",
                "tool": "manual",
            },
            headers=auth_header(user_b), timeout=TIMEOUT,
        )
        assert resp.status_code == 403

    def test_delete_entry(self, api_url, user_a, hiveshare_id):
        create_resp = requests.post(
            f"{api_url}/hiveshares/{hiveshare_id}/hives",
            json={
                "source_type": "manual",
                "source_ref": "to-delete",
                "content": "Will be deleted",
                "tool": "manual",
                "tags": [],
            },
            headers=auth_header(user_a), timeout=TIMEOUT,
        )
        entry_id = create_resp.json()["id"]

        del_resp = requests.delete(
            f"{api_url}/hiveshares/{hiveshare_id}/hives/{entry_id}",
            headers=auth_header(user_a), timeout=TIMEOUT,
        )
        assert del_resp.status_code == 204


class TestSearch:
    def test_search_fulltext(self, api_url, user_a, hiveshare_id):
        requests.post(
            f"{api_url}/hiveshares/{hiveshare_id}/hives",
            json={
                "source_type": "manual",
                "source_ref": "search-target",
                "content": "unique searchable platypus content",
                "tool": "manual",
                "tags": [],
            },
            headers=auth_header(user_a), timeout=TIMEOUT,
        ).raise_for_status()

        resp = requests.post(
            f"{api_url}/hiveshares/{hiveshare_id}/hives/search",
            json={"query": "searchable platypus", "limit": 5},
            headers=auth_header(user_a), timeout=TIMEOUT,
        )
        assert resp.status_code == 200
        data = resp.json()
        assert "results" in data
        assert "count" in data
        assert data["count"] >= 1
        assert "query" in data

    def test_search_missing_query_400(self, api_url, user_a, hiveshare_id):
        resp = requests.post(
            f"{api_url}/hiveshares/{hiveshare_id}/hives/search",
            json={"limit": 5},
            headers=auth_header(user_a), timeout=TIMEOUT,
        )
        assert resp.status_code == 400


