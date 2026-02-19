"""Tenant extraction from SES recipients and DynamoDB config lookup with caching."""

import logging
import re
import time
from dataclasses import dataclass

import boto3

from .exceptions import TenantDisabledError

logger = logging.getLogger("ses_invoice_processor")

# Matches invoices@<tenant>.satvos.com (tenant = lowercase alphanum + hyphens)
_RECIPIENT_RE = re.compile(r"^invoices@([a-z0-9-]+)\.satvos\.com$", re.IGNORECASE)

CACHE_TTL_SECONDS = 300  # 5 minutes

# Module-level cache survives across Lambda container reuse.
# Maps tenant_slug -> (TenantConfig, expiry_timestamp)
_cache: dict[str, tuple["TenantConfig", float]] = {}


def extract_tenant_slug(recipients: list[str]) -> str | None:
    """Extract tenant slug from SES recipient list.

    Looks for the first recipient matching invoices@<tenant>.satvos.com.
    Returns the tenant slug or None if no match found.
    """
    for addr in recipients:
        m = _RECIPIENT_RE.match(addr.strip())
        if m:
            return m.group(1).lower()
    return None


@dataclass(frozen=True)
class TenantConfig:
    tenant_slug: str
    service_api_key: str
    enabled: bool
    api_base_url: str | None = None

    @classmethod
    def from_dynamodb_item(cls, item: dict) -> "TenantConfig":
        """Parse a DynamoDB item (already deserialized by boto3 resource/Table)."""
        return cls(
            tenant_slug=item["tenant_slug"],
            service_api_key=item["service_api_key"],
            enabled=item.get("enabled", True),
            api_base_url=item.get("api_base_url") or None,
        )


class TenantConfigStore:
    """Fetches and caches tenant configuration from DynamoDB."""

    def __init__(self, table_name: str, dynamodb_resource=None):
        self._table_name = table_name
        self._dynamodb = dynamodb_resource

    def _get_table(self):
        if self._dynamodb is None:
            self._dynamodb = boto3.resource("dynamodb")
        return self._dynamodb.Table(self._table_name)

    def get(self, tenant_slug: str) -> TenantConfig | None:
        """Look up tenant config, using cache if fresh.

        Returns None if tenant not found in DynamoDB.
        Raises TenantDisabledError if tenant exists but is disabled.
        """
        now = time.monotonic()

        # Check cache
        if tenant_slug in _cache:
            config, expiry = _cache[tenant_slug]
            if now < expiry:
                logger.debug("Tenant config cache hit for %s", tenant_slug)
                if not config.enabled:
                    raise TenantDisabledError(f"Tenant '{tenant_slug}' is disabled")
                return config

        # Cache miss or expired — fetch from DynamoDB
        logger.info("Fetching tenant config from DynamoDB for %s", tenant_slug)
        table = self._get_table()
        resp = table.get_item(Key={"tenant_slug": tenant_slug})

        item = resp.get("Item")
        if item is None:
            logger.warning("Tenant '%s' not found in DynamoDB", tenant_slug)
            return None

        config = TenantConfig.from_dynamodb_item(item)
        _cache[tenant_slug] = (config, now + CACHE_TTL_SECONDS)

        if not config.enabled:
            raise TenantDisabledError(f"Tenant '{tenant_slug}' is disabled")

        return config

    @staticmethod
    def clear_cache():
        """Clear the module-level cache (useful for testing)."""
        _cache.clear()
