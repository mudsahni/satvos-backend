"""AWS Lambda entry point for SES inbound email invoice processing (multi-tenant)."""

import logging

import boto3

from .config import Config
from .email_parser import parse_raw_email
from .exceptions import (
    ConfigError,
    NoAttachmentsError,
    SubjectMismatchError,
    TenantDisabledError,
)
from .satvos_client import SatvosClient
from .tenant_config import TenantConfigStore, extract_tenant_slug

logger = logging.getLogger("ses_invoice_processor")


def lambda_handler(event: dict, context) -> dict:
    """Process an SES inbound email event.

    Always returns 200 to prevent SES retries. Errors are logged to CloudWatch.
    """
    try:
        config = Config.from_env()
        config.configure_logging()
    except ConfigError as exc:
        # Still log even if config is broken — this is a deployment issue
        logging.basicConfig(level="ERROR")
        logging.error("Configuration error: %s", exc)
        return {"statusCode": 200, "body": f"Configuration error: {exc}"}

    try:
        return _process_event(event, config)
    except Exception:
        logger.exception("Unexpected error processing SES event")
        return {"statusCode": 200, "body": "Internal error (logged)"}


def _process_event(event: dict, config: Config) -> dict:
    """Inner processing logic, separated for testability."""
    # Extract SES event data
    ses_record = event["Records"][0]["ses"]
    mail = ses_record["mail"]
    receipt = ses_record["receipt"]
    message_id = mail["messageId"]

    logger.info("Processing email message_id=%s from=%s", message_id, mail.get("source", "unknown"))

    # Extract tenant from recipients
    recipients = receipt.get("recipients", [])
    tenant_slug = extract_tenant_slug(recipients)
    if tenant_slug is None:
        logger.warning("No valid tenant found in recipients: %s", recipients)
        return {"statusCode": 200, "body": "Ignored: no matching tenant recipient"}

    logger.info("Extracted tenant_slug=%s from recipients", tenant_slug)

    # Look up tenant config in DynamoDB
    store = TenantConfigStore(config.dynamodb_table_name)
    try:
        tenant_config = store.get(tenant_slug)
    except TenantDisabledError:
        logger.warning("Tenant '%s' is disabled, skipping", tenant_slug)
        return {"statusCode": 200, "body": f"Ignored: tenant '{tenant_slug}' is disabled"}

    if tenant_config is None:
        logger.warning("Tenant '%s' not found in config store", tenant_slug)
        return {"statusCode": 200, "body": f"Ignored: tenant '{tenant_slug}' not configured"}

    # Check spam/virus verdicts
    for verdict_key in ("spamVerdict", "virusVerdict"):
        verdict = receipt.get(verdict_key, {})
        if verdict.get("status") == "FAIL":
            logger.warning("Email %s rejected: %s=FAIL", message_id, verdict_key)
            return {"statusCode": 200, "body": f"Rejected: {verdict_key} FAIL"}

    # Fetch raw email from S3 (key is <prefix>/<tenant>/<messageId>)
    s3 = boto3.client("s3")
    s3_key = f"{config.ses_email_prefix}/{tenant_slug}/{message_id}"
    logger.info("Fetching email from s3://%s/%s", config.ses_email_bucket, s3_key)

    s3_response = s3.get_object(Bucket=config.ses_email_bucket, Key=s3_key)
    raw_bytes = s3_response["Body"].read()

    # Parse email
    try:
        parsed = parse_raw_email(raw_bytes)
    except SubjectMismatchError as exc:
        logger.info("Ignoring email (subject mismatch): %s", exc)
        return {"statusCode": 200, "body": "Ignored: subject mismatch"}
    except NoAttachmentsError as exc:
        logger.info("Ignoring email (no attachments): %s", exc)
        return {"statusCode": 200, "body": "Ignored: no attachments"}

    logger.info(
        "Parsed email: company=%s, attachments=%d, sender=%s",
        parsed.company_name,
        len(parsed.attachments),
        parsed.sender,
    )

    # Authenticate and process
    api_base_url = tenant_config.api_base_url or config.default_api_base_url
    client = SatvosClient(api_base_url, tenant_slug)
    client.authenticate_with_api_key(tenant_config.service_api_key)

    result = client.process_attachments(parsed.company_name, parsed.attachments, sender_email=parsed.sender)

    summary = (
        f"Processed: tenant={tenant_slug}, collection={result.collection_name}, "
        f"files_uploaded={result.files_uploaded}, "
        f"files_failed={len(result.files_failed)}, "
        f"documents_created={result.documents_created}, "
        f"documents_failed={len(result.documents_failed)}"
    )
    logger.info(summary)

    return {"statusCode": 200, "body": summary}
