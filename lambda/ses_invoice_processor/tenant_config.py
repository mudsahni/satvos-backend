"""Tenant extraction from SES recipients and DynamoDB config lookup with caching."""

import logging
import re
import time
from dataclasses import dataclass, field

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
    allowed_senders: tuple[str, ...] = ()  # frozen → tuple not list
    api_base_url: str | None = None

    @classmethod
    def from_dynamodb_item(cls, item: dict) -> "TenantConfig":
        """Parse a DynamoDB item (already deserialized by boto3 resource/Table)."""
        return cls(
            tenant_slug=item["tenant_slug"],
            service_api_key=item["service_api_key"],
            enabled=item.get("enabled", True),
            allowed_senders=tuple(item.get("allowed_senders", [])),
            api_base_url=item.get("api_base_url") or None,
        )


# --- Sender allowlist helpers ---

_EMAIL_ADDR_RE = re.compile(r"<([^>]+)>")


def extract_email_address(sender: str) -> str:
    """Extract bare email from a From header value.

    "Mudit Sahni <muditsahni@msn.com>" → "muditsahni@msn.com"
    "plain@example.com" → "plain@example.com"
    """
    m = _EMAIL_ADDR_RE.search(sender)
    if m:
        return m.group(1).strip().lower()
    return sender.strip().lower()


def is_sender_allowed(sender_email: str, allowed_senders: tuple[str, ...] | list[str]) -> bool:
    """Check if a sender email is permitted by the allowlist.

    Rules:
    - Empty list → reject all (safe default)
    - "*" in list → accept all
    - Entry starting with "@" → domain match (case-insensitive)
    - Otherwise → exact email match (case-insensitive)
    """
    if not allowed_senders:
        return False
    email_lower = sender_email.lower()
    for entry in allowed_senders:
        entry = entry.strip()
        if entry == "*":
            return True
        if entry.startswith("@"):
            if email_lower.endswith(entry.lower()):
                return True
        elif email_lower == entry.lower():
            return True
    return False


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
