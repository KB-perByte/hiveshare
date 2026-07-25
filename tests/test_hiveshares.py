"""Integration tests for hiveshare CRUD, members, and invitations."""

import requests

from conftest import auth_header

TIMEOUT = 10


class TestHiveshareCRUD:
    def test_create_hiveshare(self, api_url, user_a):
        resp = requests.post(f"{api_url}/hiveshares", timeout=TIMEOUT, json={
            "name": "CRUD Test",
            "description": "Testing create",
        }, headers=auth_header(user_a))
        assert resp.status_code == 201
        data = resp.json()
        assert data["name"] == "CRUD Test"
        assert data["role"] == "all"
        assert data["member_count"] == 1

    def test_list_hiveshares(self, api_url, user_a, hiveshare_id):
        resp = requests.get(
            f"{api_url}/hiveshares",
            headers=auth_header(user_a), timeout=TIMEOUT,
        )
        assert resp.status_code == 200
        ids = [hs["id"] for hs in resp.json()]
        assert hiveshare_id in ids

    def test_get_hiveshare(self, api_url, user_a, hiveshare_id):
        resp = requests.get(
            f"{api_url}/hiveshares/{hiveshare_id}",
            headers=auth_header(user_a), timeout=TIMEOUT,
        )
        assert resp.status_code == 200
        assert resp.json()["id"] == hiveshare_id

    def test_get_hiveshare_non_member_404(self, api_url, user_b, hiveshare_id):
        resp = requests.get(
            f"{api_url}/hiveshares/{hiveshare_id}",
            headers=auth_header(user_b), timeout=TIMEOUT,
        )
        assert resp.status_code == 404

    def test_update_hiveshare(self, api_url, user_a, hiveshare_id):
        resp = requests.put(
            f"{api_url}/hiveshares/{hiveshare_id}",
            json={"name": "Updated Name", "description": "Updated"},
            headers=auth_header(user_a), timeout=TIMEOUT,
        )
        assert resp.status_code == 200
        assert resp.json()["name"] == "Updated Name"

    def test_delete_hiveshare(self, api_url, user_a):
        create_resp = requests.post(f"{api_url}/hiveshares", timeout=TIMEOUT, json={
            "name": "To Delete",
        }, headers=auth_header(user_a))
        hs_id = create_resp.json()["id"]

        del_resp = requests.delete(
            f"{api_url}/hiveshares/{hs_id}",
            headers=auth_header(user_a), timeout=TIMEOUT,
        )
        assert del_resp.status_code == 204


class TestMembers:
    def test_list_members(self, api_url, user_a, hiveshare_id):
        resp = requests.get(
            f"{api_url}/hiveshares/{hiveshare_id}/members",
            headers=auth_header(user_a), timeout=TIMEOUT,
        )
        assert resp.status_code == 200
        assert len(resp.json()) >= 1


class TestInvitations:
    def test_invite_and_accept(self, api_url, user_a, user_b, hiveshare_id):
        invite_resp = requests.post(
            f"{api_url}/hiveshares/{hiveshare_id}/invite",
            json={"email": user_b["email"], "role": "view"},
            headers=auth_header(user_a), timeout=TIMEOUT,
        )
        assert invite_resp.status_code == 201
        token = invite_resp.json()["token"]

        accept_resp = requests.post(
            f"{api_url}/invitations/{token}/accept",
            json={}, timeout=TIMEOUT,
        )
        assert accept_resp.status_code == 200

        get_resp = requests.get(
            f"{api_url}/hiveshares/{hiveshare_id}",
            headers=auth_header(user_b), timeout=TIMEOUT,
        )
        assert get_resp.status_code == 200

        members_resp = requests.get(
            f"{api_url}/hiveshares/{hiveshare_id}/members",
            headers=auth_header(user_a), timeout=TIMEOUT,
        )
        assert len(members_resp.json()) >= 2
