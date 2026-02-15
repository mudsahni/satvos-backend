# SATVOS GST Invoice Validation Rules Guide

> **Audience**: Accountants, auditors, product managers, and developers who need to understand, configure, or extend the validation rules.
>
> **Last updated**: February 2026

---

## Table of Contents

1. [How Validation Works (The Big Picture)](#how-validation-works)
2. [What "Severity" Means](#severity)
3. [What "Reconciliation-Critical" Means](#reconciliation-critical)
4. [Validation Statuses Explained](#statuses)
5. [All Rules by Category](#all-rules)
   - [Required Fields (12 rules)](#required-fields)
   - [Format Checks (15 rules)](#format-checks)
   - [Mathematical Checks (12 rules)](#mathematical-checks)
   - [Cross-Field Checks (9 rules)](#cross-field-checks)
   - [Logical Checks (8 rules)](#logical-checks)
   - [HSN Master Checks (2 rules)](#hsn-checks)
   - [Duplicate Detection (1 rule)](#duplicate-detection)
6. [Tolerance Values](#tolerances)
7. [Reconciliation-Critical Rules (Quick Reference)](#recon-quick-ref)
8. [Ideas for New Rules](#ideas-for-new-rules)
9. [How to Change Rules](#how-to-change)

---

<a id="how-validation-works"></a>
## 1. How Validation Works (The Big Picture)

When a GST invoice is uploaded and parsed (or manually edited), the system automatically runs **59 validation rules** against the extracted data. Think of it as a checklist that the system runs through, checking everything from "is the invoice number present?" to "do the tax amounts add up correctly?"

Each rule produces a **pass** or **fail** result. Failed rules include a message explaining what went wrong (e.g., "Seller GSTIN format is invalid: expected 15-character alphanumeric pattern").

After all rules run, the system computes two independent statuses for the document:
- **Validation Status** — overall data quality
- **Reconciliation Status** — readiness for GSTR-2A/2B matching

These two statuses are independent. A document can be "valid" for reconciliation but "invalid" overall (e.g., a payment IFSC code is wrong — that doesn't affect GSTR matching).

---

<a id="severity"></a>
## 2. What "Severity" Means

Every rule has a severity level:

| Severity | What it means | Example |
|----------|---------------|---------|
| **Error** | This is wrong and must be fixed. The data is incorrect or unusable. | Invoice number is missing, GSTIN format is invalid, tax amounts don't add up |
| **Warning** | This looks suspicious but may be acceptable. Worth reviewing. | Currency field is empty, due date is missing, invoice date is in the future |

**The key difference**: A single **error** makes the document "invalid". Warnings alone make it "warning" status (needs review but not necessarily wrong).

---

<a id="reconciliation-critical"></a>
## 3. What "Reconciliation-Critical" Means

Some rules are marked as **reconciliation-critical**. These are the rules that matter specifically for GSTR-2A/2B matching — the process of reconciling your purchase invoices against the seller's filed returns.

If a reconciliation-critical rule fails, it means the invoice data cannot be reliably matched against GST portal records. For example:
- If the seller's GSTIN is missing or malformed, you can't look it up in GSTR-2A
- If the tax amounts don't add up, the figures won't match the portal
- If the invoice number is missing, there's nothing to match against

Non-reconciliation-critical rules (like "is the payment IFSC code valid?") don't affect your ability to match invoices — they're just general data quality checks.

---

<a id="statuses"></a>
## 4. Validation Statuses Explained

### Validation Status (overall data quality)

| Status | Meaning | When it happens |
|--------|---------|-----------------|
| **Valid** | All rules passed. Data looks correct. | Every rule returned "pass" |
| **Warning** | No hard errors, but some things look suspicious. | At least one warning-severity rule failed, but no error-severity rules failed |
| **Invalid** | There are definite errors in the data. | At least one error-severity rule failed |
| **Pending** | Not yet validated. | Document hasn't been parsed yet, or was just edited |

### Reconciliation Status (GSTR-2A/2B readiness)

| Status | Meaning | When it happens |
|--------|---------|-----------------|
| **Valid** | Ready for reconciliation. All critical fields are present and correct. | All reconciliation-critical rules passed |
| **Warning** | Might have issues during reconciliation. | A reconciliation-critical warning failed (e.g., IRN hash mismatch) |
| **Invalid** | Cannot be reliably reconciled. | A reconciliation-critical error failed (e.g., GSTIN missing or malformed) |
| **Pending** | Not yet validated. | Same as above |

---

<a id="all-rules"></a>
## 5. All Rules by Category

---

<a id="required-fields"></a>
### Required Fields (12 rules)

These check whether essential fields are present (not blank).

| # | Rule Key | What It Checks | Severity | Recon-Critical? | Notes |
|---|----------|---------------|----------|-----------------|-------|
| 1 | `req.invoice.number` | Invoice number is not blank | Error | **Yes** | Without this, you can't identify the invoice at all |
| 2 | `req.invoice.date` | Invoice date is not blank | Error | **Yes** | Required for GSTR filing period matching |
| 3 | `req.invoice.place_of_supply` | Place of supply is not blank | Error | **Yes** | Determines whether CGST/SGST or IGST applies |
| 4 | `req.invoice.currency` | Currency is not blank | Warning | No | Defaults are usually INR; missing is suspicious but not critical |
| 5 | `req.seller.name` | Seller name is not blank | Error | **Yes** | Essential for identifying the supplier |
| 6 | `req.seller.gstin` | Seller GSTIN is not blank | Error | **Yes** | The primary key for GSTR-2A matching |
| 7 | `req.seller.state_code` | Seller state code is not blank | Error | No | Needed for intra/interstate tax type checks |
| 8 | `req.buyer.name` | Buyer name is not blank | Error | No | Identifies your entity |
| 9 | `req.buyer.gstin` | Buyer GSTIN is not blank | Error | **Yes** | Your GSTIN — needed for GSTR-2B matching |
| 10 | `req.buyer.state_code` | Buyer state code is not blank | Error | No | Needed for intra/interstate tax type checks |
| 11 | `req.line_item.description` | Each line item has a description | Warning | No | Checked per line item; missing description is suspicious but not blocking |
| 12 | `req.line_item.hsn_sac` | Each line item has an HSN/SAC code | Warning | No | Checked per line item; HSN codes are increasingly mandatory under GST |

**What "not blank" means**: The field must contain at least one character. Whitespace-only values are currently treated as present (not trimmed before checking).

---

<a id="format-checks"></a>
### Format Checks (15 rules)

These check whether field values follow the correct pattern/format. **Important**: If a field is blank, the format check is skipped (passes automatically). This avoids double-flagging — the required field check already handles missing values.

#### GSTIN Format (2 rules)

| # | Rule Key | What It Checks | Severity | Recon-Critical? |
|---|----------|---------------|----------|-----------------|
| 1 | `fmt.seller.gstin` | Seller GSTIN matches the 15-character pattern | Error | **Yes** |
| 2 | `fmt.buyer.gstin` | Buyer GSTIN matches the 15-character pattern | Error | **Yes** |

**Expected GSTIN pattern**: `22AAAAA0000A1Z5`
- Positions 1-2: State code (two digits, 01-38)
- Positions 3-7: Five letters from PAN
- Positions 8-11: Four digits from PAN
- Position 12: One letter from PAN
- Position 13: One letter or digit (entity number within state, cannot be 0)
- Position 14: Always the letter "Z"
- Position 15: One letter or digit (check digit)

#### PAN Format (2 rules)

| # | Rule Key | What It Checks | Severity | Recon-Critical? |
|---|----------|---------------|----------|-----------------|
| 3 | `fmt.seller.pan` | Seller PAN matches the 10-character pattern | Error | No |
| 4 | `fmt.buyer.pan` | Buyer PAN matches the 10-character pattern | Error | No |

**Expected PAN pattern**: `AAAAA0000A` — 5 letters + 4 digits + 1 letter.

#### State Code Format (2 rules)

| # | Rule Key | What It Checks | Severity | Recon-Critical? |
|---|----------|---------------|----------|-----------------|
| 5 | `fmt.seller.state_code` | Seller state code is a valid 2-digit code (01-38) | Error | **Yes** |
| 6 | `fmt.buyer.state_code` | Buyer state code is a valid 2-digit code (01-38) | Error | **Yes** |

**Valid range**: 01 through 38 (covers all Indian states and union territories).

#### Date Format (2 rules)

| # | Rule Key | What It Checks | Severity | Recon-Critical? |
|---|----------|---------------|----------|-----------------|
| 7 | `fmt.invoice.date` | Invoice date is a recognizable date format | Error | No |
| 8 | `fmt.invoice.due_date` | Due date is a recognizable date format | Warning | No |

**Accepted date formats**: `2025-01-15`, `15-01-2025`, `15/01/2025`, `01-15-2025`, `01/15/2025`, `2025/01/15`, `15 Jan 2025`, `January 15, 2025`, and more (12 formats supported). Also supports timestamps like `15-01-2025 14:30:00` and ISO format `2025-01-15T14:30:00+05:30`.

#### Currency Format (1 rule)

| # | Rule Key | What It Checks | Severity | Recon-Critical? |
|---|----------|---------------|----------|-----------------|
| 9 | `fmt.invoice.currency` | Currency is a recognized ISO code | Warning | No |

**Recognized currencies**: INR, USD, EUR, GBP, JPY, AUD, CAD, CHF, CNY, SGD, AED, SAR, HKD, MYR, THB, NZD, SEK, NOK, DKK, ZAR. The check is case-insensitive.

#### Payment Details Format (2 rules)

| # | Rule Key | What It Checks | Severity | Recon-Critical? |
|---|----------|---------------|----------|-----------------|
| 10 | `fmt.payment.ifsc` | IFSC code matches the 11-character pattern | Warning | No |
| 11 | `fmt.payment.account_no` | Account number is 9-18 digits | Warning | No |

**Expected IFSC pattern**: `ABCD0123456` — 4 letters + digit "0" + 6 alphanumeric characters.

#### HSN/SAC Code Format (1 rule)

| # | Rule Key | What It Checks | Severity | Recon-Critical? |
|---|----------|---------------|----------|-----------------|
| 12 | `fmt.line_item.hsn_sac` | Each line item's HSN/SAC code is 4-8 digits | Warning | No |

Checked per line item. HSN codes can be 4, 6, or 8 digits depending on turnover threshold.

#### IRN & e-Invoice Format (3 rules)

| # | Rule Key | What It Checks | Severity | Recon-Critical? |
|---|----------|---------------|----------|-----------------|
| 13 | `fmt.invoice.irn` | IRN is a valid 64-character hex string (SHA-256 hash) | Error | No |
| 14 | `fmt.invoice.ack_number` | Acknowledgement number is numeric | Warning | No |
| 15 | `fmt.invoice.ack_date` | Acknowledgement date is a recognizable date format | Warning | No |

**IRN format**: Exactly 64 lowercase hexadecimal characters (the SHA-256 hash used by the e-invoicing system).

---

<a id="mathematical-checks"></a>
### Mathematical Checks (12 rules)

These verify that the numbers add up correctly. All calculations allow a **tolerance of +/- Rs. 1.00** to account for rounding differences between the invoice and the parsed data.

#### Per-Line-Item Math (5 rules, checked for each line item)

| # | Rule Key | What It Checks | Formula | Severity | Recon-Critical? |
|---|----------|---------------|---------|----------|-----------------|
| 1 | `math.line_item.taxable_amount` | Taxable amount calculation | taxable = (quantity x unit_price) - discount | Error | No |
| 2 | `math.line_item.cgst_amount` | CGST amount calculation | cgst_amount = taxable x cgst_rate / 100 | Error | No |
| 3 | `math.line_item.sgst_amount` | SGST amount calculation | sgst_amount = taxable x sgst_rate / 100 | Error | No |
| 4 | `math.line_item.igst_amount` | IGST amount calculation | igst_amount = taxable x igst_rate / 100 | Error | No |
| 5 | `math.line_item.total` | Line item total calculation | total = taxable + cgst + sgst + igst | Error | No |

#### Invoice-Level Totals (7 rules)

| # | Rule Key | What It Checks | Formula | Severity | Recon-Critical? |
|---|----------|---------------|---------|----------|-----------------|
| 6 | `math.totals.subtotal` | Subtotal is the sum of all line item taxable amounts | subtotal = SUM(all line item taxable amounts) | Error | No |
| 7 | `math.totals.taxable_amount` | Taxable amount after discount | taxable = subtotal - total_discount | Error | **Yes** |
| 8 | `math.totals.cgst` | Total CGST is the sum of all line item CGST | cgst = SUM(all line item CGST amounts) | Error | **Yes** |
| 9 | `math.totals.sgst` | Total SGST is the sum of all line item SGST | sgst = SUM(all line item SGST amounts) | Error | **Yes** |
| 10 | `math.totals.igst` | Total IGST is the sum of all line item IGST | igst = SUM(all line item IGST amounts) | Error | **Yes** |
| 11 | `math.totals.grand_total` | Grand total calculation | total = taxable + cgst + sgst + igst + cess + round_off | Error | **Yes** |
| 12 | `math.totals.round_off` | Round-off amount is reasonable | abs(round_off) must be <= Rs. 0.50 | Warning | No |

**Note on round-off**: The round-off check uses a tighter threshold of **Rs. 0.50** (not 1.00). Round-off amounts larger than 50 paise are suspicious and flagged for review.

---

<a id="cross-field-checks"></a>
### Cross-Field Checks (9 rules)

These verify that related fields are consistent with each other.

#### GSTIN-State Code Consistency (2 rules)

| # | Rule Key | What It Checks | Severity | Recon-Critical? |
|---|----------|---------------|----------|-----------------|
| 1 | `xf.seller.gstin_state` | First 2 digits of seller GSTIN match seller state code | Error | **Yes** |
| 2 | `xf.buyer.gstin_state` | First 2 digits of buyer GSTIN match buyer state code | Error | **Yes** |

**Why this matters**: A GSTIN always starts with the 2-digit state code. If someone's GSTIN says "29" (Karnataka) but their state code says "27" (Maharashtra), something is wrong.

#### GSTIN-PAN Consistency (2 rules)

| # | Rule Key | What It Checks | Severity | Recon-Critical? |
|---|----------|---------------|----------|-----------------|
| 3 | `xf.seller.gstin_pan` | Characters 3-12 of seller GSTIN match seller PAN | Error | No |
| 4 | `xf.buyer.gstin_pan` | Characters 3-12 of buyer GSTIN match buyer PAN | Error | No |

**Why this matters**: A GSTIN embeds the entity's PAN in positions 3-12. If the PAN on the invoice doesn't match the PAN embedded in the GSTIN, one of them is wrong.

#### Tax Type Consistency (2 rules)

| # | Rule Key | What It Checks | Severity | Recon-Critical? |
|---|----------|---------------|----------|-----------------|
| 5 | `xf.tax_type.intrastate` | Intrastate invoices use CGST+SGST (not IGST) | Error | **Yes** |
| 6 | `xf.tax_type.interstate` | Interstate invoices use IGST (not CGST+SGST) | Error | **Yes** |

**How it works**: If the seller and buyer are in the same state (same state code), the invoice must use CGST + SGST. If they're in different states, it must use IGST. You cannot use both on the same line item.

These two rules are **mutually exclusive** — only one fires per invoice depending on whether it's intrastate or interstate. If either state code is missing, both rules are skipped.

#### Other Cross-Field Checks (3 rules)

| # | Rule Key | What It Checks | Severity | Recon-Critical? |
|---|----------|---------------|----------|-----------------|
| 7 | `xf.invoice.due_after_date` | Due date is on or after invoice date | Warning | No |
| 8 | `xf.parties.different_gstin` | Seller and buyer have different GSTINs | Warning | No |
| 9 | `xf.invoice.irn_hash` | IRN matches the expected SHA-256 hash | Warning | **Yes** |

**Rule 8 explained**: If the seller and buyer have the exact same GSTIN, this is flagged. While self-invoicing exists in rare cases, it's usually a data entry error.

**Rule 9 explained (IRN hash verification)**: The e-invoicing system generates the IRN as `SHA-256(sellerGSTIN + invoiceNumber + financialYear)`. This rule recomputes the hash and checks if it matches the stated IRN. A mismatch means either the IRN was copied incorrectly, or the invoice number/seller GSTIN was modified after IRN generation. The financial year follows April-March convention (e.g., an invoice dated January 2025 falls in FY 2024-25).

---

<a id="logical-checks"></a>
### Logical Checks (8 rules)

These check for business logic violations that don't fit into the other categories.

#### Line Item Logic (4 rules, checked per line item)

| # | Rule Key | What It Checks | Severity | Recon-Critical? |
|---|----------|---------------|----------|-----------------|
| 1 | `logic.line_item.non_negative` | Quantities, prices, and tax amounts are not negative | Error | No |
| 2 | `logic.line_item.valid_tax_rate` | Tax rates are standard GST rates | Warning | No |
| 3 | `logic.line_item.cgst_eq_sgst` | CGST rate equals SGST rate | Error | No |
| 4 | `logic.line_item.exclusive_tax` | Line item uses EITHER CGST+SGST OR IGST, never both | Error | **Yes** |

**Rule 1 detail**: Checks 7 fields per line item: quantity, unit_price, taxable_amount, cgst_amount, sgst_amount, igst_amount, total. Each must be >= 0.

**Rule 2 detail**: Valid GST rates are: **0%, 0.125%, 0.25%, 1.5%, 2.5%, 3%, 5%, 6%, 9%, 12%, 14%, 18%, 28%**. Any other rate is flagged. Note: CGST and SGST rates are typically half the total rate (e.g., 9% CGST + 9% SGST = 18% total), while IGST is the full rate.

**Rule 3 detail**: Under GST, CGST and SGST must always be equal. If CGST is 9% and SGST is 6%, something is wrong.

**Rule 4 detail**: A single line item cannot have both CGST/SGST and IGST applied. This is a fundamental GST rule — intrastate transactions use CGST+SGST, interstate use IGST, never both.

#### Invoice-Level Logic (4 rules)

| # | Rule Key | What It Checks | Severity | Recon-Critical? |
|---|----------|---------------|----------|-----------------|
| 5 | `logic.line_items.at_least_one` | Invoice has at least one line item | Error | **Yes** |
| 6 | `logic.invoice.date_not_future` | Invoice date is not in the future | Warning | No |
| 7 | `logic.totals.non_negative` | Total amounts (subtotal, taxable, cgst, sgst, igst, total) are not negative | Error | No |
| 8 | `logic.invoice.irn_expected` | B2B invoices should have an IRN (e-invoice) | Warning | No |

**Rule 7 detail**: Checks 6 totals fields. Note that **cess**, **round_off**, and **total_discount** are deliberately NOT checked here because they can legitimately be zero or negative.

**Rule 8 detail**: If a seller has a GSTIN (indicating a registered business, i.e., B2B), an IRN is expected under the e-invoicing mandate. This is a warning because not all B2B transactions require e-invoicing (turnover thresholds apply).

---

<a id="hsn-checks"></a>
### HSN Master Checks (2 rules)

These validate line item HSN/SAC codes against the government's master list of HSN codes, which is loaded into the system at startup.

| # | Rule Key | What It Checks | Severity | Recon-Critical? |
|---|----------|---------------|----------|-----------------|
| 1 | `logic.line_item.hsn_exists` | HSN code exists in the government master list | Warning | No |
| 2 | `xf.line_item.hsn_rate` | The applied GST rate matches valid rates for that HSN code | Warning | No |

**Rule 1 detail**: Uses hierarchical lookup — if the exact 8-digit code isn't found, it tries the 6-digit prefix, then the 4-digit prefix. This accommodates different levels of HSN granularity.

**Rule 2 detail**: Compares the effective GST rate on the line item against the valid rates for that HSN code in the master list. The effective rate is IGST (for interstate) or CGST + SGST (for intrastate). A tolerance of 0.01% is used for rate comparison.

**Note**: If the HSN master list is empty (no data loaded), rule 1 will fail for all codes but rule 2 will skip gracefully.

---

<a id="duplicate-detection"></a>
### Duplicate Detection (1 rule)

| # | Rule Key | What It Checks | Severity | Recon-Critical? |
|---|----------|---------------|----------|-----------------|
| 1 | `logic.invoice.duplicate` | No other document has the same seller GSTIN + invoice number | Warning | No |

**How it works**: Searches the database for other documents (within the same tenant) that have the same seller GSTIN and invoice number combination. If duplicates are found, the message includes the names and upload dates of the matching documents.

**Edge cases handled**:
- The current document is excluded from the search (it won't match against itself)
- If the seller GSTIN or invoice number is blank, the check is skipped
- If the database query fails, the check passes silently (doesn't block validation)

---

<a id="tolerances"></a>
## 6. Tolerance Values

| What | Tolerance | Where Used | Why |
|------|-----------|------------|-----|
| Math calculations | +/- Rs. 1.00 | All math rules (line items and totals) | Rounding differences between invoice software and our parser |
| Round-off amount | +/- Rs. 0.50 | `math.totals.round_off` only | Round-off should never exceed 50 paise |
| HSN rate matching | +/- 0.01% | `xf.line_item.hsn_rate` | Floating point precision |

**Should the math tolerance be changed?** The Rs. 1.00 tolerance is generous. For high-value invoices, a Rs. 1.00 difference is negligible. For very low-value invoices (< Rs. 100), it might mask real errors. If you need tighter checking, this value can be reduced — but beware of false positives from legitimate rounding.

---

<a id="recon-quick-ref"></a>
## 7. Reconciliation-Critical Rules (Quick Reference)

These 22 rules determine whether an invoice can be reliably matched against GSTR-2A/2B records. If any of these fail, the reconciliation status is affected.

| Category | Rules | Count |
|----------|-------|-------|
| **Required Fields** | Invoice number, invoice date, place of supply, seller name, seller GSTIN, buyer GSTIN | 6 |
| **Format** | Seller GSTIN format, buyer GSTIN format, seller state code, buyer state code | 4 |
| **Cross-Field** | Seller GSTIN-state match, buyer GSTIN-state match, intrastate tax type, interstate tax type, IRN hash | 5 |
| **Math** | Taxable amount total, CGST total, SGST total, IGST total, grand total | 5 |
| **Logical** | Exclusive tax types (per item), at least one line item | 2 |
| | **Total** | **22** |

---

<a id="ideas-for-new-rules"></a>
## 8. Ideas for New Rules

These rules don't currently exist but could add value:

### High Value

| Proposed Rule | Category | Suggested Severity | Recon-Critical? | Rationale |
|---------------|----------|-------------------|-----------------|-----------|
| **Stale invoice date** — invoice date more than 180 days old | Logical | Warning | No | Very old invoices might indicate data entry errors or delayed processing; ITC eligibility has time limits |
| **GSTIN active status check** — verify GSTIN is active on the GST portal | Cross-field | Warning | Yes | A cancelled/suspended GSTIN means ITC cannot be claimed; requires external API integration |
| **E-invoicing turnover threshold** — require IRN based on seller turnover | Logical | Warning | No | E-invoicing is mandatory above certain turnover thresholds; currently rule 8 just flags any B2B without IRN |
| **Reverse charge validation** — when reverse_charge is true, verify tax is computed on buyer side | Cross-field | Error | Yes | Reverse charge transactions have specific tax treatment that should be validated |
| **Cess rate validation** — check cess rates against known cess-applicable goods | Cross-field | Warning | No | Cess applies only to specific goods (tobacco, aerated drinks, motor vehicles); arbitrary cess is suspicious |
| **Place of supply vs state codes** — place of supply should match buyer state for most transactions | Cross-field | Warning | Yes | Place of supply determines tax type; inconsistency with buyer state is suspicious |
| **Credit/debit note linking** — credit/debit notes should reference an original invoice | Required | Warning | No | Credit/debit notes without an original invoice reference are incomplete |
| **Amount in words vs total** — verify the amount-in-words matches the numeric total | Cross-field | Warning | No | Discrepancy suggests data entry error or OCR misread |

### Medium Value

| Proposed Rule | Category | Suggested Severity | Rationale |
|---------------|----------|-------------------|-----------|
| **Whitespace-only field detection** — treat fields containing only spaces/tabs as blank | Required | Same as parent rule | Currently, a field with just spaces passes the "required" check |
| **Duplicate line items** — flag line items with identical HSN + description + amount | Logical | Warning | May indicate copy-paste errors |
| **Total discount reasonableness** — flag discounts > 50% of subtotal | Logical | Warning | Unusually high discounts are suspicious |
| **Unit price reasonableness** — flag unit prices of 0 when quantity > 0 | Logical | Warning | Free goods should be explicitly marked |
| **Missing payment details for large invoices** — flag invoices > Rs. 50,000 without bank details | Logical | Warning | Large invoices typically include payment info for audit trail |

---

<a id="how-to-change"></a>
## 9. How to Change Rules

### Changing Severity

To change a rule from **error** to **warning** (or vice versa), find the rule's definition and change its `Severity` field. For example, to make "Required: Currency" an error instead of a warning:

**File**: `internal/validator/invoice/required.go`
**Change**: `Severity: domain.ValidationSeverityWarning` → `Severity: domain.ValidationSeverityError`

### Making a Rule Reconciliation-Critical

To add a rule to the reconciliation-critical set, change its `ReconciliationCritical` field to `true`. For example, to make the duplicate detection rule affect reconciliation status:

**File**: `internal/validator/invoice/duplicate.go`
**Change**: `ReconciliationCritical: false` → `ReconciliationCritical: true`

### Changing Math Tolerance

To tighten or loosen the math tolerance:

**File**: `internal/validator/invoice/math.go`
**Change**: `mathTolerance = 1.00` → your desired value (e.g., `0.50` for stricter, `2.00` for more lenient)

### Changing the Round-Off Threshold

**File**: `internal/validator/invoice/math.go`
**Change**: `math.Abs(inv.Totals.RoundOff) <= 0.50` → your desired threshold

### Adding a New Required Field

1. Add a new entry to the `RequiredFieldValidators()` function in `internal/validator/invoice/required.go`
2. Follow the existing pattern — specify the rule key, name, field path, severity, whether it's reconciliation-critical, and the extraction function
3. The system will auto-seed the new rule for all tenants on next validation

### Adding a New Format Validator

1. Add to `FormatValidators()` in `internal/validator/invoice/format.go`
2. Provide the regex pattern or custom validation function
3. Remember: format validators should pass when the field is empty (let the required field check handle that)

### Adding an Entirely New Rule Category

1. Create a new file in `internal/validator/invoice/`
2. Implement the `Validator` interface (or use the `BuiltinValidator` wrapper)
3. Register your validators in `AllBuiltinValidators()` in `builtin_rules.go`
4. The engine will automatically pick them up

### Rule Configuration per Tenant

Rules are stored in the database per tenant. The system auto-seeds all builtin rules, but individual rules can be enabled/disabled per tenant via the `document_validation_rules` table. This allows different tenants to have different validation configurations without code changes.

---

## Appendix: Valid GST Tax Rates

The following rates are considered valid under GST:

| Rate | Common Usage |
|------|-------------|
| 0% | Exempted goods/services (milk, fresh vegetables, etc.) |
| 0.125% | Rough diamonds (special rate) |
| 0.25% | Rough precious/semi-precious stones |
| 1.5% | Cut and polished diamonds |
| 2.5% | CGST/SGST component of 5% slab |
| 3% | Gold, silver, platinum |
| 5% | Essential goods (packaged food, footwear < Rs. 1000, etc.) |
| 6% | CGST/SGST component of 12% slab |
| 9% | CGST/SGST component of 18% slab |
| 12% | Processed food, business class air travel, etc. |
| 14% | CGST/SGST component of 28% slab |
| 18% | Most goods and services (standard rate) |
| 28% | Luxury goods, demerit goods (tobacco, aerated drinks, etc.) |

**Note**: CGST and SGST are always equal and each is half the total rate. For example, an 18% GST item has 9% CGST + 9% SGST for intrastate, or 18% IGST for interstate.
