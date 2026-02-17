# Frontend Integration Guide: Duplicate Invoice Detection

## Overview

The validation engine includes a duplicate invoice detection rule (`logic.invoice.duplicate`) that checks whether another document in the same tenant already has the same seller GSTIN + invoice number combination. It returns tiered match results: exact IRN matches, strong matches (same financial year), and weak matches (different financial year).

This guide covers how to parse the duplicate detection result from the validation API and render it in a React + TypeScript frontend.

---

## 1. API Response Shape

### Endpoint

```
GET /api/v1/documents/:id/validation
Authorization: Bearer <access_token>
```

### Response

```json
{
  "success": true,
  "data": {
    "document_id": "880e8400-e29b-41d4-a716-446655440003",
    "validation_status": "warning",
    "summary": {
      "total": 59,
      "passed": 57,
      "errors": 0,
      "warnings": 2
    },
    "reconciliation_status": "valid",
    "reconciliation_summary": {
      "total": 22,
      "passed": 22,
      "errors": 0,
      "warnings": 0
    },
    "results": [
      {
        "rule_name": "Logical: Duplicate Invoice Detection",
        "rule_type": "custom",
        "severity": "warning",
        "passed": false,
        "field_path": "invoice",
        "expected_value": "no duplicate invoices",
        "actual_value": "2 duplicate(s) found [error]",
        "message": "Logical: Duplicate Invoice Detection: invoice INV-001 from seller 29ABCDE1234F1Z5 already exists in: \"Invoice-A.pdf\" [exact_irn] (uploaded 2025-08-01), \"Invoice-B.pdf\" [weak] (uploaded 2024-03-15)",
        "reconciliation_critical": false
      }
    ],
    "field_statuses": {}
  }
}
```

### TypeScript Types

```typescript
interface ValidationSummary {
  total: number;
  passed: number;
  errors: number;
  warnings: number;
}

interface ValidationResultItem {
  rule_name: string;
  rule_type: string;
  severity: string;
  passed: boolean;
  field_path: string;
  expected_value: string;
  actual_value: string;
  message: string;
  reconciliation_critical: boolean;
}

interface FieldStatus {
  status: string;
  messages: string[];
}

interface ValidationResponse {
  document_id: string;
  validation_status: string;
  summary: ValidationSummary;
  reconciliation_status: string;
  reconciliation_summary: ValidationSummary;
  results: ValidationResultItem[];
  field_statuses: Record<string, FieldStatus>;
}

interface APIResponse<T> {
  success: boolean;
  data: T;
}
```

### Key fields on the duplicate result

| Field | Value when duplicates found | Notes |
|-------|---------------------------|-------|
| `rule_name` | `"Logical: Duplicate Invoice Detection"` | Always this exact string |
| `severity` | `"warning"` | The rule's **registered** severity. Do NOT use this for display |
| `actual_value` | `"2 duplicate(s) found [error]"` | Contains the **effective** severity in brackets |
| `message` | Full text with each duplicate's name, match type, and upload date | Match types in brackets: `[exact_irn]`, `[strong]`, `[weak]` |
| `passed` | `false` | `true` means no duplicates (or check was skipped) |

**Important:** The `severity` field is always `"warning"` because that is the rule's registered severity in the database. The *effective* severity is embedded in `actual_value` -- it escalates to `"error"` when any match is `exact_irn` or `strong`.

---

## 2. Parsing Logic

```typescript
// lib/duplicate-detection.ts

const DUPLICATE_RULE_NAME = "Logical: Duplicate Invoice Detection";

type EffectiveSeverity = "error" | "warning";

type MatchType = "exact_irn" | "strong" | "weak";

interface DuplicateDetectionResult {
  found: true;
  effectiveSeverity: EffectiveSeverity;
  matchCount: number;
  message: string;
  matches: DuplicateMatchInfo[];
}

interface DuplicateMatchInfo {
  documentName: string;
  matchType: MatchType;
  uploadDate: string;
}

interface NoDuplicateResult {
  found: false;
}

type DuplicateParseResult = DuplicateDetectionResult | NoDuplicateResult;

/**
 * Find and parse the duplicate invoice detection result from validation results.
 */
export function parseDuplicateResult(
  results: ValidationResultItem[]
): DuplicateParseResult {
  const result = results.find((r) => r.rule_name === DUPLICATE_RULE_NAME);

  if (!result || result.passed) {
    return { found: false };
  }

  return {
    found: true,
    effectiveSeverity: extractEffectiveSeverity(result.actual_value),
    matchCount: extractMatchCount(result.actual_value),
    message: result.message,
    matches: extractMatches(result.message),
  };
}

/**
 * Extract effective severity from actual_value.
 * Format: "2 duplicate(s) found [error]" or "1 duplicate(s) found [warning]"
 */
function extractEffectiveSeverity(actualValue: string): EffectiveSeverity {
  const match = actualValue.match(/\[(error|warning)\]/);
  return match?.[1] === "error" ? "error" : "warning";
}

/**
 * Extract duplicate count from actual_value.
 * Format: "2 duplicate(s) found [error]"
 */
function extractMatchCount(actualValue: string): number {
  const match = actualValue.match(/^(\d+)\s+duplicate/);
  return match ? parseInt(match[1], 10) : 0;
}

/**
 * Extract individual match details from the message string.
 * Format: "Invoice-A.pdf" [exact_irn] (uploaded 2025-08-01), "Invoice-B.pdf" [weak] (uploaded 2024-03-15)
 */
function extractMatches(message: string): DuplicateMatchInfo[] {
  const pattern = /"([^"]+)"\s+\[(exact_irn|strong|weak)\]\s+\(uploaded\s+(\d{4}-\d{2}-\d{2})\)/g;
  const matches: DuplicateMatchInfo[] = [];
  let m: RegExpExecArray | null;

  while ((m = pattern.exec(message)) !== null) {
    matches.push({
      documentName: m[1],
      matchType: m[2] as MatchType,
      uploadDate: m[3],
    });
  }

  return matches;
}
```

---

## 3. React Component

```tsx
// components/DuplicateInvoiceAlert.tsx

import { useMemo } from "react";
import {
  parseDuplicateResult,
  type DuplicateMatchInfo,
} from "../lib/duplicate-detection";
import type { ValidationResultItem } from "../types";

const MATCH_TYPE_LABELS: Record<string, string> = {
  exact_irn: "Exact IRN match -- confirmed duplicate",
  strong:
    "Same seller, invoice number & financial year -- very likely duplicate",
  weak: "Same seller & invoice number, different financial year -- possible duplicate",
};

interface DuplicateInvoiceAlertProps {
  results: ValidationResultItem[];
}

export function DuplicateInvoiceAlert({ results }: DuplicateInvoiceAlertProps) {
  const parsed = useMemo(() => parseDuplicateResult(results), [results]);

  if (!parsed.found) {
    return null;
  }

  const isError = parsed.effectiveSeverity === "error";

  return (
    <div
      role="alert"
      style={{
        padding: "12px 16px",
        borderRadius: 6,
        border: `1px solid ${isError ? "#fca5a5" : "#fde68a"}`,
        backgroundColor: isError ? "#fef2f2" : "#fffbeb",
        color: isError ? "#991b1b" : "#92400e",
        marginBottom: 16,
      }}
    >
      <div style={{ fontWeight: 600, marginBottom: 8 }}>
        {isError
          ? `Duplicate invoice detected (${parsed.matchCount} match${parsed.matchCount > 1 ? "es" : ""})`
          : `Possible duplicate invoice (${parsed.matchCount} match${parsed.matchCount > 1 ? "es" : ""})`}
      </div>

      <ul style={{ margin: 0, paddingLeft: 20 }}>
        {parsed.matches.map((match, i) => (
          <DuplicateMatchItem key={i} match={match} />
        ))}
      </ul>
    </div>
  );
}

function DuplicateMatchItem({ match }: { match: DuplicateMatchInfo }) {
  return (
    <li style={{ marginBottom: 4 }}>
      <strong>{match.documentName}</strong>
      <span style={{ marginLeft: 8, fontSize: "0.875em", opacity: 0.8 }}>
        {MATCH_TYPE_LABELS[match.matchType] ?? match.matchType}
      </span>
      <span style={{ marginLeft: 8, fontSize: "0.875em", opacity: 0.6 }}>
        Uploaded {match.uploadDate}
      </span>
    </li>
  );
}
```

---

## 4. Usage Example

Fetch validation data and render the alert on a document detail page:

```tsx
// pages/document-detail.tsx

import { useEffect, useState } from "react";
import { DuplicateInvoiceAlert } from "../components/DuplicateInvoiceAlert";
import type { ValidationResponse, APIResponse } from "../types";

async function fetchValidation(
  documentId: string
): Promise<ValidationResponse> {
  const res = await fetch(`/api/v1/documents/${documentId}/validation`, {
    headers: { Authorization: `Bearer ${getAccessToken()}` },
  });
  if (!res.ok) throw new Error("Failed to fetch validation");
  const body: APIResponse<ValidationResponse> = await res.json();
  return body.data;
}

export function DocumentDetailPage({
  documentId,
}: {
  documentId: string;
}) {
  const [validation, setValidation] = useState<ValidationResponse | null>(
    null
  );

  useEffect(() => {
    fetchValidation(documentId).then(setValidation).catch(console.error);
  }, [documentId]);

  if (!validation) return <div>Loading...</div>;

  return (
    <div>
      <h1>Document Details</h1>

      {/* Duplicate alert renders at the top, before other validation details */}
      <DuplicateInvoiceAlert results={validation.results} />

      {/* Rest of your document detail UI */}
      <ValidationSummaryPanel summary={validation.summary} />
      <ValidationResultsTable results={validation.results} />
    </div>
  );
}
```

---

## 5. Match Type Descriptions

| Match Type | Severity Impact | User-Friendly Description |
|-----------|----------------|--------------------------|
| `exact_irn` | Escalates to **error** | Exact IRN match -- this is a confirmed duplicate. |
| `strong` | Escalates to **error** | Same seller, invoice number, and financial year -- very likely a duplicate. |
| `weak` | Stays as **warning** | Same seller and invoice number from a different financial year -- possible duplicate. |

**Severity escalation logic:** If any match in the result is `exact_irn` or `strong`, the effective severity is `error`. If all matches are `weak`, the effective severity stays `warning`.

---

## Gotchas

1. **Do not use `severity` for display.** The `severity` field on the result is always `"warning"` (the rule's registered database severity). Use the bracketed value in `actual_value` for the effective severity.
2. **`passed: true` means no duplicates or check skipped.** When `passed` is `true`, the `message` will say "no duplicate invoices found" or indicate the check was skipped (empty GSTIN/invoice number). The component correctly renders nothing in this case.
3. **The duplicate rule is not reconciliation-critical.** `reconciliation_critical` is always `false` for this rule, so duplicates do not affect `reconciliation_status`.
4. **Match details are embedded in the `message` string.** There is no structured array of matches in the API response. The parsing functions above extract them via regex from the message text.
5. **The validation endpoint requires a parsed document.** If `parsing_status` is not `"completed"`, the endpoint returns a 400 error. Only render the duplicate alert after confirming the document is parsed.
