"""Tests for satvos_client module."""

import json
from datetime import datetime, timedelta, timezone

import pytest
import responses

from ses_invoice_processor.email_parser import Attachment
from ses_invoice_processor.exceptions import AuthenticationError, SatvosAPIError
from ses_invoice_processor.satvos_client import SatvosClient

BASE_URL = "https://api.test.satvos.com/api/v1"


def _future_expiry(minutes: int = 15) -> str:
    return (datetime.now(timezone.utc) + timedelta(minutes=minutes)).isoformat()


def _past_expiry() -> str:
    return (datetime.now(timezone.utc) - timedelta(minutes=1)).isoformat()


def _mock_login(status: int = 200, access_token: str = "access-tok", expires_at: str | None = None):
    responses.add(
        responses.POST,
        f"{BASE_URL}/auth/login",
        json={
            "success": True,
            "data": {
                "access_token": access_token,
                "refresh_token": "refresh-tok",
                "expires_at": expires_at or _future_expiry(),
            },
        },
        status=status,
    )


def _mock_refresh(status: int = 200, access_token: str = "new-access-tok"):
    responses.add(
        responses.POST,
        f"{BASE_URL}/auth/refresh",
        json={
            "success": True,
            "data": {
                "access_token": access_token,
                "refresh_token": "new-refresh-tok",
                "expires_at": _future_expiry(),
            },
        },
        status=status,
    )


def _make_attachment(filename: str = "test.pdf", content_type: str = "application/pdf", data: bytes = b"%PDF") -> Attachment:
    return Attachment(filename=filename, content_type=content_type, data=data, extension="pdf")


class TestAuthenticate:
    @responses.activate
    def test_login_success(self):
        _mock_login()
        client = SatvosClient(BASE_URL)
        client.authenticate("user@test.com", "password123")
        assert client._token is not None
        assert client._token.access_token == "access-tok"

    @responses.activate
    def test_login_failure(self):
        responses.add(
            responses.POST,
            f"{BASE_URL}/auth/login",
            json={"success": False, "error": "invalid credentials"},
            status=401,
        )
        client = SatvosClient(BASE_URL)
        with pytest.raises(AuthenticationError) as exc_info:
            client.authenticate("bad@test.com", "wrong")
        assert exc_info.value.status_code == 401


class TestTokenRefresh:
    @responses.activate
    def test_refresh_when_token_expired(self):
        _mock_login(expires_at=_past_expiry())
        _mock_refresh()
        responses.add(
            responses.POST,
            f"{BASE_URL}/collections",
            json={"success": True, "data": {"id": "coll-123"}},
            status=201,
        )

        client = SatvosClient(BASE_URL)
        client.authenticate("user@test.com", "password123")
        # Token is already expired, so create_collection should trigger refresh
        client.create_collection("Test", "desc")

        # Verify refresh was called
        assert len(responses.calls) == 3  # login + refresh + create_collection
        assert "/auth/refresh" in responses.calls[1].request.url

    @responses.activate
    def test_no_refresh_when_token_valid(self):
        _mock_login()
        responses.add(
            responses.POST,
            f"{BASE_URL}/collections",
            json={"success": True, "data": {"id": "coll-123"}},
            status=201,
        )

        client = SatvosClient(BASE_URL)
        client.authenticate("user@test.com", "password123")
        client.create_collection("Test", "desc")

        # Only login + create_collection, no refresh
        assert len(responses.calls) == 2

    @responses.activate
    def test_refresh_failure_raises(self):
        _mock_login(expires_at=_past_expiry())
        responses.add(
            responses.POST,
            f"{BASE_URL}/auth/refresh",
            json={"success": False, "error": "refresh token expired"},
            status=401,
        )

        client = SatvosClient(BASE_URL)
        client.authenticate("user@test.com", "password123")
        with pytest.raises(AuthenticationError):
            client.create_collection("Test", "desc")


class TestCreateCollection:
    @responses.activate
    def test_success(self):
        _mock_login()
        responses.add(
            responses.POST,
            f"{BASE_URL}/collections",
            json={"success": True, "data": {"id": "coll-abc"}},
            status=201,
        )

        client = SatvosClient(BASE_URL)
        client.authenticate("user@test.com", "password123")
        coll_id = client.create_collection("Test Collection", "A test")
        assert coll_id == "coll-abc"

    @responses.activate
    def test_success_with_owner_email(self):
        _mock_login()
        responses.add(
            responses.POST,
            f"{BASE_URL}/collections",
            json={"success": True, "data": {"id": "coll-own"}},
            status=201,
        )

        client = SatvosClient(BASE_URL)
        client.authenticate("user@test.com", "password123")
        coll_id = client.create_collection("Test", "desc", owner_email="owner@co.com")
        assert coll_id == "coll-own"

        # Verify owner_email was sent in the request body
        body = json.loads(responses.calls[1].request.body)
        assert body["owner_email"] == "owner@co.com"

    @responses.activate
    def test_no_owner_email_omits_field(self):
        _mock_login()
        responses.add(
            responses.POST,
            f"{BASE_URL}/collections",
            json={"success": True, "data": {"id": "coll-no"}},
            status=201,
        )

        client = SatvosClient(BASE_URL)
        client.authenticate("user@test.com", "password123")
        client.create_collection("Test", "desc")

        # Verify owner_email is NOT in the request body when not provided
        body = json.loads(responses.calls[1].request.body)
        assert "owner_email" not in body

    @responses.activate
    def test_failure(self):
        _mock_login()
        responses.add(
            responses.POST,
            f"{BASE_URL}/collections",
            json={"success": False, "error": "server error"},
            status=500,
        )

        client = SatvosClient(BASE_URL)
        client.authenticate("user@test.com", "password123")
        with pytest.raises(SatvosAPIError) as exc_info:
            client.create_collection("Test", "desc")
        assert exc_info.value.status_code == 500


class TestBatchUploadFiles:
    @responses.activate
    def test_all_success(self):
        _mock_login()
        responses.add(
            responses.POST,
            f"{BASE_URL}/collections/coll-1/files",
            json={
                "success": True,
                "data": [
                    {"success": True, "file": {"id": "f1", "original_name": "inv.pdf"}, "error": None},
                ],
            },
            status=201,
        )

        client = SatvosClient(BASE_URL)
        client.authenticate("user@test.com", "password123")
        results = client.batch_upload_files("coll-1", [_make_attachment()])
        assert len(results) == 1
        assert results[0]["success"] is True

    @responses.activate
    def test_partial_success(self):
        _mock_login()
        responses.add(
            responses.POST,
            f"{BASE_URL}/collections/coll-1/files",
            json={
                "success": True,
                "data": [
                    {"success": True, "file": {"id": "f1", "original_name": "a.pdf"}, "error": None},
                    {"success": False, "file": {"original_name": "b.pdf"}, "error": "too large"},
                ],
            },
            status=207,
        )

        client = SatvosClient(BASE_URL)
        client.authenticate("user@test.com", "password123")
        results = client.batch_upload_files("coll-1", [_make_attachment("a.pdf"), _make_attachment("b.pdf")])
        assert len(results) == 2
        assert results[0]["success"] is True
        assert results[1]["success"] is False

    @responses.activate
    def test_total_failure(self):
        _mock_login()
        responses.add(
            responses.POST,
            f"{BASE_URL}/collections/coll-1/files",
            json={"success": False, "error": "server error"},
            status=500,
        )

        client = SatvosClient(BASE_URL)
        client.authenticate("user@test.com", "password123")
        with pytest.raises(SatvosAPIError):
            client.batch_upload_files("coll-1", [_make_attachment()])


class TestCreateDocument:
    @responses.activate
    def test_success(self):
        _mock_login()
        responses.add(
            responses.POST,
            f"{BASE_URL}/documents",
            json={"success": True, "data": {"id": "doc-xyz", "parsing_status": "pending"}},
            status=201,
        )

        client = SatvosClient(BASE_URL)
        client.authenticate("user@test.com", "password123")
        doc_id = client.create_document("file-1", "coll-1")
        assert doc_id == "doc-xyz"

        # Verify request body
        body = json.loads(responses.calls[1].request.body)
        assert body["file_id"] == "file-1"
        assert body["collection_id"] == "coll-1"
        assert body["document_type"] == "invoice"
        assert body["parse_mode"] == "single"


class TestProcessAttachments:
    @responses.activate
    def test_full_pipeline(self):
        _mock_login()
        # Create collection
        responses.add(
            responses.POST,
            f"{BASE_URL}/collections",
            json={"success": True, "data": {"id": "coll-new"}},
            status=201,
        )
        # Batch upload
        responses.add(
            responses.POST,
            f"{BASE_URL}/collections/coll-new/files",
            json={
                "success": True,
                "data": [
                    {"success": True, "file": {"id": "f1", "original_name": "inv1.pdf"}, "error": None},
                    {"success": True, "file": {"id": "f2", "original_name": "inv2.pdf"}, "error": None},
                ],
            },
            status=201,
        )
        # Create documents (one per file)
        responses.add(
            responses.POST,
            f"{BASE_URL}/documents",
            json={"success": True, "data": {"id": "d1", "parsing_status": "pending"}},
            status=201,
        )
        responses.add(
            responses.POST,
            f"{BASE_URL}/documents",
            json={"success": True, "data": {"id": "d2", "parsing_status": "pending"}},
            status=201,
        )

        client = SatvosClient(BASE_URL)
        client.authenticate("user@test.com", "password123")
        result = client.process_attachments(
            "Acme Corp",
            [_make_attachment("inv1.pdf"), _make_attachment("inv2.pdf")],
            sender_email="billing@acme.com",
        )

        assert result.collection_id == "coll-new"
        assert "Acme Corp" in result.collection_name
        assert result.files_uploaded == 2
        assert result.files_failed == []
        assert result.documents_created == 2
        assert result.documents_failed == []

    @responses.activate
    def test_sender_email_in_collection_description_and_owner_email(self):
        _mock_login()
        responses.add(
            responses.POST,
            f"{BASE_URL}/collections",
            json={"success": True, "data": {"id": "coll-s"}},
            status=201,
        )
        responses.add(
            responses.POST,
            f"{BASE_URL}/collections/coll-s/files",
            json={
                "success": True,
                "data": [
                    {"success": True, "file": {"id": "f1", "original_name": "inv.pdf"}, "error": None},
                ],
            },
            status=201,
        )
        responses.add(
            responses.POST,
            f"{BASE_URL}/documents",
            json={"success": True, "data": {"id": "d1"}},
            status=201,
        )

        client = SatvosClient(BASE_URL)
        client.authenticate("user@test.com", "password123")
        client.process_attachments("Test Co", [_make_attachment()], sender_email="sender@test.com")

        # Verify collection description includes sender and owner_email is passed
        create_coll_call = responses.calls[1]  # login=0, create_collection=1
        body = json.loads(create_coll_call.request.body)
        assert "sender@test.com" in body["description"]
        assert body["owner_email"] == "sender@test.com"

    @responses.activate
    def test_no_sender_email_fallback(self):
        _mock_login()
        responses.add(
            responses.POST,
            f"{BASE_URL}/collections",
            json={"success": True, "data": {"id": "coll-f"}},
            status=201,
        )
        responses.add(
            responses.POST,
            f"{BASE_URL}/collections/coll-f/files",
            json={
                "success": True,
                "data": [
                    {"success": True, "file": {"id": "f1", "original_name": "inv.pdf"}, "error": None},
                ],
            },
            status=201,
        )
        responses.add(
            responses.POST,
            f"{BASE_URL}/documents",
            json={"success": True, "data": {"id": "d1"}},
            status=201,
        )

        client = SatvosClient(BASE_URL)
        client.authenticate("user@test.com", "password123")
        client.process_attachments("Test Co", [_make_attachment()])

        # Verify fallback description (no sender_email) and no owner_email
        create_coll_call = responses.calls[1]
        body = json.loads(create_coll_call.request.body)
        assert "Auto-imported from email for Test Co" in body["description"]
        assert "owner_email" not in body

    @responses.activate
    def test_partial_upload_failure(self):
        _mock_login()
        responses.add(
            responses.POST,
            f"{BASE_URL}/collections",
            json={"success": True, "data": {"id": "coll-p"}},
            status=201,
        )
        responses.add(
            responses.POST,
            f"{BASE_URL}/collections/coll-p/files",
            json={
                "success": True,
                "data": [
                    {"success": True, "file": {"id": "f1", "original_name": "ok.pdf"}, "error": None},
                    {"success": False, "file": {"original_name": "bad.pdf"}, "error": "unsupported"},
                ],
            },
            status=207,
        )
        responses.add(
            responses.POST,
            f"{BASE_URL}/documents",
            json={"success": True, "data": {"id": "d1"}},
            status=201,
        )

        client = SatvosClient(BASE_URL)
        client.authenticate("user@test.com", "password123")
        result = client.process_attachments(
            "Test Co",
            [_make_attachment("ok.pdf"), _make_attachment("bad.pdf")],
            sender_email="s@t.com",
        )

        assert result.files_uploaded == 1
        assert len(result.files_failed) == 1
        assert result.documents_created == 1

    @responses.activate
    def test_document_creation_failure_continues(self):
        _mock_login()
        responses.add(
            responses.POST,
            f"{BASE_URL}/collections",
            json={"success": True, "data": {"id": "coll-d"}},
            status=201,
        )
        responses.add(
            responses.POST,
            f"{BASE_URL}/collections/coll-d/files",
            json={
                "success": True,
                "data": [
                    {"success": True, "file": {"id": "f1", "original_name": "a.pdf"}, "error": None},
                    {"success": True, "file": {"id": "f2", "original_name": "b.pdf"}, "error": None},
                ],
            },
            status=201,
        )
        # First document fails
        responses.add(
            responses.POST,
            f"{BASE_URL}/documents",
            json={"success": False, "error": "parse error"},
            status=500,
        )
        # Second document succeeds
        responses.add(
            responses.POST,
            f"{BASE_URL}/documents",
            json={"success": True, "data": {"id": "d2"}},
            status=201,
        )

        client = SatvosClient(BASE_URL)
        client.authenticate("user@test.com", "password123")
        result = client.process_attachments(
            "Fail Co",
            [_make_attachment("a.pdf"), _make_attachment("b.pdf")],
            sender_email="s@t.com",
        )

        assert result.files_uploaded == 2
        assert result.documents_created == 1
        assert len(result.documents_failed) == 1

    def test_not_authenticated_raises(self):
        client = SatvosClient(BASE_URL)
        with pytest.raises(AuthenticationError, match="Not authenticated"):
            client.create_collection("Test", "desc")
