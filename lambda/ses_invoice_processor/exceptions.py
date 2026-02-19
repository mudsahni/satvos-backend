"""Custom exception hierarchy for the SES invoice processor."""


class InvoiceProcessorError(Exception):
    """Base exception for all invoice processor errors."""


class ConfigError(InvoiceProcessorError):
    """Missing or invalid environment variable configuration."""


class SubjectMismatchError(InvoiceProcessorError):
    """Email subject does not match the expected format."""


class NoAttachmentsError(InvoiceProcessorError):
    """No valid PDF/JPG/PNG attachments found in the email."""


class SatvosAPIError(InvoiceProcessorError):
    """SATVOS backend API call failure."""

    def __init__(self, message: str, status_code: int | None = None, response_body: str | None = None):
        super().__init__(message)
        self.status_code = status_code
        self.response_body = response_body


class AuthenticationError(SatvosAPIError):
    """Login or token refresh failure."""


class TenantDisabledError(InvoiceProcessorError):
    """Tenant exists but processing is disabled."""


class SenderNotAllowedError(InvoiceProcessorError):
    """Email sender is not in the tenant's allowed_senders list."""
