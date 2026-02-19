# SES Inbound Email Invoice Processor — Deployment Guide

Multi-tenant email processing: receive invoices at `invoices@<tenant>.satvos.com`, extract attachments, and process them through the SATVOS backend. A single shared Lambda handles all tenants, with per-tenant credentials stored in DynamoDB.

## Prerequisites

- AWS account with SES, S3, Lambda, DynamoDB, IAM access
- Domain managed in GoDaddy (or any DNS provider)
- A running SATVOS backend instance
- Python 3.12 installed locally (for packaging)

## Architecture

```
Email → GoDaddy MX → AWS SES (inbound)
                          │
                          ├── S3 (raw MIME at ses-inbound/<tenant>/<messageId>)
                          └── Lambda (shared, per-tenant receipt rules)
                                │
                                ▼
                         Extract tenant from recipient
                         DynamoDB lookup (tenant config, cached 5min)
                         Parse MIME email
                         Validate subject
                         Extract attachments
                                │
                                ▼
                         SATVOS Backend API (API key auth)
                         ├── Create collection
                         ├── Batch upload files
                         └── Create documents
```

### What's Shared vs Per-Tenant

| Shared (one-time setup) | Per-Tenant (onboarding) |
|--------------------------|------------------------|
| Lambda function | GoDaddy MX + TXT records |
| S3 bucket (`satvos-uploads`) | SES domain verification |
| IAM role | SES receipt rule |
| DynamoDB table | DynamoDB tenant config entry |
| | SATVOS service account |

---

## Initial Setup (One-Time)

### 1. S3 Bucket Policy

SES needs permission to write email objects. Go to **S3** → `satvos-uploads` → **Permissions** → **Bucket policy**.

Add this statement:

```json
{
    "Sid": "AllowSESPuts",
    "Effect": "Allow",
    "Principal": {
        "Service": "ses.amazonaws.com"
    },
    "Action": "s3:PutObject",
    "Resource": "arn:aws:s3:::satvos-uploads/ses-inbound/*",
    "Condition": {
        "StringEquals": {
            "AWS:SourceAccount": "<YOUR_AWS_ACCOUNT_ID>"
        }
    }
}
```

### 2. DynamoDB Table

Create the tenant config table:

```bash
aws dynamodb create-table \
    --table-name satvos-email-processor-tenants \
    --attribute-definitions AttributeName=tenant_slug,AttributeType=S \
    --key-schema AttributeName=tenant_slug,KeyType=HASH \
    --billing-mode PAY_PER_REQUEST \
    --region ap-south-1
```

**Table Schema:**

| Attribute | Type | Description |
|-----------|------|-------------|
| `tenant_slug` (PK) | String | e.g. `passpl` |
| `service_api_key` | String | Service account API key (`sk_...`) |
| `enabled` | Boolean | Toggle processing on/off |
| `api_base_url` | String (optional) | Override default API URL |
| `created_at` | String | ISO 8601 |
| `updated_at` | String | ISO 8601 |

### 3. SES Receipt Rule Set

1. Go to **SES** → **Email receiving** → **Create rule set**
2. Rule set name: `satvos-inbound`
3. Click **Create rule set** → **Set as active**

### 4. IAM Role

1. Go to **IAM** → **Roles** → **Create role**
2. Trusted entity: **AWS service** → **Lambda** → Next
3. Attach policy: **AWSLambdaBasicExecutionRole** → Next
4. Role name: `ses-inbound-email-processor-role` → **Create role**

Add inline policy for S3 and DynamoDB:

```json
{
    "Version": "2012-10-17",
    "Statement": [
        {
            "Effect": "Allow",
            "Action": [
                "s3:GetObject",
                "s3:ListBucket"
            ],
            "Resource": [
                "arn:aws:s3:::satvos-uploads",
                "arn:aws:s3:::satvos-uploads/ses-inbound/*"
            ]
        },
        {
            "Effect": "Allow",
            "Action": "dynamodb:GetItem",
            "Resource": "arn:aws:dynamodb:ap-south-1:<ACCOUNT_ID>:table/satvos-email-processor-tenants"
        }
    ]
}
```

### 5. Lambda Function

#### Package the code

```bash
cd lambda
rm -rf package lambda.zip
pip install -r requirements.txt -t package/
cp -r ses_invoice_processor package/
cd package && zip -r ../lambda.zip . && cd ..
```

#### Create the function

1. Go to **Lambda** → **Create function** → **Author from scratch**
2. Function name: `ses-inbound-email-processor`
3. Runtime: **Python 3.12**, Architecture: **x86_64**
4. **IMPORTANT: Function type must be "Standard"** (NOT Durable)
5. Execution role: select the role from step 4
6. **Create function**

#### Configure

**Upload code:** **Code** tab → **Upload from** → **.zip file**

**Set handler:** `ses_invoice_processor.handler.lambda_handler`

**Timeout/memory:** 120 seconds, 256 MB

**Environment variables:**

| Key | Value | Notes |
|-----|-------|-------|
| `SATVOS_API_BASE_URL` | `https://<your-api>/api/v1` | Default API URL for all tenants |
| `SES_EMAIL_BUCKET` | `satvos-uploads` | |
| `DYNAMODB_TABLE_NAME` | `satvos-email-processor-tenants` | |
| `LOG_LEVEL` | `INFO` | Optional |

> **Note:** No per-tenant credentials in env vars. All tenant-specific config comes from DynamoDB.

**Resource-based policy for SES:**

1. **Configuration** → **Permissions** → **Resource-based policy**
2. **Add permissions**: Principal `ses.amazonaws.com`, Action `lambda:InvokeFunction`
3. Source ARN: `arn:aws:ses:<region>:<account-id>:receipt-rule-set/satvos-inbound:receipt-rule/*`

> Use wildcard `*` for the rule name so all per-tenant receipt rules can invoke the shared Lambda.

---

## Onboarding a New Tenant

Use the onboarding script for automated setup:

```bash
cd lambda/scripts
pip install -r requirements-scripts.txt

python onboard_tenant.py \
    --tenant-slug acme \
    --service-api-key sk_<key-from-backend> \
    --lambda-function-arn arn:aws:lambda:ap-south-1:353922334273:function:ses-inbound-email-processor \
    --dry-run  # Remove for actual execution
```

The script will:
1. Create SES domain identity for `<tenant>.satvos.com`
2. Create SES receipt rule pointing to the shared Lambda
3. Insert tenant config (with API key) into DynamoDB
4. Print GoDaddy DNS records you need to add manually

> **Note:** The `inbound_email` service account is auto-created by the backend when a tenant is created. The API key is printed to stdout at startup. Use `POST /service-accounts/:id/rotate-key` to generate a new key if needed.

### Manual Onboarding Steps

If you prefer to onboard manually:

#### Step 1: DNS (GoDaddy)

Add MX record:

| Type | Name | Value | Priority | TTL |
|------|------|-------|----------|-----|
| MX | `<tenant>` | `inbound-smtp.<region>.amazonaws.com` | 10 | 1 Hour |

#### Step 2: SES Domain Verification

1. **SES** → **Identities** → **Create identity** → Domain: `<tenant>.satvos.com`
2. Add the TXT record to GoDaddy:

| Type | Name | Value | TTL |
|------|------|-------|-----|
| TXT | `_amazonses.<tenant>` | *(token from SES)* | 1 Hour |

#### Step 3: SES Receipt Rule

1. Go to rule set `satvos-inbound` → **Create rule**
2. Rule name: `<tenant>-invoices`
3. Recipient: `invoices@<tenant>.satvos.com`
4. Actions (in order):
   - **S3**: bucket `satvos-uploads`, prefix `ses-inbound/<tenant>/`
   - **Lambda**: select the shared Lambda, invocation type **Event**
5. Enable spam/virus scanning

> The S3 action must be BEFORE the Lambda action.

#### Step 4: SATVOS Service Account

The `inbound_email` service account is auto-created when the tenant is created. The API key is printed to the server's stdout at startup.

If you need to create one manually:

```
POST /api/v1/service-accounts
Authorization: Bearer <admin-jwt>
{
    "name": "inbound_email",
    "description": "Service account for inbound email processing"
}
```

The response contains the raw API key (shown only once). Use this key in the DynamoDB config entry below.

#### Step 5: DynamoDB Config Entry

```bash
aws dynamodb put-item \
    --table-name satvos-email-processor-tenants \
    --item '{
        "tenant_slug": {"S": "<tenant>"},
        "service_api_key": {"S": "sk_<key-from-step-4>"},
        "enabled": {"BOOL": true},
        "created_at": {"S": "2025-01-01T00:00:00Z"},
        "updated_at": {"S": "2025-01-01T00:00:00Z"}
    }' \
    --region ap-south-1
```

Optional: add `"api_base_url": {"S": "https://custom.api/v1"}` to use a different API endpoint for this tenant.

---

## Disabling a Tenant

Set `enabled` to `false` in DynamoDB:

```bash
aws dynamodb update-item \
    --table-name satvos-email-processor-tenants \
    --key '{"tenant_slug": {"S": "<tenant>"}}' \
    --update-expression "SET enabled = :e, updated_at = :u" \
    --expression-attribute-values '{":e": {"BOOL": false}, ":u": {"S": "2025-06-01T00:00:00Z"}}' \
    --region ap-south-1
```

The Lambda will log a warning and return 200 (no processing, no SES retries). The cache expires in 5 minutes.

---

## Migrating from Single-Tenant

If you have an existing single-tenant deployment (passpl):

1. Create DynamoDB table (see Initial Setup step 2)
2. Get the `inbound_email` service account API key from the backend (printed at startup, or create/rotate via admin API)
3. Insert passpl config:
   ```bash
   aws dynamodb put-item \
       --table-name satvos-email-processor-tenants \
       --item '{
           "tenant_slug": {"S": "passpl"},
           "service_api_key": {"S": "sk_<api-key-from-backend>"},
           "enabled": {"BOOL": true},
           "created_at": {"S": "2025-01-01T00:00:00Z"},
           "updated_at": {"S": "2025-01-01T00:00:00Z"}
       }' \
       --region ap-south-1
   ```
4. Update Lambda IAM role (add DynamoDB GetItem, widen S3 prefix to `ses-inbound/*`)
5. Update Lambda env vars:
   - **Remove:** `SATVOS_SERVICE_EMAIL`, `SATVOS_SERVICE_PASSWORD`, `SATVOS_TENANT_SLUG`, `SES_EMAIL_PREFIX`
   - **Add:** `DYNAMODB_TABLE_NAME=satvos-email-processor-tenants`
   - **Keep:** `SATVOS_API_BASE_URL`, `SES_EMAIL_BUCKET`, `LOG_LEVEL`
5. Deploy new Lambda code
6. Update SES receipt rule: ensure `recipients` includes `invoices@passpl.satvos.com`
7. Test with email to `invoices@passpl.satvos.com`

---

## Testing

### Run unit tests

```bash
cd lambda
python3 -m venv .venv && source .venv/bin/activate
pip install -r requirements-dev.txt
PYTHONPATH=. pytest tests/ -v
```

### Lambda test event

In **Lambda** → **Test** tab:

```json
{
    "Records": [
        {
            "ses": {
                "mail": {
                    "messageId": "test-000",
                    "source": "test@example.com"
                },
                "receipt": {
                    "recipients": ["invoices@passpl.satvos.com"],
                    "spamVerdict": {"status": "PASS"},
                    "virusVerdict": {"status": "PASS"}
                }
            }
        }
    ]
}
```

This will fail with `NoSuchKey` (no email in S3) but confirms the Lambda runs, reads DynamoDB, and has correct permissions.

### End-to-end test

Send a real email:
- **To:** `invoices@<tenant>.satvos.com`
- **Subject:** `INVOICES: Test Company`
- **Attach:** One or more PDF/JPG/PNG files

Check:
1. **S3**: Email at `satvos-uploads/ses-inbound/<tenant>/<message-id>`
2. **CloudWatch Logs**: Lambda logs should show `tenant=<tenant>` in the summary
3. **SATVOS Backend**: New collection with uploaded documents

---

## Updating the Lambda

```bash
cd lambda
rm -rf package lambda.zip
pip install -r requirements.txt -t package/
cp -r ses_invoice_processor package/
cd package && zip -r ../lambda.zip . && cd ..
```

Upload via: **Lambda** → function → **Code** tab → **Upload from** → **.zip file**

---

## Email Subject Format

The Lambda only processes emails with subject matching:

```
INVOICES: <Company Name>
```

- Case-insensitive (`invoices:`, `Invoices:`, `INVOICES:` all work)
- Company name is used for the collection name
- All other subjects are silently ignored (logged, returns 200)

---

## Troubleshooting

| Symptom | Cause | Fix |
|---------|-------|-----|
| Email not arriving in S3 | MX record not propagated | Verify MX record, wait for propagation |
| Email in S3 but Lambda not triggered | Lambda action missing from SES rule | Add Lambda action to receipt rule |
| `AccessDenied` on `s3:GetObject` | IAM policy missing or wrong | Ensure both bucket ARN and path ARN in policy |
| `NoSuchKey` | S3 prefix mismatch | Ensure SES rule prefix matches `ses-inbound/<tenant>/` |
| `ResourceNotFoundException` on DynamoDB | Table doesn't exist or wrong name | Check `DYNAMODB_TABLE_NAME` env var |
| `Ignored: no matching tenant recipient` | Recipient doesn't match pattern | Ensure SES rule uses `invoices@<tenant>.satvos.com` |
| `Ignored: tenant not configured` | Missing DynamoDB entry | Run onboarding script or manually insert |
| `Ignored: tenant disabled` | `enabled` is `false` in DynamoDB | Update DynamoDB item |
| `Failed to create collection: 401` | Invalid API key | Check `service_api_key` in DynamoDB matches the backend SA key |
| `Failed to create collection: 403` | API key valid but missing collection permission | Verify SA has correct permissions, or that SA auto-creation succeeded |
| `Invalid Status in invocation output` | Lambda created as "Durable" type | Recreate as "Standard" type |
| Subject ignored | Doesn't match `INVOICES: <name>` | Check for `RE:`, `FWD:`, or missing company name |
| Stale config after DynamoDB update | 5-minute cache | Wait up to 5 minutes for cache to expire |

---

## Configuration Reference

### Lambda Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `SATVOS_API_BASE_URL` | Yes | — | Default backend API URL (including `/api/v1`) |
| `SES_EMAIL_BUCKET` | Yes | — | S3 bucket for email storage |
| `DYNAMODB_TABLE_NAME` | Yes | — | DynamoDB table for tenant configs |
| `LOG_LEVEL` | No | `INFO` | Python logging level |

### Lambda Settings

| Setting | Value |
|---------|-------|
| Runtime | Python 3.12 |
| Handler | `ses_invoice_processor.handler.lambda_handler` |
| Timeout | 120 seconds |
| Memory | 256 MB |
| Concurrency | 10 (reserved) |
| Type | Standard (NOT Durable) |
