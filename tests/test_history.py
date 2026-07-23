"""Integration tests for memory history, snapshots, rollback, and copy.

Run: pytest tests/ -v
Requires: make dev (server + postgres + redis running)
"""

import time
import requests
import pytest

from conftest import auth_header

TIMEOUT = 10


class TestEntryHistory:
    """Per-entry history, rollback, and undelete."""

    def test_create_generates_history(self, api_url, user_a, hiveshare_id, memory_entry):
        resp = requests.get(
            f"{api_url}/hiveshares/{hiveshare_id}/memory/{memory_entry['id']}/history",
            headers=auth_header(user_a), timeout=TIMEOUT,
        )
        resp.raise_for_status()
        versions = resp.json()
        assert len(versions) >= 1
        assert versions[-1]["action"] == "insert"

    def test_update_generates_history(self, api_url, user_a, hiveshare_id, memory_entry):
        requests.put(
            f"{api_url}/hiveshares/{hiveshare_id}/memory/{memory_entry['id']}",
            json={"content": "Updated content", "summary": "Updated", "tags": ["test", "updated"]},
            headers=auth_header(user_a), timeout=TIMEOUT,
        ).raise_for_status()

        resp = requests.get(
            f"{api_url}/hiveshares/{hiveshare_id}/memory/{memory_entry['id']}/history",
            headers=auth_header(user_a), timeout=TIMEOUT,
        )
        resp.raise_for_status()
        versions = resp.json()
        actions = [v["action"] for v in versions]
        assert "update" in actions

    def test_rollback_restores_content(self, api_url, user_a, hiveshare_id, memory_entry):
        resp = requests.get(
            f"{api_url}/hiveshares/{hiveshare_id}/memory/{memory_entry['id']}/history",
            headers=auth_header(user_a), timeout=TIMEOUT,
        )
        resp.raise_for_status()
        versions = resp.json()
        insert_version = [v for v in versions if v["action"] == "insert"][-1]

        resp = requests.post(
            f"{api_url}/hiveshares/{hiveshare_id}/memory/{memory_entry['id']}/rollback",
            json={"history_id": insert_version["history_id"]},
            headers=auth_header(user_a), timeout=TIMEOUT,
        )
        resp.raise_for_status()
        restored = resp.json()
        assert restored["content"] == "Original content for testing history"

    def test_delete_and_undelete(self, api_url, user_a, hiveshare_id):
        create_resp = requests.post(
            f"{api_url}/hiveshares/{hiveshare_id}/memory",
            json={
                "source_type": "manual",
                "source_ref": "delete-test",
                "content": "Entry to be deleted and restored",
                "tool": "manual",
                "tags": [],
            },
            headers=auth_header(user_a), timeout=TIMEOUT,
        )
        create_resp.raise_for_status()
        entry_id = create_resp.json()["id"]

        requests.delete(
            f"{api_url}/hiveshares/{hiveshare_id}/memory/{entry_id}",
            headers=auth_header(user_a), timeout=TIMEOUT,
        ).raise_for_status()

        get_resp = requests.get(
            f"{api_url}/hiveshares/{hiveshare_id}/memory/{entry_id}",
            headers=auth_header(user_a), timeout=TIMEOUT,
        )
        assert get_resp.status_code == 404

        hist_resp = requests.get(
            f"{api_url}/hiveshares/{hiveshare_id}/memory/{entry_id}/history",
            headers=auth_header(user_a), timeout=TIMEOUT,
        )
        hist_resp.raise_for_status()
        versions = hist_resp.json()
        delete_version = [v for v in versions if v["action"] == "delete"][0]

        undelete_resp = requests.post(
            f"{api_url}/hiveshares/{hiveshare_id}/memory/undelete",
            json={"history_id": delete_version["history_id"]},
            headers=auth_header(user_a), timeout=TIMEOUT,
        )
        undelete_resp.raise_for_status()
        assert undelete_resp.status_code == 201
        restored = undelete_resp.json()
        assert restored["content"] == "Entry to be deleted and restored"
        assert restored["id"] == entry_id


class TestSnapshots:
    """Hiveshare-level snapshots and restore-to-new-hiveshare."""

    def test_create_snapshot(self, api_url, user_a, hiveshare_id):
        resp = requests.post(
            f"{api_url}/hiveshares/{hiveshare_id}/snapshots",
            json={"name": "test-snapshot", "description": "Integration test"},
            headers=auth_header(user_a), timeout=TIMEOUT,
        )
        resp.raise_for_status()
        assert resp.status_code == 201
        snap = resp.json()
        assert snap["name"] == "test-snapshot"
        assert snap["entry_count"] >= 1

    def test_list_snapshots(self, api_url, user_a, hiveshare_id):
        resp = requests.get(
            f"{api_url}/hiveshares/{hiveshare_id}/snapshots",
            headers=auth_header(user_a), timeout=TIMEOUT,
        )
        resp.raise_for_status()
        snaps = resp.json()
        assert len(snaps) >= 1

    def test_get_snapshot_detail(self, api_url, user_a, hiveshare_id):
        list_resp = requests.get(
            f"{api_url}/hiveshares/{hiveshare_id}/snapshots",
            headers=auth_header(user_a), timeout=TIMEOUT,
        )
        list_resp.raise_for_status()
        snapshot_id = int(list_resp.json()[0]["snapshot_id"])

        detail_resp = requests.get(
            f"{api_url}/hiveshares/{hiveshare_id}/snapshots/{snapshot_id}",
            headers=auth_header(user_a), timeout=TIMEOUT,
        )
        detail_resp.raise_for_status()
        data = detail_resp.json()
        assert "snapshot" in data
        assert "entries" in data
        assert len(data["entries"]) >= 1

    def test_restore_creates_new_hiveshare(self, api_url, user_a, hiveshare_id):
        list_resp = requests.get(
            f"{api_url}/hiveshares/{hiveshare_id}/snapshots",
            headers=auth_header(user_a), timeout=TIMEOUT,
        )
        list_resp.raise_for_status()
        snapshot_id = list_resp.json()[0]["snapshot_id"]

        restore_resp = requests.post(
            f"{api_url}/hiveshares/{hiveshare_id}/snapshots/{int(snapshot_id)}/restore",
            json={"name": "Restored Hiveshare"},
            headers=auth_header(user_a), timeout=TIMEOUT,
        )
        restore_resp.raise_for_status()
        assert restore_resp.status_code == 201
        result = restore_resp.json()
        assert result["hiveshare"]["name"] == "Restored Hiveshare"
        assert result["hiveshare"]["id"] != hiveshare_id
        assert result["entries_restored"] >= 1

    def test_delete_snapshot(self, api_url, user_a, hiveshare_id):
        create_resp = requests.post(
            f"{api_url}/hiveshares/{hiveshare_id}/snapshots",
            json={"name": "to-delete"},
            headers=auth_header(user_a), timeout=TIMEOUT,
        )
        create_resp.raise_for_status()
        snap_id = int(create_resp.json()["snapshot_id"])

        del_resp = requests.delete(
            f"{api_url}/hiveshares/{hiveshare_id}/snapshots/{snap_id}",
            headers=auth_header(user_a), timeout=TIMEOUT,
        )
        assert del_resp.status_code == 204


class TestCopyEntries:
    """Cross-hiveshare entry copy (rollforward merge)."""

    def test_copy_entry_to_another_hiveshare(self, api_url, user_a, hiveshare_id, memory_entry):
        new_hs_resp = requests.post(
            f"{api_url}/hiveshares",
            json={"name": "Copy Target"},
            headers=auth_header(user_a), timeout=TIMEOUT,
        )
        new_hs_resp.raise_for_status()
        target_id = new_hs_resp.json()["id"]

        copy_resp = requests.post(
            f"{api_url}/hiveshares/{target_id}/memory/copy",
            json={"entry_ids": [memory_entry["id"]]},
            headers=auth_header(user_a), timeout=TIMEOUT,
        )
        copy_resp.raise_for_status()
        assert copy_resp.status_code == 201
        copied = copy_resp.json()
        assert len(copied) == 1
        assert copied[0]["hiveshare_id"] == target_id
        assert copied[0]["content"] == memory_entry.get("content", copied[0]["content"])
