#!/usr/bin/env python3
"""CLI tool to onboard a new tenant for the SES invoice processor.

Automates:
  - SES domain identity verification
  - SES receipt rule creation
  - DynamoDB tenant config entry

The inbound_email service account is auto-created by the SATVOS backend
when the tenant is created. Use the admin API to retrieve or rotate the key.

Outputs manual steps for GoDaddy DNS records.

Usage:
    pip install -r requirements-scripts.txt
    python onboard_tenant.py --tenant-slug acme --service-api-key sk_...

Use --dry-run to see what would be done without making changes.
"""

import sys
from datetime import datetime, timezone

import boto3
import click


@click.command()
@click.option("--tenant-slug", required=True, help="Tenant slug (e.g. 'passpl')")
@click.option("--service-api-key", required=True, help="Service account API key (sk_...)")
@click.option("--aws-region", default="ap-south-1", help="AWS region for SES and DynamoDB")
@click.option("--ses-rule-set", default="satvos-inbound", help="SES receipt rule set name")
@click.option("--lambda-function-arn", required=True, help="ARN of the shared Lambda function")
@click.option("--s3-bucket", default="satvos-uploads", help="S3 bucket for email storage")
@click.option("--dynamodb-table", default="satvos-email-processor-tenants", help="DynamoDB table name")
@click.option("--api-base-url", default=None, help="Custom API base URL for this tenant (optional)")
@click.option("--dry-run", is_flag=True, help="Show what would be done without making changes")
def onboard_tenant(
    tenant_slug,
    service_api_key,
    aws_region,
    ses_rule_set,
    lambda_function_arn,
    s3_bucket,
    dynamodb_table,
    api_base_url,
    dry_run,
):
    """Onboard a new tenant for the SES invoice processor."""
    domain = f"{tenant_slug}.satvos.com"
    recipient = f"invoices@{domain}"

    click.echo(f"\n{'=' * 60}")
    click.echo(f"  Onboarding tenant: {tenant_slug}")
    click.echo(f"  Domain: {domain}")
    click.echo(f"  Recipient: {recipient}")
    if dry_run:
        click.echo("  MODE: DRY RUN (no changes will be made)")
    click.echo(f"{'=' * 60}\n")

    # Step 1: SES domain verification
    click.echo("Step 1: SES Domain Verification")
    click.echo("-" * 40)
    ses = boto3.client("ses", region_name=aws_region)

    if dry_run:
        click.echo(f"  [DRY RUN] Would verify domain identity: {domain}")
    else:
        resp = ses.verify_domain_identity(Domain=domain)
        verification_token = resp["VerificationToken"]
        click.echo(f"  Domain identity created: {domain}")
        click.echo(f"  Verification token: {verification_token}")

    # Step 2: SES receipt rule
    click.echo(f"\nStep 2: SES Receipt Rule")
    click.echo("-" * 40)
    rule_name = f"{tenant_slug}-invoices"
    rule = {
        "Name": rule_name,
        "Enabled": True,
        "Recipients": [recipient],
        "ScanEnabled": True,
        "Actions": [
            {
                "S3Action": {
                    "BucketName": s3_bucket,
                    "ObjectKeyPrefix": f"ses-inbound/{tenant_slug}/",
                },
            },
            {
                "LambdaAction": {
                    "FunctionArn": lambda_function_arn,
                    "InvocationType": "Event",
                },
            },
        ],
    }

    if dry_run:
        click.echo(f"  [DRY RUN] Would create receipt rule: {rule_name}")
        click.echo(f"    Rule set: {ses_rule_set}")
        click.echo(f"    Recipient: {recipient}")
        click.echo(f"    S3 prefix: ses-inbound/{tenant_slug}/")
    else:
        ses.create_receipt_rule(RuleSetName=ses_rule_set, Rule=rule)
        click.echo(f"  Receipt rule created: {rule_name}")

    # Step 3: DynamoDB tenant config
    click.echo(f"\nStep 3: DynamoDB Tenant Config")
    click.echo("-" * 40)
    now = datetime.now(timezone.utc).isoformat()
    item = {
        "tenant_slug": tenant_slug,
        "service_api_key": service_api_key,
        "enabled": True,
        "created_at": now,
        "updated_at": now,
    }
    if api_base_url:
        item["api_base_url"] = api_base_url

    if dry_run:
        click.echo(f"  [DRY RUN] Would insert into {dynamodb_table}:")
        click.echo(f"    tenant_slug: {tenant_slug}")
        click.echo(f"    service_api_key: {service_api_key[:10]}...")
        click.echo(f"    enabled: True")
        if api_base_url:
            click.echo(f"    api_base_url: {api_base_url}")
    else:
        dynamodb = boto3.resource("dynamodb", region_name=aws_region)
        table = dynamodb.Table(dynamodb_table)
        table.put_item(Item=item)
        click.echo(f"  Tenant config inserted into {dynamodb_table}")

    # Step 4: Print manual DNS steps
    click.echo(f"\n{'=' * 60}")
    click.echo("  MANUAL STEPS REQUIRED — GoDaddy DNS Records")
    click.echo(f"{'=' * 60}\n")

    click.echo("Go to GoDaddy > DNS Management for satvos.com and add:\n")

    click.echo("1. MX Record (for email routing):")
    click.echo(f"   Type:     MX")
    click.echo(f"   Name:     {tenant_slug}")
    click.echo(f"   Value:    inbound-smtp.{aws_region}.amazonaws.com")
    click.echo(f"   Priority: 10")
    click.echo(f"   TTL:      1 Hour\n")

    if not dry_run:
        click.echo("2. TXT Record (for SES domain verification):")
        click.echo(f"   Type:  TXT")
        click.echo(f"   Name:  _amazonses.{tenant_slug}")
        click.echo(f"   Value: {verification_token}")  # noqa: F821
        click.echo(f"   TTL:   1 Hour\n")
    else:
        click.echo("2. TXT Record (for SES domain verification):")
        click.echo(f"   Type:  TXT")
        click.echo(f"   Name:  _amazonses.{tenant_slug}")
        click.echo(f"   Value: (will be provided after SES verification)")
        click.echo(f"   TTL:   1 Hour\n")

    click.echo("After adding DNS records, wait for propagation (usually minutes, up to 72 hours).")
    click.echo(f"Then verify SES domain status in AWS Console > SES > Identities > {domain}\n")

    if dry_run:
        click.echo("[DRY RUN] No changes were made.")
    else:
        click.echo("Onboarding complete! Test by sending an email to:")
        click.echo(f"  To: {recipient}")
        click.echo(f"  Subject: INVOICES: Test Company")
        click.echo(f"  Attach: One or more PDF/JPG/PNG files")

    return 0


if __name__ == "__main__":
    sys.exit(onboard_tenant())
