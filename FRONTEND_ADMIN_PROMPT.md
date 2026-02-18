# SATVOS Admin Dashboard — Frontend Implementation Prompt

You are building an admin dashboard for SATVOS, a multi-tenant GST document processing platform. The frontend is a **Next.js + TypeScript** application that connects to an existing Go REST API. This is an **existing codebase** — integrate into the current project structure.

---

## Table of Contents

1. [API Foundation](#1-api-foundation)
2. [Authentication & Session Management](#2-authentication--session-management)
3. [Role-Based Access Control (RBAC)](#3-role-based-access-control-rbac)
4. [Page-by-Page Specification](#4-page-by-page-specification)
   - 4.1 [Dashboard (Home)](#41-dashboard-home)
   - 4.2 [Document Management](#42-document-management)
   - 4.3 [Collection Management](#43-collection-management)
   - 4.4 [File Management](#44-file-management)
   - 4.5 [User Management](#45-user-management)
   - 4.6 [Service Account Management](#46-service-account-management)
   - 4.7 [Reports](#47-reports)
   - 4.8 [Tenant Management (Super Admin)](#48-tenant-management-super-admin)
5. [TypeScript Types](#5-typescript-types)
6. [API Client Layer](#6-api-client-layer)
7. [UI/UX Guidelines](#7-uiux-guidelines)
8. [Error Handling](#8-error-handling)

---

## 1. API Foundation

**Base URL**: `/api/v1`

**Response Envelope** — Every API response follows this shape:

```typescript
interface APIResponse<T> {
  success: boolean;
  data?: T;
  error?: { code: string; message: string };
  meta?: { total: number; offset: number; limit: number };
}
```

**Pagination** — All list endpoints accept `?offset=0&limit=20` (max 100). Response includes `meta` with `total` count.

**Authentication** — All protected endpoints require `Authorization: Bearer <access_token>` header.

**Health Checks**:
- `GET /healthz` — Liveness (no auth)
- `GET /readyz` — Readiness (no auth)

---

## 2. Authentication & Session Management

### 2.1 Login Flow

```
POST /api/v1/auth/login
Body: { "tenant_slug": string, "email": string, "password": string }
Response: { "access_token": string, "refresh_token": string }
```

The login page needs a **tenant slug** field in addition to email/password. The tenant slug identifies which organization the user belongs to. Store both tokens securely (httpOnly cookies preferred, or secure localStorage with XSS precautions).

**JWT Claims** (decoded from access token for client-side role checks):
```typescript
interface JWTClaims {
  tenant_id: string;   // UUID
  user_id: string;     // UUID
  email: string;
  role: "admin" | "manager" | "member" | "viewer" | "free" | "service";
  exp: number;
  iss: string;         // "satvos"
}
```

Access token expires in **15 minutes**. Refresh token expires in **7 days**.

### 2.2 Token Refresh

```
POST /api/v1/auth/refresh
Body: { "refresh_token": string }
Response: { "access_token": string, "refresh_token": string }
```

Implement automatic token refresh: intercept 401 responses, attempt refresh, retry the original request. If refresh fails, redirect to login.

### 2.3 Registration (Free Tier)

```
POST /api/v1/auth/register
Body: { "email": string, "full_name": string, "password": string }
Response 201: { "user": User, "collection": Collection, "tokens": TokenPair }
```

Free-tier users self-register into a shared "satvos" tenant. They get a personal collection automatically. After registration, show a banner prompting email verification.

### 2.4 Email Verification

```
GET /api/v1/auth/verify-email?token=<jwt_token>
Response: { "message": "email verified successfully" }
```

This is typically opened from an email link. Build a `/verify-email` page that reads the `token` query param and calls the API. Show success/error state.

```
POST /api/v1/auth/resend-verification  (authenticated)
Response: { "message": "verification email sent" }
```

### 2.5 Password Reset

```
POST /api/v1/auth/forgot-password
Body: { "tenant_slug": string, "email": string }
Response: { "message": "if an account with that email exists, a password reset link has been sent" }
```

Always returns 200 (no email enumeration). Build a forgot-password form and a "check your email" confirmation page.

```
POST /api/v1/auth/reset-password
Body: { "token": string, "new_password": string }
Response: { "message": "password has been reset successfully" }
```

Build a `/reset-password` page that reads the token from the URL and shows a new password form.

### 2.6 Google Social Login

```
POST /api/v1/auth/social-login
Body: { "provider": "google", "id_token": string }
Response 200|201: { "user": User, "collection"?: Collection, "tokens": TokenPair, "is_new_user": boolean }
```

Integrate Google Sign-In button. On success, send the Google ID token to the backend. If `is_new_user` is true, the user was auto-registered on the free tier.

---

## 3. Role-Based Access Control (RBAC)

### 3.1 Role Hierarchy

| Role | Level | Description | Implicit Collection Access |
|------|-------|-------------|---------------------------|
| `admin` | 4 | Full tenant control | Owner on all collections |
| `manager` | 3 | Collection editing, policy enforcement | Editor on all collections |
| `member` | 2 | Standard user | Viewer on all collections |
| `viewer` | 1 | Read-only, explicit grants only | None |
| `free` | 0 | Self-registered, quota-limited | None (only personal collection) |
| `service` | 0 | API-only (never logs into UI) | None |

### 3.2 Collection Permissions

| Permission | Level | Capabilities |
|-----------|-------|-------------|
| `owner` | 3 | Full control: create/delete collection, manage permissions, upload, review, validate, tag |
| `editor` | 2 | Upload files, create/modify/validate documents, manage tags |
| `viewer` | 1 | Read-only access to documents and reports |

**Effective Permission** = `max(implicit_from_role, explicit_collection_grant)`

### 3.3 UI Visibility Rules

Implement these visibility rules in a centralized RBAC utility:

| Feature/Page | Visible To |
|---|---|
| Dashboard | All authenticated users |
| Documents (list, view, search) | All (scoped by collection access) |
| Documents (create, edit, retry, validate, tag) | editor+ on collection |
| Documents (review, assign) | editor+ on collection (not service accounts) |
| Documents (delete) | admin only |
| Collections (list, view) | All |
| Collections (create) | admin, manager, member |
| Collections (edit, permissions) | owner on collection |
| Collections (delete) | owner on collection |
| Files (upload) | admin, manager, member, free, service |
| Files (delete) | admin only |
| Users page | admin only |
| Service Accounts page | admin only |
| Tenant Management page | admin only |
| Reports | All (data scoped by role — viewer/free see only their collections) |
| Review Queue | All (shows docs assigned to current user) |

### 3.4 Free Tier Restrictions

- Email must be verified before uploading files or creating documents. Show a verification banner if `user.email_verified === false` and `user.role === "free"`.
- Monthly document quota: `user.monthly_document_limit` (default 5). Show usage: `user.documents_used_this_period` / `user.monthly_document_limit`. Quota resets every 30 days from `user.current_period_start`.
- File listing is filtered server-side to only show files uploaded by the free user.

---

## 4. Page-by-Page Specification

### 4.1 Dashboard (Home)

**Route**: `/dashboard`
**API**: `GET /api/v1/stats`

**Response**:
```typescript
interface Stats {
  total_documents: number;
  total_collections: number;
  parsing_completed: number;
  parsing_failed: number;
  parsing_processing: number;
  parsing_pending: number;
  parsing_queued: number;
  validation_valid: number;
  validation_warning: number;
  validation_invalid: number;
  reconciliation_valid: number;
  reconciliation_warning: number;
  reconciliation_invalid: number;
  review_pending: number;
  review_approved: number;
  review_rejected: number;
}
```

**Layout**:
1. **Summary Cards** (top row):
   - Total Documents (with trend arrow if you track history)
   - Total Collections
   - Review Pending (clickable → navigates to review queue)
   - Parsing Failed (clickable → navigates to documents filtered by status)

2. **Parsing Status** — Donut/pie chart:
   - Segments: Completed (green), Processing (blue), Pending (gray), Failed (red), Queued (yellow)

3. **Validation Status** — Donut/pie chart:
   - Segments: Valid (green), Warning (amber), Invalid (red)

4. **Reconciliation Status** — Donut/pie chart:
   - Segments: Valid (green), Warning (amber), Invalid (red)

5. **Review Status** — Horizontal stacked bar:
   - Segments: Approved (green), Pending (gray), Rejected (red)

6. **Quick Actions** panel:
   - "Upload File" button
   - "Create Document" button
   - "View Review Queue" link
   - "View Reports" link

**Note**: Stats are tenant-scoped and role-filtered server-side. Admin/manager/member see tenant-wide stats. Viewer/free see only stats from their accessible collections.

---

### 4.2 Document Management

#### 4.2.1 Document List

**Route**: `/documents`
**API**: `GET /api/v1/documents?offset=0&limit=20&collection_id=<optional>&assigned_to=<optional>`

**Response**: Paginated array of `Document` objects.

**Features**:
- **Table view** with columns: Name, Collection, Parsing Status, Validation Status, Reconciliation Status, Review Status, Assigned To, Created At
- **Status badges** with color coding:
  - Parsing: pending (gray), processing (blue), completed (green), failed (red), queued (yellow)
  - Validation: pending (gray), valid (green), warning (amber), invalid (red)
  - Reconciliation: pending (gray), valid (green), warning (amber), invalid (red)
  - Review: pending (gray), approved (green), rejected (red)
- **Filters**:
  - Collection dropdown (from user's accessible collections)
  - Assigned To dropdown (from tenant users)
  - (Client-side or future API) Parsing status, validation status, review status filters
- **Pagination**: Offset-based with page size selector (20/50/100)
- **Row click**: Navigate to document detail page
- **Actions column**:
  - Retry (if parsing failed) — `POST /documents/:id/retry`
  - Delete (admin only) — `DELETE /documents/:id`

#### 4.2.2 Create Document

**Route**: `/documents/new` (or modal)
**API**: `POST /api/v1/documents`

**Request**:
```json
{
  "file_id": "uuid",
  "collection_id": "uuid",
  "document_type": "gst_invoice",
  "parse_mode": "single" | "dual",
  "name": "optional name",
  "tags": { "key": "value" }
}
```

**Form fields**:
1. **File**: Select from uploaded files (show file picker or link to upload first)
2. **Collection**: Dropdown of collections where user has editor+ access
3. **Document Type**: Currently only `gst_invoice`
4. **Parse Mode**: Radio — "Single" (default, faster) or "Dual" (two parsers, higher accuracy)
5. **Name**: Optional text input
6. **Tags**: Optional key-value pair inputs (add/remove rows)

After creation, the document starts parsing in the background. Navigate to the document detail page and show real-time status.

#### 4.2.3 Document Detail

**Route**: `/documents/:id`
**API**: `GET /api/v1/documents/:id`

This is the **most complex page**. It should display:

**Header Section**:
- Document name, ID, collection link
- Status badges: parsing, validation, reconciliation, review
- Parse mode badge (single/dual)
- Parser model used
- Assignment info (assigned to user, assigned by, assigned at)
- Action buttons:
  - "Retry Parse" (if failed/completed)
  - "Validate" (re-run validation)
  - "Assign" (open user picker)
  - "Review" (approve/reject with notes)
  - "Delete" (admin only, with confirmation)

**Tab 1: Structured Data (Invoice View)**

Display the parsed `structured_data` (GSTInvoice) in a structured, readable form:

```typescript
interface GSTInvoice {
  invoice: {
    invoice_number: string;
    invoice_date: string;        // DD/MM/YYYY
    due_date: string;
    invoice_type: string;        // "Tax Invoice", "Credit Note", etc.
    currency: string;            // "INR"
    place_of_supply: string;
    reverse_charge: boolean;
    irn: string;                 // 64-char hex, Invoice Registration Number
    acknowledgement_number: string;
    acknowledgement_date: string;
    qr_code_data: string;
  };
  seller: {
    name: string;
    address: string;
    gstin: string;               // 15-char GSTIN format: 22AAAAA0000A1Z5
    pan: string;                 // 10-char PAN
    state: string;
    state_code: string;          // 2-digit code
  };
  buyer: {
    name: string;
    address: string;
    gstin: string;
    pan: string;
    state: string;
    state_code: string;
  };
  line_items: Array<{
    description: string;
    hsn_sac_code: string;        // 4-8 digit HSN/SAC code
    quantity: number;
    unit: string;
    unit_price: number;
    discount: number;
    taxable_amount: number;
    cgst_rate: number;           // percentage
    cgst_amount: number;
    sgst_rate: number;
    sgst_amount: number;
    igst_rate: number;
    igst_amount: number;
    total: number;
  }>;
  totals: {
    subtotal: number;
    total_discount: number;
    taxable_amount: number;
    cgst: number;
    sgst: number;
    igst: number;
    cess: number;
    round_off: number;
    total: number;
    amount_in_words: string;
  };
  payment: {
    bank_name: string;
    account_number: string;
    ifsc_code: string;
    payment_terms: string;
  };
  notes: string;
}
```

**Display layout for structured data**:
- **Invoice Header**: Card with key invoice metadata fields in a 2-column grid
- **Seller/Buyer**: Side-by-side cards with party details
- **Line Items**: Editable table with all columns. Show HSN code, quantities, rates, amounts. Include row totals
- **Totals**: Summary card showing subtotal → taxes → total breakdown
- **Payment**: Card with bank details
- **Notes**: Text block

**Confidence Indicators**: The `confidence_scores` field mirrors the GSTInvoice structure but with float64 (0.0-1.0) values. Display confidence as:
- >= 0.8: green checkmark (high confidence)
- 0.5-0.8: amber warning (medium confidence)
- < 0.5: red flag (low confidence)

Show confidence inline next to each field value.

**Field Provenance** (for dual-parse mode): The `field_provenance` JSON records how each field was determined:
- `"agree"` — Both parsers agreed
- `"primary"` — Only primary parser had a value
- `"secondary"` — Only secondary parser had a value
- `"primary_format"` / `"secondary_format"` — Disagreement resolved by format matching
- `"disagreement"` — Unresolved disagreement
- `"manual_edit"` — User edited this field

Show provenance as small badges/tooltips next to each field.

**Inline Editing**: Users with editor+ permission can edit structured data inline.
- Click field to edit → modify → Save triggers `PUT /documents/:id/structured-data` with the full updated `structured_data` JSON
- After save: confidence resets to 1.0, review resets to pending, auto-tags regenerate, validation re-runs, provenance becomes `"manual_edit"`

**Tab 2: Validation Results**

**API**: `GET /api/v1/documents/:id/validation`

Display validation results grouped by category. Each result entry:
```typescript
interface ValidationResultEntry {
  rule_id: string;
  passed: boolean;
  field_path: string;          // e.g., "invoice.invoice_number"
  expected_value: string;
  actual_value: string;
  message: string;
  reconciliation_critical: boolean;
  validated_at: string;
}
```

**Layout**:
- Summary bar: X passed, Y warnings, Z errors
- Grouped by field path (invoice, seller, buyer, line_items, totals)
- Each rule shows: pass/fail icon, rule message, expected vs actual values
- Reconciliation-critical rules flagged with a special badge
- "Re-validate" button at top

**Tab 3: Tags**

**API**:
- `GET /documents/:id/tags` — List tags
- `POST /documents/:id/tags` — Add tags `{ "tags": { "key": "value" } }`
- `DELETE /documents/:id/tags/:tagId` — Delete tag

Display as key-value chip list. Tags have a `source` field:
- `"auto"` — System-generated from parsed data (show with auto badge, not deletable)
- `"user"` — User-created (editable/deletable)

"Add Tag" form: key input + value input + add button.

**Tab 4: Audit Trail**

**API**: `GET /api/v1/documents/:id/audit?offset=0&limit=20`

**Response**: Paginated array of:
```typescript
interface DocumentAuditEntry {
  id: string;
  tenant_id: string;
  document_id: string;
  user_id?: string;
  action: string;
  changes: object;             // Action-specific metadata
  created_at: string;
}
```

**Actions** (13 types):
- `document.created` — Document created
- `document.parse_completed` — AI parsing succeeded
- `document.parse_failed` — AI parsing failed
- `document.parse_queued` — Queued for retry (rate limited)
- `document.retry` — User triggered re-parse
- `document.review` — Approved/rejected with notes
- `document.edit_structured_data` — Manual data edit
- `document.validate` — Manual validation trigger
- `document.validation_completed` — Validation results updated (changes: `{validation_status, reconciliation_status, trigger}`)
- `document.tags_added` — Tags added
- `document.tag_deleted` — Tag removed
- `document.deleted` — Document deleted
- `document.assigned` — Assignment changed (changes: `{assigned_to, assigned_by}`)

**Display**: Vertical timeline with:
- Timestamp
- User who performed the action (resolve user_id to name if possible)
- Action description (human-readable)
- Expandable `changes` JSON details

#### 4.2.4 Review Queue

**Route**: `/documents/review-queue`
**API**: `GET /api/v1/documents/review-queue?offset=0&limit=20`

Shows documents assigned to the current user that are parsed (parsing_status=completed) and pending review (review_status=pending). Ordered by assignment date.

**Table columns**: Document Name, Collection, Assigned At, Validation Status, Reconciliation Status
**Row click**: Navigate to document detail page (review tab)

#### 4.2.5 Document Assignment

**API**: `PUT /api/v1/documents/:id/assign`

```json
{
  "assignee_id": "user-uuid"    // null to unassign
}
```

Build a user-picker dialog. The assignee must have editor+ permission on the document's collection. Service accounts cannot be assigned.

#### 4.2.6 Document Review

**API**: `PUT /api/v1/documents/:id/review`

```json
{
  "status": "approved" | "rejected",
  "notes": "optional reviewer notes"
}
```

Build a review dialog with approve/reject buttons and a notes textarea. The assignee of a document **cannot** approve/reject it themselves (backend enforces `ErrAssigneeCannotReview`). Service accounts cannot review.

#### 4.2.7 Tag Search

**Route**: `/documents/search` (or integrated into document list)
**API**: `GET /api/v1/documents/search/tags?key=<key>&value=<value>&offset=0&limit=20`

Search form with key and value inputs. Returns paginated document list.

---

### 4.3 Collection Management

#### 4.3.1 Collection List

**Route**: `/collections`
**API**: `GET /api/v1/collections?offset=0&limit=20`

Returns collections the user has access to (all for admin, permission-filtered for others).

**Table columns**: Name, Description, Document Count, Created At
**Actions**: Edit, Delete (if owner), View Permissions

#### 4.3.2 Create Collection

**API**: `POST /api/v1/collections`

Handler binds from JSON body:
```json
{
  "name": "string (required)",
  "description": "string"
}
```

Allowed roles: admin, manager, member. Creator automatically gets owner permission.

#### 4.3.3 Collection Detail

**Route**: `/collections/:id`
**API**: `GET /api/v1/collections/:id`

**Tabs**:

**Tab 1: Documents** — List documents in this collection using `GET /documents?collection_id=:id`

**Tab 2: Files** — Files in this collection
- Batch upload: `POST /collections/:id/files` (multipart form with multiple `files` fields)
  - Response: Array of `{ file_name, success, file?, error? }` per file
- Remove file: `DELETE /collections/:id/files/:fileId`

**Tab 3: Permissions** (owner only)
- List: `GET /collections/:id/permissions?offset=0&limit=20`
  - Returns: Array of `{ id, collection_id, tenant_id, user_id, permission, granted_by, created_at }`
- Set: `POST /collections/:id/permissions`
  ```json
  { "user_id": "uuid", "permission": "owner" | "editor" | "viewer" }
  ```
- Remove: `DELETE /collections/:id/permissions/:userId`
  - Cannot remove your own permission

Build a permission management UI:
- Table of current permissions (user name, email, permission level, granted by)
- "Add Permission" button → user picker + permission level dropdown
- Edit permission level inline
- Delete button (with confirmation, disabled for self)

**Tab 4: Export**
- CSV Export: `GET /collections/:id/export/csv` — Downloads a CSV file (33 columns including reconciliation fields, UTF-8 BOM). Trigger as a file download
- Tally XML Export: `GET /collections/:id/export/tally?company_name=<optional>` — Downloads Tally Prime purchase voucher XML. Optional `company_name` query param (defaults to collection name)

#### 4.3.4 Edit Collection

**API**: `PUT /api/v1/collections/:id`

Handler binds from JSON:
```json
{
  "name": "string",
  "description": "string"
}
```

Requires owner permission on the collection.

#### 4.3.5 Delete Collection

**API**: `DELETE /api/v1/collections/:id`

Requires owner permission. Cascading delete (documents, files associations, permissions). Show strong confirmation dialog.

---

### 4.4 File Management

#### 4.4.1 File List

**Route**: `/files`
**API**: `GET /api/v1/files?offset=0&limit=20`

Note: Free-tier and service accounts only see files they uploaded (server-filtered).

**Table columns**: Original Name, File Type, File Size, Status, Uploaded By, Created At
**Row click**: View file detail / download

#### 4.4.2 Upload File

**API**: `POST /api/v1/files/upload` (multipart/form-data)

Form fields:
- `file` — The file (PDF, JPG, PNG, max 50MB)
- `collection_id` — Optional collection to add to

Build a drag-and-drop upload zone. Show upload progress. After upload, optionally prompt to create a document from the file.

Free-tier users must have verified email. If not verified, show verification banner instead of upload form.

#### 4.4.3 File Detail

**API**: `GET /api/v1/files/:id`

**Response**:
```json
{
  "file": { ...FileMeta },
  "download_url": "presigned S3 URL (expires in 1 hour)"
}
```

Show file metadata and a "Download" button linking to the presigned URL. For images (JPG/PNG), show an inline preview. For PDFs, show an embedded PDF viewer or download link.

#### 4.4.4 Delete File

**API**: `DELETE /api/v1/files/:id` (admin only)

---

### 4.5 User Management

**Route**: `/users`
**Visibility**: Admin only

#### 4.5.1 User List

**API**: `GET /api/v1/users?offset=0&limit=20` (admin only)

**Table columns**: Full Name, Email, Role, Active Status, Email Verified, Auth Provider, Document Usage, Created At

**Role badge colors**: admin (purple), manager (blue), member (teal), viewer (gray), free (green)

#### 4.5.2 Create User

**API**: `POST /api/v1/users` (admin only)

```json
{
  "email": "string (required)",
  "password": "string (required, min 8 chars)",
  "full_name": "string (required)",
  "role": "admin" | "manager" | "member" | "viewer"
}
```

Form with email, password, full name, and role dropdown. Note: `free` role is only for self-registration, `service` role is only for service accounts — neither should appear in the create user form.

#### 4.5.3 User Detail / Edit

**API**:
- `GET /api/v1/users/:id` — Self or admin access
- `PUT /api/v1/users/:id` — Self or admin access

```json
{
  "email": "string (optional)",
  "full_name": "string (optional)",
  "role": "string (optional, admin only)",
  "is_active": "boolean (optional, admin only)"
}
```

**Display**:
- User info card (name, email, role, tenant, auth provider)
- Account status (active/inactive toggle — admin only)
- Email verification status
- Document quota usage (for free users): `documents_used_this_period / monthly_document_limit`
- Role change dropdown (admin only — non-admins cannot change roles)
- "Deactivate Account" toggle

#### 4.5.4 Delete User

**API**: `DELETE /api/v1/users/:id` (admin only)

Confirmation dialog with user name/email.

---

### 4.6 Service Account Management

**Route**: `/service-accounts`
**Visibility**: Admin only

#### 4.6.1 Service Account List

**API**: `GET /api/v1/service-accounts?offset=0&limit=20`

**Table columns**: Name, Description, API Key Prefix (`sk_XXXXXXXX...`), Active Status, Last Used At, Expires At, Created At

#### 4.6.2 Create Service Account

**API**: `POST /api/v1/service-accounts`

Handler binds from JSON:
```json
{
  "name": "string (required)",
  "description": "string",
  "expires_at": "ISO 8601 datetime (optional)"
}
```

**Response**:
```json
{
  "service_account": { ...ServiceAccount },
  "api_key": "sk_<64-hex-chars>"
}
```

**CRITICAL UX**: The raw API key (`sk_...`) is shown **only once** at creation time. Build a prominent key display with:
- Copy-to-clipboard button
- Warning: "This key will not be shown again. Save it securely."
- Dismissal requires explicit "I've saved this key" confirmation

#### 4.6.3 Service Account Detail

**Route**: `/service-accounts/:id`
**API**: `GET /api/v1/service-accounts/:id`

**Sections**:

**Info Card**: Name, description, key prefix, status, created by, last used, expiry

**Key Management**:
- "Rotate Key" button: `POST /service-accounts/:id/rotate-key`
  - Returns new API key (show with same "save it now" UX as creation)
  - Old key is immediately invalidated
- "Revoke" button: `POST /service-accounts/:id/revoke`
  - Deactivates the service account (soft delete)

**Collection Permissions**:
- List: `GET /service-accounts/:id/permissions`
  - Returns: Array of `{ id, service_account_id, collection_id, tenant_id, permission, granted_by }`
- Add: `POST /service-accounts/:id/permissions`
  ```json
  { "collection_id": "uuid", "permission": "owner" | "editor" | "viewer" }
  ```
- Remove: `DELETE /service-accounts/:id/permissions/:collectionId`

Build a permissions table showing collection name, permission level, with add/remove controls. Service accounts have NO implicit collection access — all access is via these explicit grants.

#### 4.6.4 Delete Service Account

**API**: `DELETE /api/v1/service-accounts/:id`

Hard delete with confirmation dialog.

---

### 4.7 Reports

**Route**: `/reports`
**Visibility**: All authenticated users (data scoped by role)

All report endpoints share common query parameters:

```
?from=YYYY-MM-DD          # Start date filter
&to=YYYY-MM-DD            # End date filter
&collection_id=<uuid>     # Filter by collection
&seller_gstin=<string>    # Filter by seller GSTIN
&buyer_gstin=<string>     # Filter by buyer GSTIN
&granularity=monthly       # daily|weekly|monthly|quarterly|yearly (for time-series)
&offset=0&limit=20        # Pagination (for table reports)
```

Build a **shared filter bar** component used across all report pages with date range picker, collection dropdown, GSTIN inputs, and granularity selector.

#### 4.7.1 Seller Summary

**API**: `GET /api/v1/reports/sellers`

```typescript
interface SellerSummaryRow {
  seller_gstin: string;
  seller_name: string;
  seller_state: string;
  invoice_count: number;
  total_amount: number;
  total_tax: number;
  cgst: number;
  sgst: number;
  igst: number;
  average_invoice_value: number;
  first_invoice_date: string;
  last_invoice_date: string;
}
```

**Display**: Paginated data table. Sortable columns. Click a seller row to navigate to party ledger pre-filtered by that GSTIN.

#### 4.7.2 Buyer Summary

**API**: `GET /api/v1/reports/buyers`

Same structure as seller summary but for buyers. Same table layout.

#### 4.7.3 Party Ledger

**API**: `GET /api/v1/reports/party-ledger?gstin=<required>`

```typescript
interface PartyLedgerRow {
  document_id: string;
  invoice_number: string;
  invoice_date: string;
  invoice_type: string;
  counterparty_name: string;
  counterparty_gstin: string;
  role: string;                // "seller" or "buyer"
  subtotal: number;
  taxable_amount: number;
  cgst: number;
  sgst: number;
  igst: number;
  total_amount: number;
  validation_status: string;
  review_status: string;
}
```

**Display**: Requires GSTIN input/param. Shows all invoices for that party (as seller or buyer). Table with document links. Running total at bottom.

#### 4.7.4 Financial Summary

**API**: `GET /api/v1/reports/financial-summary`

```typescript
interface FinancialSummaryRow {
  period: string;
  period_start: string;
  period_end: string;
  invoice_count: number;
  subtotal: number;
  taxable_amount: number;
  cgst: number;
  sgst: number;
  igst: number;
  cess: number;
  total_amount: number;
}
```

**Display**: Time-series chart (bar or line) showing total_amount over periods + data table below. Granularity selector controls the period grouping.

#### 4.7.5 Tax Summary

**API**: `GET /api/v1/reports/tax-summary`

```typescript
interface TaxSummaryRow {
  period: string;
  period_start: string;
  period_end: string;
  intrastate_count: number;
  intrastate_taxable: number;
  cgst: number;
  sgst: number;
  interstate_count: number;
  interstate_taxable: number;
  igst: number;
  cess: number;
  total_tax: number;
}
```

**Display**: Stacked bar chart (intrastate CGST+SGST vs interstate IGST per period) + data table. Show intrastate vs interstate breakdown clearly.

#### 4.7.6 HSN Summary

**API**: `GET /api/v1/reports/hsn-summary`

```typescript
interface HSNSummaryRow {
  hsn_code: string;
  description: string;
  invoice_count: number;
  line_item_count: number;
  total_quantity: number;
  taxable_amount: number;
  cgst: number;
  sgst: number;
  igst: number;
  total_tax: number;
}
```

**Display**: Paginated table sorted by taxable_amount descending. HSN code as primary column. Show tax breakdown per HSN code.

#### 4.7.7 Collections Overview

**API**: `GET /api/v1/reports/collections-overview`

```typescript
interface CollectionOverviewRow {
  collection_id: string;
  collection_name: string;
  document_count: number;
  total_amount: number;
  validation_valid_pct: number;      // 0-100
  validation_warning_pct: number;
  validation_invalid_pct: number;
  review_approved_pct: number;
  review_pending_pct: number;
}
```

**Display**: Table with progress bars for validation and review percentages. Click collection name to navigate to collection detail.

---

### 4.8 Tenant Management (Super Admin)

**Route**: `/admin/tenants`
**Visibility**: Admin only
**API base**: `/api/v1/admin/tenants`

#### 4.8.1 Tenant List

**API**: `GET /admin/tenants?offset=0&limit=20`

**Table columns**: Name, Slug, Active Status, Created At, Updated At
**Actions**: Edit, Delete, Toggle Active

#### 4.8.2 Create Tenant

**API**: `POST /admin/tenants`

```json
{
  "name": "string (required)",
  "slug": "string (required, unique)"
}
```

Slug must be URL-safe (lowercase alphanumeric + hyphens). Validate client-side before submission.

#### 4.8.3 Tenant Detail / Edit

**API**:
- `GET /admin/tenants/:id`
- `PUT /admin/tenants/:id`

```json
{
  "name": "string (optional)",
  "slug": "string (optional)",
  "is_active": "boolean (optional)"
}
```

Show tenant details with inline editing. Active/inactive toggle with warning: "Deactivating a tenant will prevent all its users from logging in."

#### 4.8.4 Delete Tenant

**API**: `DELETE /admin/tenants/:id`

**Destructive action**. Double-confirmation dialog: "This will permanently delete the tenant and all its data (users, collections, documents, files). Type the tenant slug to confirm."

---

## 5. TypeScript Types

Define these core types in a shared `types/` directory:

```typescript
// types/api.ts
export interface APIResponse<T> {
  success: boolean;
  data?: T;
  error?: APIError;
  meta?: PaginationMeta;
}

export interface APIError {
  code: string;
  message: string;
}

export interface PaginationMeta {
  total: number;
  offset: number;
  limit: number;
}

// types/auth.ts
export interface TokenPair {
  access_token: string;
  refresh_token: string;
}

export interface LoginRequest {
  tenant_slug: string;
  email: string;
  password: string;
}

export interface RegisterRequest {
  email: string;
  password: string;
  full_name: string;
}

export interface ForgotPasswordRequest {
  tenant_slug: string;
  email: string;
}

export interface ResetPasswordRequest {
  token: string;
  new_password: string;
}

// types/domain.ts
export type UserRole = "admin" | "manager" | "member" | "viewer" | "free" | "service";
export type CollectionPermission = "owner" | "editor" | "viewer";
export type ParsingStatus = "pending" | "processing" | "completed" | "failed" | "queued";
export type ReviewStatus = "pending" | "approved" | "rejected";
export type ValidationStatus = "pending" | "valid" | "invalid" | "warning";
export type ReconciliationStatus = "pending" | "valid" | "invalid" | "warning";
export type ParseMode = "single" | "dual";
export type FileType = "pdf" | "jpg" | "png";
export type FileStatus = "pending" | "uploaded" | "failed" | "deleted";
export type AuthProvider = "email" | "google" | "api_key";

export interface Tenant {
  id: string;
  name: string;
  slug: string;
  is_active: boolean;
  created_at: string;
  updated_at: string;
}

export interface User {
  id: string;
  tenant_id: string;
  email: string;
  full_name: string;
  role: UserRole;
  is_active: boolean;
  monthly_document_limit: number;
  documents_used_this_period: number;
  current_period_start: string;
  email_verified: boolean;
  email_verified_at?: string;
  auth_provider: AuthProvider;
  created_at: string;
  updated_at: string;
}

export interface Collection {
  id: string;
  tenant_id: string;
  name: string;
  description: string;
  created_by: string;
  document_count: number;
  created_at: string;
  updated_at: string;
}

export interface CollectionPermissionEntry {
  id: string;
  collection_id: string;
  tenant_id: string;
  user_id: string;
  permission: CollectionPermission;
  granted_by: string;
  created_at: string;
}

export interface FileMeta {
  id: string;
  tenant_id: string;
  uploaded_by: string;
  file_name: string;
  original_name: string;
  file_type: FileType;
  file_size: number;
  s3_bucket: string;
  s3_key: string;
  content_type: string;
  status: FileStatus;
  created_at: string;
  updated_at: string;
}

export interface Document {
  id: string;
  tenant_id: string;
  collection_id: string;
  file_id: string;
  name: string;
  document_type: string;
  parser_model: string;
  parser_prompt: string;
  structured_data: GSTInvoice | null;
  confidence_scores: ConfidenceScores | null;
  parsing_status: ParsingStatus;
  parsing_error: string;
  parsed_at?: string;
  review_status: ReviewStatus;
  reviewed_by?: string;
  reviewed_at?: string;
  reviewer_notes: string;
  validation_status: ValidationStatus;
  validation_results: ValidationResultEntry[] | null;
  reconciliation_status: ReconciliationStatus;
  parse_mode: ParseMode;
  field_provenance: Record<string, string> | null;
  secondary_parser_model: string;
  parse_attempts: number;
  retry_after?: string;
  assigned_to?: string;
  assigned_at?: string;
  assigned_by?: string;
  created_by: string;
  created_at: string;
  updated_at: string;
}

export interface DocumentTag {
  id: string;
  document_id: string;
  tenant_id: string;
  key: string;
  value: string;
  source: "user" | "auto";
  created_at: string;
}

export interface DocumentAuditEntry {
  id: string;
  tenant_id: string;
  document_id: string;
  user_id?: string;
  action: string;
  changes: Record<string, unknown>;
  created_at: string;
}

export interface ValidationResultEntry {
  rule_id: string;
  passed: boolean;
  field_path: string;
  expected_value: string;
  actual_value: string;
  message: string;
  reconciliation_critical: boolean;
  validated_at: string;
}

export interface ServiceAccount {
  id: string;
  tenant_id: string;
  name: string;
  description: string;
  api_key_prefix: string;
  is_active: boolean;
  created_by: string;
  last_used_at?: string;
  expires_at?: string;
  created_at: string;
  updated_at: string;
}

export interface ServiceAccountPermission {
  id: string;
  service_account_id: string;
  collection_id: string;
  tenant_id: string;
  permission: CollectionPermission;
  granted_by: string;
}

export interface Stats {
  total_documents: number;
  total_collections: number;
  parsing_completed: number;
  parsing_failed: number;
  parsing_processing: number;
  parsing_pending: number;
  parsing_queued: number;
  validation_valid: number;
  validation_warning: number;
  validation_invalid: number;
  reconciliation_valid: number;
  reconciliation_warning: number;
  reconciliation_invalid: number;
  review_pending: number;
  review_approved: number;
  review_rejected: number;
}

// types/invoice.ts — GSTInvoice types (see Section 4.2.3 for full structure)
export interface GSTInvoice {
  invoice: InvoiceHeader;
  seller: Party;
  buyer: Party;
  line_items: LineItem[];
  totals: Totals;
  payment: Payment;
  notes: string;
}

export interface InvoiceHeader {
  invoice_number: string;
  invoice_date: string;
  due_date: string;
  invoice_type: string;
  currency: string;
  place_of_supply: string;
  reverse_charge: boolean;
  irn: string;
  acknowledgement_number: string;
  acknowledgement_date: string;
  qr_code_data: string;
}

export interface Party {
  name: string;
  address: string;
  gstin: string;
  pan: string;
  state: string;
  state_code: string;
}

export interface LineItem {
  description: string;
  hsn_sac_code: string;
  quantity: number;
  unit: string;
  unit_price: number;
  discount: number;
  taxable_amount: number;
  cgst_rate: number;
  cgst_amount: number;
  sgst_rate: number;
  sgst_amount: number;
  igst_rate: number;
  igst_amount: number;
  total: number;
}

export interface Totals {
  subtotal: number;
  total_discount: number;
  taxable_amount: number;
  cgst: number;
  sgst: number;
  igst: number;
  cess: number;
  round_off: number;
  total: number;
  amount_in_words: string;
}

export interface Payment {
  bank_name: string;
  account_number: string;
  ifsc_code: string;
  payment_terms: string;
}

export interface ConfidenceScores {
  invoice: Record<string, number>;
  seller: Record<string, number>;
  buyer: Record<string, number>;
  line_items: Array<Record<string, number>>;
  totals: Record<string, number>;
  payment: Record<string, number>;
}
```

---

## 6. API Client Layer

Build a centralized API client with:

1. **Base HTTP client** — Axios or fetch wrapper with:
   - Base URL configuration (`/api/v1`)
   - Authorization header injection from stored access token
   - Automatic 401 → token refresh → retry logic
   - Request/response interceptors for error normalization
   - Request timeout (30s default, 120s for file uploads)

2. **Service modules** — One per domain:
   - `authApi` — login, refresh, register, verify-email, resend-verification, forgot-password, reset-password, social-login
   - `fileApi` — upload, list, getById, delete
   - `collectionApi` — CRUD, batchUpload, removeFile, setPermission, listPermissions, removePermission, exportCSV, exportTally
   - `documentApi` — CRUD, retry, review, assign, validate, getValidation, listTags, addTags, deleteTag, searchByTag, listAudit, reviewQueue
   - `userApi` — CRUD
   - `tenantApi` — CRUD
   - `serviceAccountApi` — CRUD, rotateKey, revoke, setPermission, listPermissions, removePermission
   - `reportApi` — sellers, buyers, partyLedger, financialSummary, taxSummary, hsnSummary, collectionsOverview
   - `statsApi` — getStats

3. **React Query / SWR** hooks wrapping each API call for caching, revalidation, and optimistic updates.

---

## 7. UI/UX Guidelines

### 7.1 Layout

- **Sidebar navigation** (collapsible):
  - Dashboard
  - Documents (with sub-items: All Documents, Review Queue)
  - Collections
  - Files
  - Reports (with sub-items for each report type)
  - Users (admin only)
  - Service Accounts (admin only)
  - Tenants (admin only)
  - Settings / Profile
- **Top bar**: User avatar + name, tenant name, notification area, logout
- **Breadcrumbs**: For nested navigation (e.g., Collections > Collection Name > Document)

### 7.2 Common Components

- **DataTable**: Sortable, paginated table with row selection, column visibility toggle
- **StatusBadge**: Colored badge component for all status types
- **ConfidenceIndicator**: Visual indicator (green/amber/red) for confidence scores
- **FilterBar**: Reusable date range + collection + GSTIN filter component
- **EmptyState**: Illustration + CTA for empty lists
- **ConfirmDialog**: Reusable confirmation with danger variant for destructive actions
- **UserPicker**: Searchable dropdown to select tenant users
- **CollectionPicker**: Searchable dropdown for collections
- **FileUploadZone**: Drag-and-drop with progress
- **KeyValueEditor**: Add/remove key-value pairs (for tags)

### 7.3 Color Scheme for Statuses

Use consistent colors across the entire app:

| Status | Color | Usage |
|--------|-------|-------|
| Valid / Approved / Completed | Green (#22c55e) | Success states |
| Warning | Amber (#f59e0b) | Warning states |
| Invalid / Rejected / Failed | Red (#ef4444) | Error states |
| Pending | Gray (#9ca3af) | Waiting states |
| Processing / Queued | Blue (#3b82f6) | Active states |

### 7.4 Responsive Design

- Desktop-first but responsive down to tablet (1024px)
- Mobile: Collapse sidebar to hamburger menu, stack cards vertically
- Tables: Horizontal scroll on mobile with sticky first column

---

## 8. Error Handling

### 8.1 Error Codes from Backend

Map these error codes to user-friendly messages and actions:

| Error Code | HTTP | User Message | Action |
|---|---|---|---|
| `NOT_FOUND` | 404 | "Resource not found" | Navigate back or show 404 page |
| `UNAUTHORIZED` | 401 | "Session expired" | Attempt token refresh, then redirect to login |
| `FORBIDDEN` | 403 | "You don't have permission" | Show access denied state |
| `INVALID_CREDENTIALS` | 401 | "Invalid email or password" | Highlight login form fields |
| `TENANT_INACTIVE` | 403 | "Your organization has been deactivated" | Show contact admin message |
| `USER_INACTIVE` | 403 | "Your account has been deactivated" | Show contact admin message |
| `UNSUPPORTED_FILE_TYPE` | 400 | "Only PDF, JPG, and PNG files are supported" | Show in upload error |
| `FILE_TOO_LARGE` | 413 | "File exceeds the 50MB size limit" | Show in upload error |
| `DUPLICATE_EMAIL` | 409 | "An account with this email already exists" | Highlight email field |
| `DUPLICATE_SLUG` | 409 | "This tenant slug is already taken" | Highlight slug field |
| `COLLECTION_PERMISSION_DENIED` | 403 | "You don't have access to this collection" | Show access denied |
| `DOCUMENT_ALREADY_EXISTS` | 409 | "A document already exists for this file" | Link to existing document |
| `DOCUMENT_NOT_PARSED` | 400 | "Document hasn't been parsed yet" | Show parsing status |
| `QUOTA_EXCEEDED` | 429 | "Monthly document limit reached" | Show upgrade prompt |
| `EMAIL_NOT_VERIFIED` | 403 | "Please verify your email first" | Show verification banner with resend button |
| `INVALID_RESET_TOKEN` | 401 | "Reset link is invalid or has been used" | Show "Request new reset" link |
| `PASSWORD_LOGIN_NOT_ALLOWED` | 400 | "This account uses Google sign-in" | Show Google login button |
| `ASSIGNEE_CANNOT_REVIEW` | 400 | "The assigned reviewer cannot approve their own assignment" | Explain constraint |
| `SERVICE_ACCOUNT_REVIEW` | 403 | "Service accounts cannot review documents" | N/A (shouldn't appear in UI) |
| `INVALID_API_KEY` | 401 | N/A | N/A (API key auth, not UI) |
| `API_KEY_REVOKED` | 401 | N/A | N/A (API key auth, not UI) |
| `INTERNAL_ERROR` | 500 | "Something went wrong. Please try again." | Show retry button + error ID |

### 8.2 Client-Side Validation

Validate before API calls:
- Email format
- Password: min 8 characters
- GSTIN format: `^[0-9]{2}[A-Z]{5}[0-9]{4}[A-Z]{1}[1-9A-Z]{1}Z[0-9A-Z]{1}$`
- PAN format: `^[A-Z]{5}[0-9]{4}[A-Z]{1}$`
- UUID format for IDs
- File type (PDF/JPG/PNG) and size (50MB) before upload
- Tenant slug: lowercase alphanumeric + hyphens
- Required fields per form

### 8.3 Loading States

- Skeleton loaders for initial page loads
- Spinner for form submissions
- Progress bar for file uploads
- Disabled buttons during mutations with loading indicator
- Optimistic updates for tag add/remove operations

---

## Summary of All API Endpoints

### Public (No Auth)
| Method | Path | Description |
|--------|------|-------------|
| GET | `/healthz` | Liveness check |
| GET | `/readyz` | Readiness check |
| POST | `/api/v1/auth/login` | Login |
| POST | `/api/v1/auth/refresh` | Refresh tokens |
| POST | `/api/v1/auth/register` | Register free-tier account |
| GET | `/api/v1/auth/verify-email?token=` | Verify email |
| POST | `/api/v1/auth/forgot-password` | Request password reset |
| POST | `/api/v1/auth/reset-password` | Reset password |
| POST | `/api/v1/auth/social-login` | Google social login |

### Authenticated
| Method | Path | Roles | Description |
|--------|------|-------|-------------|
| POST | `/api/v1/auth/resend-verification` | all except service | Resend verification email |
| POST | `/api/v1/files/upload` | admin,manager,member,free,service | Upload file |
| GET | `/api/v1/files` | all | List files |
| GET | `/api/v1/files/:id` | all | Get file + download URL |
| DELETE | `/api/v1/files/:id` | admin | Delete file |
| POST | `/api/v1/collections` | admin,manager,member | Create collection |
| GET | `/api/v1/collections` | all | List collections |
| GET | `/api/v1/collections/:id` | all | Get collection |
| PUT | `/api/v1/collections/:id` | owner | Update collection |
| DELETE | `/api/v1/collections/:id` | owner | Delete collection |
| POST | `/api/v1/collections/:id/files` | editor+ | Batch upload files |
| DELETE | `/api/v1/collections/:id/files/:fileId` | editor+ | Remove file from collection |
| POST | `/api/v1/collections/:id/permissions` | owner | Set permission |
| GET | `/api/v1/collections/:id/permissions` | owner | List permissions |
| DELETE | `/api/v1/collections/:id/permissions/:userId` | owner | Remove permission |
| GET | `/api/v1/collections/:id/export/csv` | viewer+ | Export CSV |
| GET | `/api/v1/collections/:id/export/tally` | viewer+ | Export Tally XML |
| POST | `/api/v1/documents` | admin,manager,member,free,service | Create document |
| GET | `/api/v1/documents` | all | List documents |
| GET | `/api/v1/documents/search/tags` | all | Search by tag |
| GET | `/api/v1/documents/review-queue` | all | My review queue |
| GET | `/api/v1/documents/:id` | viewer+ on collection | Get document |
| PUT | `/api/v1/documents/:id` | editor+ on collection | Edit structured data |
| POST | `/api/v1/documents/:id/retry` | editor+ on collection | Retry parsing |
| PUT | `/api/v1/documents/:id/review` | editor+ on collection | Review (approve/reject) |
| PUT | `/api/v1/documents/:id/assign` | editor+ on collection | Assign for review |
| PUT | `/api/v1/documents/:id/structured-data` | editor+ on collection | Edit structured data (alias) |
| POST | `/api/v1/documents/:id/validate` | viewer+ on collection | Re-run validation |
| GET | `/api/v1/documents/:id/validation` | viewer+ on collection | Get validation results |
| GET | `/api/v1/documents/:id/tags` | viewer+ on collection | List tags |
| POST | `/api/v1/documents/:id/tags` | editor+ on collection | Add tags |
| DELETE | `/api/v1/documents/:id/tags/:tagId` | editor+ on collection | Delete tag |
| GET | `/api/v1/documents/:id/audit` | all | View audit trail |
| DELETE | `/api/v1/documents/:id` | admin | Delete document |
| GET | `/api/v1/stats` | all | Tenant statistics |
| GET | `/api/v1/reports/sellers` | all | Seller summary |
| GET | `/api/v1/reports/buyers` | all | Buyer summary |
| GET | `/api/v1/reports/party-ledger?gstin=` | all | Party ledger |
| GET | `/api/v1/reports/financial-summary` | all | Financial summary |
| GET | `/api/v1/reports/tax-summary` | all | Tax summary |
| GET | `/api/v1/reports/hsn-summary` | all | HSN summary |
| GET | `/api/v1/reports/collections-overview` | all | Collections overview |
| POST | `/api/v1/users` | admin | Create user |
| GET | `/api/v1/users` | admin | List users |
| GET | `/api/v1/users/:id` | self or admin | Get user |
| PUT | `/api/v1/users/:id` | self or admin | Update user |
| DELETE | `/api/v1/users/:id` | admin | Delete user |
| POST | `/api/v1/service-accounts` | admin | Create service account |
| GET | `/api/v1/service-accounts` | admin | List service accounts |
| GET | `/api/v1/service-accounts/:id` | admin | Get service account |
| POST | `/api/v1/service-accounts/:id/rotate-key` | admin | Rotate API key |
| POST | `/api/v1/service-accounts/:id/revoke` | admin | Revoke service account |
| DELETE | `/api/v1/service-accounts/:id` | admin | Delete service account |
| POST | `/api/v1/service-accounts/:id/permissions` | admin | Set SA permission |
| GET | `/api/v1/service-accounts/:id/permissions` | admin | List SA permissions |
| DELETE | `/api/v1/service-accounts/:id/permissions/:collectionId` | admin | Remove SA permission |
| POST | `/api/v1/admin/tenants` | admin | Create tenant |
| GET | `/api/v1/admin/tenants` | admin | List tenants |
| GET | `/api/v1/admin/tenants/:id` | admin | Get tenant |
| PUT | `/api/v1/admin/tenants/:id` | admin | Update tenant |
| DELETE | `/api/v1/admin/tenants/:id` | admin | Delete tenant |
