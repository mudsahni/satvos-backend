"""Tests for handler module (Lambda entry point) — multi-tenant."""

import json
import os
from unittest.mock import patch

import boto3
import responses
from moto import mock_aws

from ses_invoice_processor.handler import lambda_handler
from ses_invoice_processor.tenant_config import TenantConfigStore

BASE_URL = "https://api.test.satvos.com/api/v1"
BUCKET = "test-ses-emails"
TABLE_NAME = "test-email-processor-tenants"

# Sample valid MIME email with a PDF attachment
VALID_EMAIL_BYTES = (
    b"MIME-Version: 1.0\r\n"
    b"From: sender@example.com\r\n"
    b"To: invoices@passpl.satvos.com\r\n"
    b"Subject: INVOICES: Test Company\r\n"
    b"Message-ID: <test-msg-001@example.com>\r\n"
    b"Content-Type: multipart/mixed; boundary=\"boundary123\"\r\n"
    b"\r\n"
    b"--boundary123\r\n"
    b"Content-Type: text/plain\r\n"
    b"\r\n"
    b"See attached invoice.\r\n"
    b"--boundary123\r\n"
    b"Content-Type: application/pdf\r\n"
    b"Content-Disposition: attachment; filename=\"invoice.pdf\"\r\n"
    b"Content-Transfer-Encoding: base64\r\n"
    b"\r\n"
    b"JVBERi0xLjQK\r\n"
    b"--boundary123--\r\n"
)

BAD_SUBJECT_EMAIL_BYTES = (
    b"From: sender@example.com\r\n"
    b"Subject: FWD: Some invoice\r\n"
    b"Message-ID: <bad-subject@example.com>\r\n"
    b"Content-Type: text/plain\r\n"
    b"\r\n"
    b"body\r\n"
)

NO_ATTACHMENTS_EMAIL_BYTES = (
    b"From: sender@example.com\r\n"
    b"Subject: INVOICES: No Files Corp\r\n"
    b"Message-ID: <no-attach@example.com>\r\n"
    b"Content-Type: text/plain\r\n"
    b"\r\n"
    b"No attachments here.\r\n"
)


def _env_vars():
    return {
        "SATVOS_API_BASE_URL": BASE_URL,
        "SES_EMAIL_BUCKET": BUCKET,
        "DYNAMODB_TABLE_NAME": TABLE_NAME,
        "LOG_LEVEL": "DEBUG",
        "AWS_DEFAULT_REGION": "us-east-1",
    }


def _ses_event(
    message_id: str = "test-msg-001",
    spam_status: str = "PASS",
    virus_status: str = "PASS",
    recipients: list[str] | None = None,
):
    if recipients is None:
        recipients = ["invoices@passpl.satvos.com"]
    return {
        "Records": [
            {
                "ses": {
                    "mail": {
                        "messageId": message_id,
                        "source": "sender@example.com",
                    },
                    "receipt": {
                        "recipients": recipients,
                        "spamVerdict": {"status": spam_status},
                        "virusVerdict": {"status": virus_status},
                    },
                }
            }
        ]
    }


def _setup_s3(tenant_slug: str, message_id: str, email_bytes: bytes):
    """Create S3 bucket and put the email object at ses-inbound/<tenant>/<messageId>."""
    s3 = boto3.client("s3", region_name="us-east-1")
    s3.create_bucket(Bucket=BUCKET)
    s3_key = f"ses-inbound/{tenant_slug}/{message_id}"
    s3.put_object(Bucket=BUCKET, Key=s3_key, Body=email_bytes)


TEST_API_KEY = "sk_" + "a" * 64


def _setup_dynamodb(tenant_slug="passpl", enabled=True, api_base_url=None):
    """Create DynamoDB table and insert a tenant config."""
    dynamodb = boto3.resource("dynamodb", region_name="us-east-1")
    dynamodb.create_table(
        TableName=TABLE_NAME,
        KeySchema=[{"AttributeName": "tenant_slug", "KeyType": "HASH"}],
        AttributeDefinitions=[{"AttributeName": "tenant_slug", "AttributeType": "S"}],
        BillingMode="PAY_PER_REQUEST",
    )
    item = {
        "tenant_slug": tenant_slug,
        "service_api_key": TEST_API_KEY,
        "enabled": enabled,
    }
    if api_base_url:
        item["api_base_url"] = api_base_url
    dynamodb.Table(TABLE_NAME).put_item(Item=item)
    return dynamodb


def _mock_api_calls(collection_id: str = "coll-1"):
    """Register standard mocked API responses (no login — API key auth)."""
    # Create collection
    responses.add(
        responses.POST,
        f"{BASE_URL}/collections",
        json={"success": True, "data": {"id": collection_id}},
        status=201,
    )
    # Batch upload
    responses.add(
        responses.POST,
        f"{BASE_URL}/collections/{collection_id}/files",
        json={
            "success": True,
            "data": [
                {"success": True, "file": {"id": "file-1", "original_name": "invoice.pdf"}, "error": None},
            ],
        },
        status=201,
    )
    # Create document
    responses.add(
        responses.POST,
        f"{BASE_URL}/documents",
        json={"success": True, "data": {"id": "doc-1", "parsing_status": "pending"}},
        status=201,
    )


class TestLambdaHandler:
    def setup_method(self):
        TenantConfigStore.clear_cache()

    @mock_aws
    @responses.activate
    def test_happy_path(self):
        _setup_dynamodb("passpl")
        _setup_s3("passpl", "test-msg-001", VALID_EMAIL_BYTES)
        _mock_api_calls()

        with patch.dict(os.environ, _env_vars(), clear=False):
            result = lambda_handler(_ses_event(), None)

        assert result["statusCode"] == 200
        assert "files_uploaded=1" in result["body"]
        assert "documents_created=1" in result["body"]
        assert "tenant=passpl" in result["body"]

    @mock_aws
    @responses.activate
    def test_sender_email_in_collection_description(self):
        _setup_dynamodb("passpl")
        _setup_s3("passpl", "test-msg-001", VALID_EMAIL_BYTES)
        _mock_api_calls()

        with patch.dict(os.environ, _env_vars(), clear=False):
            lambda_handler(_ses_event(), None)

        # Verify the collection creation request includes sender email
        create_coll_call = responses.calls[0]  # create_collection=0 (no login step)
        body = json.loads(create_coll_call.request.body)
        assert "sender@example.com" in body["description"]

    @mock_aws
    def test_spam_rejection(self):
        _setup_dynamodb("passpl")
        _setup_s3("passpl", "spam-msg", VALID_EMAIL_BYTES)

        with patch.dict(os.environ, _env_vars(), clear=False):
            result = lambda_handler(
                _ses_event("spam-msg", spam_status="FAIL"),
                None,
            )

        assert result["statusCode"] == 200
        assert "spamVerdict" in result["body"]

    @mock_aws
    def test_virus_rejection(self):
        _setup_dynamodb("passpl")
        _setup_s3("passpl", "virus-msg", VALID_EMAIL_BYTES)

        with patch.dict(os.environ, _env_vars(), clear=False):
            result = lambda_handler(
                _ses_event("virus-msg", virus_status="FAIL"),
                None,
            )

        assert result["statusCode"] == 200
        assert "virusVerdict" in result["body"]

    @mock_aws
    def test_bad_subject_ignored(self):
        _setup_dynamodb("passpl")
        _setup_s3("passpl", "bad-subj", BAD_SUBJECT_EMAIL_BYTES)

        with patch.dict(os.environ, _env_vars(), clear=False):
            result = lambda_handler(_ses_event("bad-subj"), None)

        assert result["statusCode"] == 200
        assert "subject mismatch" in result["body"]

    @mock_aws
    def test_no_attachments_ignored(self):
        _setup_dynamodb("passpl")
        _setup_s3("passpl", "no-attach", NO_ATTACHMENTS_EMAIL_BYTES)

        with patch.dict(os.environ, _env_vars(), clear=False):
            result = lambda_handler(_ses_event("no-attach"), None)

        assert result["statusCode"] == 200
        assert "no attachments" in result["body"]

    def test_missing_config_returns_200(self):
        """Even config errors return 200 to avoid SES retries."""
        env = {k: v for k, v in _env_vars().items() if k != "SATVOS_API_BASE_URL"}
        with patch.dict(os.environ, env, clear=True):
            result = lambda_handler(_ses_event(), None)

        assert result["statusCode"] == 200
        assert "Configuration error" in result["body"]

    @mock_aws
    @responses.activate
    def test_api_failure_returns_200(self):
        _setup_dynamodb("passpl")
        _setup_s3("passpl", "auth-fail", VALID_EMAIL_BYTES)
        # First API call (create collection) returns 401 — simulates invalid API key
        responses.add(
            responses.POST,
            f"{BASE_URL}/collections",
            json={"success": False, "error": "unauthorized"},
            status=401,
        )

        with patch.dict(os.environ, _env_vars(), clear=False):
            result = lambda_handler(_ses_event("auth-fail"), None)

        # Should still return 200 (error caught by outer handler)
        assert result["statusCode"] == 200

    def test_unknown_tenant_recipient(self):
        """Recipients that don't match invoices@<tenant>.satvos.com are ignored."""
        with patch.dict(os.environ, _env_vars(), clear=False):
            result = lambda_handler(
                _ses_event(recipients=["random@other.com"]),
                None,
            )

        assert result["statusCode"] == 200
        assert "no matching tenant" in result["body"]

    @mock_aws
    def test_tenant_not_in_dynamodb(self):
        """Tenant extracted from email but not configured in DynamoDB."""
        # Create table but don't insert any tenant
        dynamodb = boto3.resource("dynamodb", region_name="us-east-1")
        dynamodb.create_table(
            TableName=TABLE_NAME,
            KeySchema=[{"AttributeName": "tenant_slug", "KeyType": "HASH"}],
            AttributeDefinitions=[{"AttributeName": "tenant_slug", "AttributeType": "S"}],
            BillingMode="PAY_PER_REQUEST",
        )

        with patch.dict(os.environ, _env_vars(), clear=False):
            result = lambda_handler(
                _ses_event(recipients=["invoices@unknown-co.satvos.com"]),
                None,
            )

        assert result["statusCode"] == 200
        assert "not configured" in result["body"]

    @mock_aws
    def test_disabled_tenant(self):
        """Disabled tenants are rejected gracefully."""
        _setup_dynamodb("disabled-co", enabled=False)

        with patch.dict(os.environ, _env_vars(), clear=False):
            result = lambda_handler(
                _ses_event(recipients=["invoices@disabled-co.satvos.com"]),
                None,
            )

        assert result["statusCode"] == 200
        assert "disabled" in result["body"]

    @mock_aws
    @responses.activate
    def test_two_tenants(self):
        """Two different tenants can be processed independently."""
        dynamodb = _setup_dynamodb("tenant-a")
        # Add second tenant to same table
        dynamodb.Table(TABLE_NAME).put_item(Item={
            "tenant_slug": "tenant-b",
            "service_api_key": TEST_API_KEY,
            "enabled": True,
        })

        _setup_s3("tenant-a", "msg-a", VALID_EMAIL_BYTES)
        _mock_api_calls()

        with patch.dict(os.environ, _env_vars(), clear=False):
            result = lambda_handler(
                _ses_event("msg-a", recipients=["invoices@tenant-a.satvos.com"]),
                None,
            )

        assert result["statusCode"] == 200
        assert "tenant=tenant-a" in result["body"]

    @mock_aws
    @responses.activate
    def test_tenant_custom_api_url(self):
        """Tenant with custom api_base_url uses it instead of default."""
        custom_url = "https://custom.api.com/api/v1"
        _setup_dynamodb("custom-co", api_base_url=custom_url)
        _setup_s3("custom-co", "msg-custom", VALID_EMAIL_BYTES)

        # Mock API calls at the custom URL (no login — API key auth)
        responses.add(
            responses.POST,
            f"{custom_url}/collections",
            json={"success": True, "data": {"id": "coll-c"}},
            status=201,
        )
        responses.add(
            responses.POST,
            f"{custom_url}/collections/coll-c/files",
            json={
                "success": True,
                "data": [
                    {"success": True, "file": {"id": "file-c", "original_name": "invoice.pdf"}, "error": None},
                ],
            },
            status=201,
        )
        responses.add(
            responses.POST,
            f"{custom_url}/documents",
            json={"success": True, "data": {"id": "doc-c", "parsing_status": "pending"}},
            status=201,
        )

        with patch.dict(os.environ, _env_vars(), clear=False):
            result = lambda_handler(
                _ses_event("msg-custom", recipients=["invoices@custom-co.satvos.com"]),
                None,
            )

        assert result["statusCode"] == 200
        assert "files_uploaded=1" in result["body"]
        # Verify requests went to custom URL, not default
        assert all(custom_url in call.request.url for call in responses.calls)
