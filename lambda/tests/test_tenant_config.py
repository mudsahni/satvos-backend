"""Tests for tenant_config module."""

import time
from unittest.mock import patch

import boto3
import pytest
from moto import mock_aws

from ses_invoice_processor.exceptions import TenantDisabledError
from ses_invoice_processor.tenant_config import (
    CACHE_TTL_SECONDS,
    TenantConfig,
    TenantConfigStore,
    extract_email_address,
    extract_tenant_slug,
    is_sender_allowed,
)

TABLE_NAME = "satvos-email-processor-tenants"


def _create_table(dynamodb):
    """Create the DynamoDB table for testing."""
    dynamodb.create_table(
        TableName=TABLE_NAME,
        KeySchema=[{"AttributeName": "tenant_slug", "KeyType": "HASH"}],
        AttributeDefinitions=[{"AttributeName": "tenant_slug", "AttributeType": "S"}],
        BillingMode="PAY_PER_REQUEST",
    )


TEST_API_KEY = "sk_" + "a" * 64


def _put_tenant(dynamodb, tenant_slug="passpl", enabled=True, api_base_url=None):
    """Insert a tenant config item."""
    item = {
        "tenant_slug": tenant_slug,
        "service_api_key": TEST_API_KEY,
        "enabled": enabled,
        "created_at": "2025-01-01T00:00:00Z",
        "updated_at": "2025-01-01T00:00:00Z",
    }
    if api_base_url:
        item["api_base_url"] = api_base_url
    dynamodb.Table(TABLE_NAME).put_item(Item=item)


class TestExtractTenantSlug:
    def test_valid_recipient(self):
        assert extract_tenant_slug(["invoices@passpl.satvos.com"]) == "passpl"

    def test_valid_with_hyphens(self):
        assert extract_tenant_slug(["invoices@my-tenant.satvos.com"]) == "my-tenant"

    def test_case_insensitive(self):
        assert extract_tenant_slug(["INVOICES@PassPL.satvos.com"]) == "passpl"

    def test_multiple_recipients_first_match(self):
        result = extract_tenant_slug([
            "admin@other.com",
            "invoices@tenant1.satvos.com",
            "invoices@tenant2.satvos.com",
        ])
        assert result == "tenant1"

    def test_no_match(self):
        assert extract_tenant_slug(["user@other.com"]) is None

    def test_wrong_prefix(self):
        assert extract_tenant_slug(["billing@passpl.satvos.com"]) is None

    def test_wrong_domain(self):
        assert extract_tenant_slug(["invoices@passpl.otherdomain.com"]) is None

    def test_empty_list(self):
        assert extract_tenant_slug([]) is None

    def test_whitespace_trimmed(self):
        assert extract_tenant_slug(["  invoices@passpl.satvos.com  "]) == "passpl"


class TestTenantConfigFromDynamoDBItem:
    def test_full_item(self):
        item = {
            "tenant_slug": "passpl",
            "service_api_key": TEST_API_KEY,
            "enabled": True,
            "allowed_senders": ["@company.com", "user@gmail.com"],
            "api_base_url": "https://custom.api.com/api/v1",
        }
        config = TenantConfig.from_dynamodb_item(item)
        assert config.tenant_slug == "passpl"
        assert config.service_api_key == TEST_API_KEY
        assert config.enabled is True
        assert config.allowed_senders == ("@company.com", "user@gmail.com")
        assert config.api_base_url == "https://custom.api.com/api/v1"

    def test_minimal_item(self):
        item = {
            "tenant_slug": "test",
            "service_api_key": TEST_API_KEY,
        }
        config = TenantConfig.from_dynamodb_item(item)
        assert config.enabled is True  # default
        assert config.allowed_senders == ()  # default — rejects all
        assert config.api_base_url is None  # default

    def test_allowed_senders_from_dynamodb(self):
        item = {
            "tenant_slug": "test",
            "service_api_key": TEST_API_KEY,
            "allowed_senders": ["*"],
        }
        config = TenantConfig.from_dynamodb_item(item)
        assert config.allowed_senders == ("*",)

    def test_empty_api_base_url_becomes_none(self):
        item = {
            "tenant_slug": "test",
            "service_api_key": TEST_API_KEY,
            "api_base_url": "",
        }
        config = TenantConfig.from_dynamodb_item(item)
        assert config.api_base_url is None


class TestTenantConfigStore:
    @mock_aws
    def test_found(self):
        dynamodb = boto3.resource("dynamodb", region_name="us-east-1")
        _create_table(dynamodb)
        _put_tenant(dynamodb, "passpl")

        store = TenantConfigStore(TABLE_NAME, dynamodb_resource=dynamodb)
        store.clear_cache()
        config = store.get("passpl")

        assert config is not None
        assert config.tenant_slug == "passpl"
        assert config.service_api_key == TEST_API_KEY

    @mock_aws
    def test_not_found(self):
        dynamodb = boto3.resource("dynamodb", region_name="us-east-1")
        _create_table(dynamodb)

        store = TenantConfigStore(TABLE_NAME, dynamodb_resource=dynamodb)
        store.clear_cache()
        config = store.get("nonexistent")

        assert config is None

    @mock_aws
    def test_disabled_raises(self):
        dynamodb = boto3.resource("dynamodb", region_name="us-east-1")
        _create_table(dynamodb)
        _put_tenant(dynamodb, "disabled-co", enabled=False)

        store = TenantConfigStore(TABLE_NAME, dynamodb_resource=dynamodb)
        store.clear_cache()

        with pytest.raises(TenantDisabledError, match="disabled-co"):
            store.get("disabled-co")

    @mock_aws
    def test_custom_api_base_url(self):
        dynamodb = boto3.resource("dynamodb", region_name="us-east-1")
        _create_table(dynamodb)
        _put_tenant(dynamodb, "custom", api_base_url="https://custom.api/v1")

        store = TenantConfigStore(TABLE_NAME, dynamodb_resource=dynamodb)
        store.clear_cache()
        config = store.get("custom")

        assert config.api_base_url == "https://custom.api/v1"


class TestTenantConfigStoreCache:
    @mock_aws
    def test_cache_hit(self):
        dynamodb = boto3.resource("dynamodb", region_name="us-east-1")
        _create_table(dynamodb)
        _put_tenant(dynamodb, "cached")

        store = TenantConfigStore(TABLE_NAME, dynamodb_resource=dynamodb)
        store.clear_cache()

        # First call — cache miss
        config1 = store.get("cached")
        # Delete from DynamoDB — cache should still return it
        dynamodb.Table(TABLE_NAME).delete_item(Key={"tenant_slug": "cached"})
        config2 = store.get("cached")

        assert config1 == config2

    @mock_aws
    def test_cache_expiry(self):
        dynamodb = boto3.resource("dynamodb", region_name="us-east-1")
        _create_table(dynamodb)
        _put_tenant(dynamodb, "expiry")

        store = TenantConfigStore(TABLE_NAME, dynamodb_resource=dynamodb)
        store.clear_cache()

        # First call
        store.get("expiry")

        # Expire cache by patching time.monotonic
        with patch("ses_invoice_processor.tenant_config.time") as mock_time:
            mock_time.monotonic.return_value = time.monotonic() + CACHE_TTL_SECONDS + 1
            # Delete from DynamoDB so we can detect cache miss
            dynamodb.Table(TABLE_NAME).delete_item(Key={"tenant_slug": "expiry"})
            result = store.get("expiry")

        assert result is None  # cache expired, DynamoDB miss

    @mock_aws
    def test_disabled_tenant_cached_raises_on_second_call(self):
        dynamodb = boto3.resource("dynamodb", region_name="us-east-1")
        _create_table(dynamodb)
        _put_tenant(dynamodb, "off", enabled=False)

        store = TenantConfigStore(TABLE_NAME, dynamodb_resource=dynamodb)
        store.clear_cache()

        with pytest.raises(TenantDisabledError):
            store.get("off")

        # Second call should also raise (from cache)
        with pytest.raises(TenantDisabledError):
            store.get("off")


class TestExtractEmailAddress:
    def test_bare_email(self):
        assert extract_email_address("user@example.com") == "user@example.com"

    def test_display_name_with_angle_brackets(self):
        assert extract_email_address("Mudit Sahni <muditsahni@msn.com>") == "muditsahni@msn.com"

    def test_case_normalized(self):
        assert extract_email_address("User@EXAMPLE.COM") == "user@example.com"

    def test_display_name_case_normalized(self):
        assert extract_email_address("John Doe <John@Company.COM>") == "john@company.com"

    def test_whitespace_trimmed(self):
        assert extract_email_address("  user@example.com  ") == "user@example.com"

    def test_angle_brackets_with_spaces(self):
        assert extract_email_address("Name < user@example.com >") == "user@example.com"


class TestIsSenderAllowed:
    def test_empty_list_rejects_all(self):
        assert is_sender_allowed("anyone@example.com", ()) is False
        assert is_sender_allowed("anyone@example.com", []) is False

    def test_wildcard_accepts_all(self):
        assert is_sender_allowed("anyone@example.com", ("*",)) is True
        assert is_sender_allowed("other@other.com", ["*"]) is True

    def test_exact_email_match(self):
        assert is_sender_allowed("user@gmail.com", ("user@gmail.com",)) is True

    def test_exact_email_case_insensitive(self):
        assert is_sender_allowed("User@Gmail.COM", ("user@gmail.com",)) is True

    def test_exact_email_no_match(self):
        assert is_sender_allowed("other@gmail.com", ("user@gmail.com",)) is False

    def test_domain_match(self):
        assert is_sender_allowed("anyone@company.com", ("@company.com",)) is True

    def test_domain_match_case_insensitive(self):
        assert is_sender_allowed("user@COMPANY.COM", ("@company.com",)) is True

    def test_domain_no_match(self):
        assert is_sender_allowed("user@other.com", ("@company.com",)) is False

    def test_multiple_entries(self):
        allowed = ("@company.com", "contractor@gmail.com")
        assert is_sender_allowed("employee@company.com", allowed) is True
        assert is_sender_allowed("contractor@gmail.com", allowed) is True
        assert is_sender_allowed("random@gmail.com", allowed) is False

    def test_domain_does_not_match_substring(self):
        # "@company.com" should NOT match "user@notcompany.com"
        assert is_sender_allowed("user@notcompany.com", ("@company.com",)) is False
