# SATVOS Admin Pages — Frontend Implementation Prompt

Add admin pages to the existing SATVOS Next.js + TypeScript frontend. These pages are **only visible to users with the `admin` role** and live under an `/admin` route group in the app. The regular user-facing pages (documents, collections, files, reports, dashboard) already exist — this is purely the admin control panel.

All API responses use this envelope:
```typescript
interface APIResponse<T> {
  success: boolean;
  data?: T;
  error?: { code: string; message: string };
  meta?: { total: number; offset: number; limit: number };
}
```

All endpoints require `Authorization: Bearer <access_token>`. Pagination uses `?offset=0&limit=20` (max 100).

---

## 0. Admin Scoping Model — IMPORTANT

The backend has a **single `admin` role** — there is no separate "super admin" or "platform admin" concept. However, admin operations fall into two distinct scoping levels:

### Platform-level (cross-tenant) — Tenant Management

The **tenant CRUD endpoints** (`/api/v1/admin/tenants/*`) are **not scoped to any tenant**. They operate across the entire platform:
- `GET /admin/tenants` returns **ALL tenants** in the system, not just the caller's
- `POST/PUT/DELETE /admin/tenants/:id` can create/modify/delete **any tenant**
- The handler does NOT read `tenant_id` from the JWT — it's truly global

This means any user with the `admin` role in **any** tenant can manage **all** tenants. This is by design — these are platform-level operations.

### Tenant-level (scoped) — Everything Else

All other admin operations are **scoped to the admin's own tenant** via the `tenant_id` in their JWT:
- **User management** (`/api/v1/users/*`) — Admin only sees and manages users in their own tenant
- **Service accounts** (`/api/v1/service-accounts/*`) — Scoped to the admin's tenant
- **Collection permissions** (`/api/v1/collections/:id/permissions/*`) — Collections belong to a tenant
- **Stats** (`/api/v1/stats`) — Returns stats for the admin's own tenant
- **Delete file/document** — Scoped to the admin's tenant

### Frontend Implications

Since the backend treats all admins the same, the frontend should present **both levels** on the admin pages but make the distinction clear:

1. **Tenant Management section** — Label it as "Platform Administration" or similar. Show a subtle info banner: "These settings affect the entire platform, not just your organization." This signals that the admin is operating outside their tenant boundary.

2. **User / Service Account / Permissions sections** — Label as "Organization Administration" or "Tenant Settings." These naturally scope to the admin's own tenant — the backend enforces it.

3. **Current tenant indicator** — In the admin sidebar or header, show which tenant the admin belongs to (from JWT `tenant_id`). For tenant-scoped pages, show "Managing: <tenant_name>". For the tenant list page, show "Platform-wide".

The JWT claims contain:
```typescript
interface JWTClaims {
  tenant_id: string;   // The admin's own tenant
  user_id: string;
  email: string;
  role: "admin";       // Will always be "admin" on these pages
}
```

---

## 1. Tenant Management (Platform-Level)

**Route**: `/admin/tenants`
**Scope**: Cross-tenant — operates on ALL tenants in the platform

Show a banner at the top: "Platform Administration — changes here affect all tenants."

Admins manage the platform's tenant organizations. Each tenant is an isolated workspace with its own users, collections, and documents.

### 1.1 Tenant List Page

**API**: `GET /api/v1/admin/tenants?offset=0&limit=20`

**Table columns**:
| Column | Type | Notes |
|--------|------|-------|
| Name | string | Tenant display name |
| Slug | string | URL-safe identifier, used in login (`tenant_slug` field) |
| Status | badge | Active (green) / Inactive (red) |
| Created | date | `created_at` |

**Actions per row**: Edit, Toggle Active/Inactive, Delete

### 1.2 Create Tenant

**API**: `POST /api/v1/admin/tenants`

```json
{ "name": "Acme Corp", "slug": "acme-corp" }
```

**Form**:
- Name (required, text)
- Slug (required, text — validate: lowercase alphanumeric + hyphens, no spaces)

**Error**: `DUPLICATE_SLUG` (409) — "This tenant slug is already taken"

### 1.3 Edit Tenant

**API**: `PUT /api/v1/admin/tenants/:id`

```json
{ "name": "New Name", "slug": "new-slug", "is_active": false }
```

All fields optional (partial update). Show a toggle switch for `is_active` with a warning: "Deactivating prevents all users in this tenant from logging in."

### 1.4 Delete Tenant

**API**: `DELETE /api/v1/admin/tenants/:id`

Destructive — cascades to all users, collections, documents, files in the tenant. Require a type-to-confirm dialog: "Type the tenant slug `<slug>` to confirm deletion."

### 1.5 Tenant Detail (read)

**API**: `GET /api/v1/admin/tenants/:id`

```typescript
interface Tenant {
  id: string;          // UUID
  name: string;
  slug: string;
  is_active: boolean;
  created_at: string;  // ISO 8601
  updated_at: string;
}
```

---

## 2. User Management (Tenant-Level)

**Route**: `/admin/users`
**Scope**: Own tenant only — backend filters by `tenant_id` from the admin's JWT

Admins manage users within their own tenant. The API automatically scopes all queries to the admin's tenant — there is no way to see or manage users from other tenants through these endpoints. Users are created by admins (except free-tier who self-register).

### 2.1 User List Page

**API**: `GET /api/v1/users?offset=0&limit=20` (admin only)

**Table columns**:
| Column | Type | Notes |
|--------|------|-------|
| Full Name | string | |
| Email | string | |
| Role | badge | Color-coded: admin (purple), manager (blue), member (teal), viewer (gray), free (green) |
| Status | badge | Active / Inactive |
| Email Verified | icon | Checkmark or X |
| Auth Provider | badge | `email` or `google` |
| Doc Usage | text | `documents_used_this_period / monthly_document_limit` (show for free users, "Unlimited" for others) |
| Created | date | |

**Filters** (client-side or future API): Role dropdown, Active/Inactive toggle

### 2.2 Create User

**API**: `POST /api/v1/users` (admin only)

```json
{
  "email": "user@example.com",
  "password": "min8chars",
  "full_name": "Jane Smith",
  "role": "member"
}
```

**Form**:
- Email (required, email format)
- Password (required, min 8 chars — show strength indicator)
- Full Name (required)
- Role (required, dropdown): `admin`, `manager`, `member`, `viewer`
  - Do NOT show `free` (self-registration only) or `service` (service accounts only) in this dropdown

**Error**: `DUPLICATE_EMAIL` (409) — "An account with this email already exists"

### 2.3 Edit User

**API**: `PUT /api/v1/users/:id` (admin access)

```json
{
  "email": "new@email.com",
  "full_name": "New Name",
  "role": "manager",
  "is_active": false
}
```

All fields optional. Show:
- **Role selector**: dropdown (admin, manager, member, viewer). Show warning when changing role: "Changing role affects this user's access to all collections"
- **Active toggle**: with warning "Deactivating prevents this user from logging in"
- **Email verified**: read-only display (cannot be manually set by admin)
- **Auth provider**: read-only (email or google)
- **Quota info** (for free users): `documents_used_this_period` / `monthly_document_limit`, period start date

### 2.4 View User Detail

**API**: `GET /api/v1/users/:id` (admin or self)

```typescript
interface User {
  id: string;
  tenant_id: string;
  email: string;
  full_name: string;
  role: "admin" | "manager" | "member" | "viewer" | "free" | "service";
  is_active: boolean;
  monthly_document_limit: number;
  documents_used_this_period: number;
  current_period_start: string;
  email_verified: boolean;
  email_verified_at?: string;
  auth_provider: "email" | "google" | "api_key";
  created_at: string;
  updated_at: string;
}
```

### 2.5 Delete User

**API**: `DELETE /api/v1/users/:id` (admin only)

Confirmation dialog showing user name and email.

### 2.6 Role Hierarchy Reference

Display this in a help tooltip or info panel so the admin understands what each role grants:

| Role | Level | Implicit Collection Access | Key Capabilities |
|------|-------|---------------------------|-----------------|
| admin | 4 | Owner on ALL collections | Full control, user management, tenant management, delete anything |
| manager | 3 | Editor on ALL collections | Upload, create/edit/validate docs, manage tags, cannot manage users |
| member | 2 | Viewer on ALL collections | Upload, create docs (with email verified), read access |
| viewer | 1 | None | Read-only, needs explicit collection grants to see anything |
| free | 0 | None | Self-registered, quota-limited, only personal collection |

---

## 3. Collection Permission Management (Tenant-Level)

**Route**: `/admin/permissions` or accessible from collection detail pages
**Scope**: Own tenant only — collections belong to the admin's tenant

This is how admins control who can access which collections within their tenant. Admins have implicit owner access to all collections in their tenant, but viewer/free users need explicit grants.

### 3.1 View Collection Permissions

**API**: `GET /api/v1/collections/:id/permissions?offset=0&limit=20`

Returns permissions for a specific collection:

```typescript
interface CollectionPermissionEntry {
  id: string;
  collection_id: string;
  tenant_id: string;
  user_id: string;
  permission: "owner" | "editor" | "viewer";
  granted_by: string;   // UUID of granting user
  created_at: string;
}
```

**Display as table**:
| User | Email | Explicit Permission | Effective Permission | Granted By | Date |
|------|-------|-------------------|---------------------|-----------|------|

Note: "Effective Permission" = `max(implicit_from_role, explicit_grant)`. So a `manager` user always has at least `editor` even without an explicit grant. Display both so the admin understands the distinction.

### 3.2 Grant / Change Permission

**API**: `POST /api/v1/collections/:id/permissions`

```json
{ "user_id": "uuid", "permission": "editor" }
```

**UI**: User picker (searchable dropdown of tenant users) + permission dropdown (`owner`, `editor`, `viewer`). If the user already has a grant, this upserts it.

Validate: target user must belong to the same tenant (backend enforces this).

### 3.3 Remove Permission

**API**: `DELETE /api/v1/collections/:id/permissions/:userId`

Cannot remove your own permission (backend returns `SELF_PERMISSION_REMOVAL`). Show confirmation dialog.

### 3.4 Bulk Permission View (Admin Overview)

Consider building an admin-only overview page that shows a matrix or list:
- All collections in the tenant (`GET /api/v1/collections`)
- Per collection, count of explicit permission grants
- Quick link to manage each collection's permissions

This helps admins answer: "Who has access to what?"

---

## 4. Service Account Management (Tenant-Level)

**Route**: `/admin/service-accounts`
**Scope**: Own tenant only — backend filters by `tenant_id` from the admin's JWT

Service accounts are non-human API identities for programmatic access (ERP integrations, automation, batch processing). They authenticate via API keys, not JWT. Each service account belongs to a single tenant.

### 4.1 Service Account List

**API**: `GET /api/v1/service-accounts?offset=0&limit=20` (admin only)

**Table columns**:
| Column | Type | Notes |
|--------|------|-------|
| Name | string | Account name |
| Description | string | Purpose description |
| Key Prefix | code | `sk_XXXXXXXX...` (first 8 chars for identification) |
| Status | badge | Active (green) / Revoked (red) |
| Last Used | date | `last_used_at` — shows when the key was last used for an API call |
| Expires | date | `expires_at` — null = never expires |
| Created | date | |

### 4.2 Create Service Account

**API**: `POST /api/v1/service-accounts`

```json
{
  "name": "ERP Integration",
  "description": "Uploads invoices from Tally ERP",
  "expires_at": "2025-12-31T23:59:59Z"   // optional, RFC3339
}
```

**Response**:
```json
{
  "service_account": { ...ServiceAccount },
  "api_key": "sk_<64-hex-chars>"
}
```

**CRITICAL UX**: The raw API key is returned **only once**. After creation, display:
- The full key in a monospace read-only field
- A "Copy to clipboard" button
- A warning banner: "Save this API key now. It will not be shown again."
- A checkbox or button: "I've saved this key" that must be confirmed before dismissing

**Form**:
- Name (required)
- Description (optional)
- Expiry date (optional date picker — leave blank for no expiry)

### 4.3 Service Account Detail

**API**: `GET /api/v1/service-accounts/:id`

```typescript
interface ServiceAccount {
  id: string;
  tenant_id: string;
  name: string;
  description: string;
  api_key_prefix: string;   // "sk_XXXXXXXX" — first 8 chars
  is_active: boolean;
  created_by: string;       // UUID
  last_used_at?: string;
  expires_at?: string;
  created_at: string;
  updated_at: string;
}
```

**Sections on detail page**:

1. **Info card**: Name, description, key prefix, status, creator, last used, expiry
2. **Key actions**: Rotate Key, Revoke (see below)
3. **Collection permissions** (see 4.5)

### 4.4 Key Management Actions

**Rotate Key**: `POST /api/v1/service-accounts/:id/rotate-key`
- Generates a new API key, immediately invalidates the old one
- Response includes new key — show with same "save it now" UX as creation
- Confirmation dialog: "The current key will stop working immediately"

**Revoke**: `POST /api/v1/service-accounts/:id/revoke`
- Deactivates the service account. All API calls with this key will fail
- Confirmation dialog: "This service account will be deactivated. Any integrations using this key will break."

**Delete**: `DELETE /api/v1/service-accounts/:id`
- Permanent deletion. Confirmation dialog.

### 4.5 Service Account Permissions

Service accounts have **no implicit collection access** (unlike regular users whose role grants implicit access). All access must be explicitly granted.

**List**: `GET /api/v1/service-accounts/:id/permissions`

```typescript
interface ServiceAccountPermission {
  id: string;
  service_account_id: string;
  collection_id: string;
  tenant_id: string;
  permission: "owner" | "editor" | "viewer";
  granted_by: string;
}
```

**Grant**: `POST /api/v1/service-accounts/:id/permissions`
```json
{ "collection_id": "uuid", "permission": "editor" }
```

**Remove**: `DELETE /api/v1/service-accounts/:id/permissions/:collectionId`

**Display**: Table of granted collections with name, permission level, and remove button. "Add Collection" button opens a collection picker + permission dropdown.

### 4.6 Service Account Restrictions

Display these as an info box on the service accounts page so admins understand the boundaries:
- Cannot log into the UI (API key auth only)
- Cannot review documents (approve/reject)
- Cannot be assigned documents for review
- Cannot create collections
- Cannot manage users or tenants
- Can only access collections with explicit permission grants
- File listing filtered to files uploaded by this service account

---

## 5. Admin Overview Dashboard

**Route**: `/admin` (admin landing page)

Provide an at-a-glance admin summary split into two sections.

### 5.1 Platform Section

**API**: `GET /api/v1/admin/tenants?limit=1` (read `meta.total`)

**Cards**:
- Total Tenants (link to tenant list) — count from tenants list meta
- Highlight the admin's own tenant with a "Your tenant" label

### 5.2 Organization Section (Tenant-Scoped)

**APIs**: `GET /api/v1/stats`, `GET /api/v1/users?limit=1`, `GET /api/v1/service-accounts?limit=1`

Show the current tenant name as section header.

**Cards**:
- Total Users (link to user list) — from users list `meta.total`
- Total Collections — from `stats.total_collections`
- Total Service Accounts (link to SA list) — from service accounts list `meta.total`
- Documents with Parsing Failures (`stats.parsing_failed`) — link to documents filtered by failed
- Documents Pending Review (`stats.review_pending`) — link to documents
- Validation Issues (`stats.validation_invalid + stats.validation_warning`)

---

## 6. Navigation

Add an "Admin" section to the existing sidebar, **visible only when `user.role === "admin"`**. Group items by scope so the admin understands what's platform-wide vs tenant-scoped:

```
Admin
├── Overview                → /admin
│
├── Platform                          (section header, no link)
│   └── Tenants             → /admin/tenants
│
└── Organization                      (section header, shows current tenant name)
    ├── Users               → /admin/users
    ├── Service Accounts    → /admin/service-accounts
    └── Permissions         → /admin/permissions
```

The "Organization" section header should display the current tenant name (decode from JWT or fetch from `/api/v1/admin/tenants/:id` using the `tenant_id` from the token). This reinforces that these pages are scoped to the admin's own tenant.

---

## 7. Shared Admin Components

### UserPicker
Searchable dropdown to select a user from the tenant. Fetch from `GET /api/v1/users?limit=100`. Display as `Full Name (email)`. Used in: permission grants, assignment.

### CollectionPicker
Searchable dropdown for collections. Fetch from `GET /api/v1/collections?limit=100`. Display as collection name with document count. Used in: service account permissions.

### ConfirmDialog
Reusable confirmation modal. Variants:
- **Standard**: "Are you sure?" with Cancel/Confirm
- **Destructive**: Red-themed with type-to-confirm for dangerous operations (delete tenant, delete user)

### StatusBadge
Colored badge for statuses. Use consistently:
- Active / green
- Inactive / Revoked / red
- Role badges with role-specific colors (see 2.1)

---

## 8. Error Handling for Admin Pages

Map these backend error codes to admin-friendly messages:

| Code | HTTP | Message |
|------|------|---------|
| `DUPLICATE_EMAIL` | 409 | "A user with this email already exists in this tenant" |
| `DUPLICATE_SLUG` | 409 | "This tenant slug is already taken" |
| `NOT_FOUND` | 404 | "Resource not found" |
| `FORBIDDEN` | 403 | "You don't have permission for this action" |
| `INSUFFICIENT_ROLE` | 403 | "This action requires a higher role" |
| `SELF_PERMISSION_REMOVAL` | 400 | "You cannot remove your own permission" |
| `INVALID_PERMISSION` | 400 | "Permission must be: owner, editor, or viewer" |
| `SERVICE_ACCOUNT_NOT_FOUND` | 404 | "Service account not found" |
| `TENANT_INACTIVE` | 403 | "This tenant is deactivated" |
| `USER_INACTIVE` | 403 | "This user account is deactivated" |
| `INTERNAL_ERROR` | 500 | "Something went wrong. Please try again." |

---

## 9. Admin API Endpoint Reference

All admin-only endpoints (require `admin` role):

### Platform-Level (cross-tenant, no tenant scoping)

| Method | Path | Description |
|--------|------|-------------|
| POST | `/api/v1/admin/tenants` | Create tenant |
| GET | `/api/v1/admin/tenants` | List ALL tenants |
| GET | `/api/v1/admin/tenants/:id` | Get any tenant |
| PUT | `/api/v1/admin/tenants/:id` | Update any tenant |
| DELETE | `/api/v1/admin/tenants/:id` | Delete any tenant |

### Tenant-Level (scoped to admin's own tenant via JWT `tenant_id`)

| Method | Path | Description |
|--------|------|-------------|
| **Users** | | |
| POST | `/api/v1/users` | Create user in own tenant |
| GET | `/api/v1/users` | List users in own tenant |
| GET | `/api/v1/users/:id` | Get user (own tenant, also self-access for non-admins) |
| PUT | `/api/v1/users/:id` | Update user (own tenant, also self for name/email) |
| DELETE | `/api/v1/users/:id` | Delete user from own tenant |
| **Collection Permissions** | | |
| GET | `/api/v1/collections/:id/permissions` | List permissions (owner access, own tenant) |
| POST | `/api/v1/collections/:id/permissions` | Grant permission (owner access, own tenant) |
| DELETE | `/api/v1/collections/:id/permissions/:userId` | Remove permission (owner access, own tenant) |
| **Service Accounts** | | |
| POST | `/api/v1/service-accounts` | Create SA in own tenant |
| GET | `/api/v1/service-accounts` | List SAs in own tenant |
| GET | `/api/v1/service-accounts/:id` | Get SA (own tenant) |
| POST | `/api/v1/service-accounts/:id/rotate-key` | Rotate API key (own tenant) |
| POST | `/api/v1/service-accounts/:id/revoke` | Revoke SA (own tenant) |
| DELETE | `/api/v1/service-accounts/:id` | Delete SA (own tenant) |
| POST | `/api/v1/service-accounts/:id/permissions` | Grant SA collection access (own tenant) |
| GET | `/api/v1/service-accounts/:id/permissions` | List SA collection access (own tenant) |
| DELETE | `/api/v1/service-accounts/:id/permissions/:collectionId` | Remove SA collection access (own tenant) |
| **Supporting** | | |
| GET | `/api/v1/stats` | Stats for own tenant |
| GET | `/api/v1/collections` | List collections in own tenant (for pickers) |
| DELETE | `/api/v1/files/:id` | Delete file (own tenant) |
| DELETE | `/api/v1/documents/:id` | Delete document (own tenant) |
