"""Tests for satvos_client module."""

import json

import pytest
import responses

from ses_invoice_processor.email_parser import Attachment
from ses_invoice_processor.exceptions import AuthenticationError, SatvosAPIError
from ses_invoice_processor.satvos_client import SatvosClient

BASE_URL = "https://api.test.satvos.com/api/v1"

# Valid test API key (sk_ + 64 hex chars)
TEST_API_KEY = "sk_" + "a" * 64


def _make_authenticated_client() -> SatvosClient:
    client = SatvosClient(BASE_URL)
    client.authenticate_with_api_key(TEST_API_KEY)
    return client


def _make_attachment(filename: str = "test.pdf", content_type: str = "application/pdf", data: bytes = b"%PDF") -> Attachment:
    return Attachment(filename=filename, content_type=content_type, data=data, extension="pdf")


class TestAuthenticateWithAPIKey:
    def test_success(self):
        client = SatvosClient(BASE_URL)
        client.authenticate_with_api_key(TEST_API_KEY)
        assert client._authenticated is True
        assert client._session.headers["Authorization"] == f"Bearer {TEST_API_KEY}"

    def test_invalid_format_no_prefix(self):
        client = SatvosClient(BASE_URL)
        with pytest.raises(AuthenticationError, match="Invalid API key format"):
            client.authenticate_with_api_key("not_a_valid_key")

    def test_empty_key(self):
        client = SatvosClient(BASE_URL)
        with pytest.raises(AuthenticationError, match="Invalid API key format"):
            client.authenticate_with_api_key("")

    def test_too_short_key(self):
        client = SatvosClient(BASE_URL)
        with pytest.raises(AuthenticationError, match="Invalid API key format"):
            client.authenticate_with_api_key("sk_tooshort")

    def test_not_authenticated_raises(self):
        client = SatvosClient(BASE_URL)
        with pytest.raises(AuthenticationError, match="Not authenticated"):
            client.create_collection("Test", "desc")


class TestCreateCollection:
    @responses.activate
    def test_success(self):
        responses.add(
            responses.POST,
            f"{BASE_URL}/collections",
            json={"success": True, "data": {"id": "coll-abc"}},
            status=201,
        )

        client = _make_authenticated_client()
        coll_id = client.create_collection("Test Collection", "A test")
        assert coll_id == "coll-abc"

    @responses.activate
    def test_failure(self):
        responses.add(
            responses.POST,
            f"{BASE_URL}/collections",
            json={"success": False, "error": "server error"},
            status=500,
        )

        client = _make_authenticated_client()
        with pytest.raises(SatvosAPIError) as exc_info:
            client.create_collection("Test", "desc")
        assert exc_info.value.status_code == 500


    @responses.activate
    def test_owner_email_included_when_provided(self):
        responses.add(
            responses.POST,
            f"{BASE_URL}/collections",
            json={"success": True, "data": {"id": "coll-oe"}},
            status=201,
        )

        client = _make_authenticated_client()
        coll_id = client.create_collection("Test", "desc", owner_email="user@example.com")
        assert coll_id == "coll-oe"

        body = json.loads(responses.calls[0].request.body)
        assert body["owner_email"] == "user@example.com"

    @responses.activate
    def test_owner_email_omitted_when_not_provided(self):
        responses.add(
            responses.POST,
            f"{BASE_URL}/collections",
            json={"success": True, "data": {"id": "coll-no"}},
            status=201,
        )

        client = _make_authenticated_client()
        client.create_collection("Test", "desc")

        body = json.loads(responses.calls[0].request.body)
        assert "owner_email" not in body


class TestBatchUploadFiles:
    @responses.activate
    def test_all_success(self):
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

        client = _make_authenticated_client()
        results = client.batch_upload_files("coll-1", [_make_attachment()])
        assert len(results) == 1
        assert results[0]["success"] is True

    @responses.activate
    def test_partial_success(self):
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

        client = _make_authenticated_client()
        results = client.batch_upload_files("coll-1", [_make_attachment("a.pdf"), _make_attachment("b.pdf")])
        assert len(results) == 2
        assert results[0]["success"] is True
        assert results[1]["success"] is False

    @responses.activate
    def test_total_failure(self):
        responses.add(
            responses.POST,
            f"{BASE_URL}/collections/coll-1/files",
            json={"success": False, "error": "server error"},
            status=500,
        )

        client = _make_authenticated_client()
        with pytest.raises(SatvosAPIError):
            client.batch_upload_files("coll-1", [_make_attachment()])


class TestCreateDocument:
    @responses.activate
    def test_success(self):
        responses.add(
            responses.POST,
            f"{BASE_URL}/documents",
            json={"success": True, "data": {"id": "doc-xyz", "parsing_status": "pending"}},
            status=201,
        )

        client = _make_authenticated_client()
        doc_id = client.create_document("file-1", "coll-1")
        assert doc_id == "doc-xyz"

        # Verify request body
        body = json.loads(responses.calls[0].request.body)
        assert body["file_id"] == "file-1"
        assert body["collection_id"] == "coll-1"
        assert body["document_type"] == "invoice"
        assert body["parse_mode"] == "single"


class TestProcessAttachments:
    @responses.activate
    def test_full_pipeline(self):
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

        client = _make_authenticated_client()
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
    def test_sender_email_in_collection_description(self):
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

        client = _make_authenticated_client()
        client.process_attachments("Test Co", [_make_attachment()], sender_email="sender@test.com")

        # Verify collection description includes sender email
        create_coll_call = responses.calls[0]  # create_collection=0
        body = json.loads(create_coll_call.request.body)
        assert "sender@test.com" in body["description"]
        assert body["owner_email"] == "sender@test.com"

    @responses.activate
    def test_no_sender_email_fallback(self):
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

        client = _make_authenticated_client()
        client.process_attachments("Test Co", [_make_attachment()])

        # Verify fallback description (no sender_email)
        create_coll_call = responses.calls[0]
        body = json.loads(create_coll_call.request.body)
        assert "Auto-imported from email for Test Co" in body["description"]

    @responses.activate
    def test_partial_upload_failure(self):
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

        client = _make_authenticated_client()
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

        client = _make_authenticated_client()
        result = client.process_attachments(
            "Fail Co",
            [_make_attachment("a.pdf"), _make_attachment("b.pdf")],
            sender_email="s@t.com",
        )

        assert result.files_uploaded == 2
        assert result.documents_created == 1
        assert len(result.documents_failed) == 1
