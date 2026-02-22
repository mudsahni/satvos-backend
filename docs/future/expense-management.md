# Expense Management Module

## Overview

Employee expense reimbursement workflow built on top of existing SATVOS infrastructure. Employees submit receipts/bills, system auto-extracts details via LLM parsing, expenses flow through a configurable multi-level approval chain (manager -> HR -> finance), and approved expenses are exported for reimbursement.

**Product direction**: TBD — could extend SATVOS as a broader document processing platform or become a standalone product sharing the backend codebase.

**Submission method**: Web UI only (initially). Email-based submission (forward receipts) is a natural Phase 2 using the existing Lambda email processing infrastructure.

---

## How It Fits Into SATVOS

### Directly Reusable (No Changes)

| Component | Reuse |
|---|---|
| Auth + JWT + bcrypt | 100% — same auth system |
| Middleware (auth, CORS, logger, tenant guard) | 100% |
| S3 storage (upload, download, presign) | 100% — receipts are just files |
| LLM parsing (claude/gemini/openai providers) | 100% — same `Parse(ctx, input)` interface |
| Multi-parser architecture (fallback, merge, circuit breaker) | 100% |
| Validation engine + registry | 100% — register expense-specific validators |
| File upload + DB (`files` table) | 100% — receipts are files |
| Audit trail pattern | 100% — same append-only pattern |
| Health/readyz endpoints | 100% |
| Config system (viper, env vars) | 100% |
| Service account infrastructure | 100% — ERP integrations |

### Needs Extension

| Component | Change |
|---|---|
| `domain/enums.go` | New roles or permissions for submitter/approver/finance |
| `domain/models.go` | New structs: Department, ExpensePolicy, ExpenseReport, Expense, ApprovalStep |
| `parser/prompt.go` | Add `BuildExpenseReceiptPrompt()` alongside existing `BuildGSTInvoicePrompt()` |
| Validation rules | New `validator/expense/` category (amount limits, date window, category rules) |
| CSV export | Expense report export (different columns than GST) |
| User model | Add `department_id` field |

### Completely New

| Component | Complexity | Notes |
|---|---|---|
| Department/Org hierarchy | Medium | Tree structure, CRUD, user assignment |
| Expense Policy engine | Medium | Configurable rules, approval chain templates, thresholds |
| Approval Chain orchestrator | **High** | State machine, notifications, delegation, escalation |
| Expense Report lifecycle | Medium | draft -> submit -> in_review -> approved -> rejected -> paid |
| Expense-specific handlers | Medium | CRUD, submit, approve/reject per step |
| Notification system | Medium-High | Email when action needed (pending approval, rejected, approved) |
| Dashboard/Stats | Low | Reuse existing stats pattern |

---

## Automation Potential

### Heavily Automatable (existing infrastructure)
- Receipt parsing via LLM — multi-parser architecture handles this directly
- Auto-extraction: vendor name, amount, date, payment method, GST number, category guess
- Employee identity from JWT — no manual entry
- Duplicate detection — same vendor + amount + date pattern
- Format/math validation — GST math, date sanity, amount ranges

### Partially Automatable
- Category classification (LLM guesses "travel", "meals", "office supplies" from receipt content)
- Policy compliance checks (max per-diem, pre-approved vendors, receipt age limits)
- Auto-routing to correct approver based on org hierarchy or amount thresholds
- Auto-approval below configurable amount thresholds

### Not Automatable
- Business justification (employee writes why)
- Exception approvals (over-limit, missing receipt)
- Final payment disbursement (ERP/payroll integration)

---

## Data Model

```
Tenant (existing)
 |
 +-- Department (new)
 |    +-- name, code, parent_department_id (tree structure)
 |    +-- manager_user_id
 |
 +-- ExpensePolicy (new, per-tenant, assignable to departments)
 |    +-- name ("Default", "Executive", "Sales Travel")
 |    +-- categories[] (travel, meals, office, telecom, fuel, medical, other)
 |    +-- per_expense_limit, monthly_limit
 |    +-- receipt_required_above (amount threshold)
 |    +-- submission_window_days (30 = must submit within 30 days of expense date)
 |    +-- approval_chain_template[] (ordered list of approval steps):
 |    |    step 1: { role: "department_manager", auto_skip_below: 500 }
 |    |    step 2: { role: "hr", auto_skip_below: 5000 }
 |    |    step 3: { role: "finance" }
 |    +-- auto_approve_below (amount threshold to skip chain entirely)
 |
 +-- ExpenseReport (new -- groups expenses, analogous to Collection)
 |    +-- title ("Jan 2026 Travel", "Client Visit - Mumbai")
 |    +-- submitter_id, department_id
 |    +-- status: draft -> submitted -> in_review -> approved -> rejected -> paid
 |    +-- total_amount (computed)
 |    +-- submitted_at, approved_at, paid_at
 |
 +-- Expense (new -- individual receipt, analogous to Document)
 |    +-- expense_report_id
 |    +-- file_id (-> existing files table, S3)
 |    +-- structured_data (JSONB -- parsed receipt)
 |    +-- category, description, amount, currency, expense_date
 |    +-- vendor_name, vendor_gstin
 |    +-- parsing_status (reuse existing enum)
 |    +-- validation_results (JSONB -- reuse pattern)
 |    +-- confidence_scores (JSONB)
 |
 +-- ApprovalStep (new -- the core workflow piece)
      +-- expense_report_id
      +-- step_order (1, 2, 3...)
      +-- approver_role ("department_manager", "hr", "finance")
      +-- approver_id (resolved user, nullable until claimed)
      +-- status: pending -> approved -> rejected -> skipped
      +-- notes, decided_at
      +-- auto_approved (bool -- policy threshold skip)
```

### Database Migrations Needed

1. `departments` table (id, tenant_id, name, code, parent_department_id, manager_user_id)
2. `expense_policies` table (id, tenant_id, name, config JSONB for chain template + thresholds)
3. `expense_reports` table (id, tenant_id, submitter_id, department_id, title, status, total_amount, timestamps)
4. `expenses` table (id, tenant_id, expense_report_id, file_id, structured_data JSONB, category, amount, currency, expense_date, parsing_status, validation_results JSONB, confidence_scores JSONB)
5. `approval_steps` table (id, expense_report_id, step_order, approver_role, approver_id, status, notes, decided_at, auto_approved)
6. Add `department_id` column to `users` table

---

## Receipt Parsing

### Prompt Design

Simpler than GST invoice extraction (~500 tokens vs ~2000):

```
Extract from this receipt/bill:
- vendor_name, vendor_address, vendor_gstin (if present)
- date, time
- items[] (description, quantity, unit_price, amount)
- subtotal, tax_amount, tax_rate, total
- payment_method (cash/card/upi/bank_transfer)
- category_guess (travel/meals/office/telecom/fuel/medical/accommodation/other)
```

### Integration Approach

- Add `ParseMode` concept: `"gst_invoice"` vs `"expense_receipt"`
- `BuildExpenseReceiptPrompt()` in `parser/prompt.go` alongside existing `BuildGSTInvoicePrompt()`
- Same `ParseInput`/`ParseOutput` interface — just different prompt and output schema
- Same confidence scores, field provenance, dual-parse merge

---

## Configurable Approval Chain

### How It Works

**1. Template defined on policy:**
```json
{
  "approval_chain_template": [
    {"step": 1, "role": "department_manager", "auto_skip_below": 500},
    {"step": 2, "role": "hr", "auto_skip_below": 5000},
    {"step": 3, "role": "finance"}
  ]
}
```

**2. When employee submits an expense report:**
- System looks up employee's department -> finds applicable policy
- Materializes the chain into concrete `approval_steps` rows
- Step 1 resolves `department_manager` to specific user via department.manager_user_id
- Step 2 resolves `hr` to users with HR role (or specific HR approver for that department)
- Steps where amount is below `auto_skip_below` are auto-approved

**3. Approval flow:**
- Step N must complete before Step N+1 becomes active
- Any rejection at any step -> entire report rejected with notes back to submitter
- Submitter can revise and resubmit -> chain resets
- Each step notifies the approver via email

**4. Edge cases:**
- Manager submits own expense -> skip to step 2 (can't self-approve)
- Approver on leave -> configurable delegate or escalation timeout
- Amount changes after partial approval -> chain resets
- Report total exceeds policy while individual expenses don't -> policy check at report level

---

## Expense-Specific Validation Rules

| Category | Examples |
|---|---|
| Required Fields | amount, date, vendor_name, category |
| Format | date not in future, amount > 0, valid currency code |
| Policy | amount <= per_expense_limit, within submission_window_days |
| Duplicate | same vendor + amount + date within tenant |
| Cross-field | receipt date matches claimed date, GST math on tax |
| Category | fuel receipts need odometer, travel needs origin/destination |

---

## API Endpoints (Draft)

```
# Departments
GET    /api/v1/departments
POST   /api/v1/departments                    (admin)
PUT    /api/v1/departments/:id                 (admin)
DELETE /api/v1/departments/:id                 (admin)

# Expense Policies
GET    /api/v1/expense-policies
POST   /api/v1/expense-policies                (admin)
PUT    /api/v1/expense-policies/:id            (admin)
DELETE /api/v1/expense-policies/:id            (admin)

# Expense Reports
GET    /api/v1/expense-reports                  (own reports, or all if approver/admin)
POST   /api/v1/expense-reports                  (create draft)
GET    /api/v1/expense-reports/:id
PUT    /api/v1/expense-reports/:id              (update draft)
POST   /api/v1/expense-reports/:id/submit       (submit for approval)
DELETE /api/v1/expense-reports/:id              (delete draft only)
GET    /api/v1/expense-reports/:id/export/csv

# Expenses (within a report)
POST   /api/v1/expense-reports/:id/expenses     (upload receipt + create)
GET    /api/v1/expense-reports/:id/expenses
GET    /api/v1/expense-reports/:id/expenses/:eid
PUT    /api/v1/expense-reports/:id/expenses/:eid (edit parsed data)
DELETE /api/v1/expense-reports/:id/expenses/:eid

# Approval
GET    /api/v1/approvals/pending                (my pending approvals)
POST   /api/v1/expense-reports/:id/approve      (approve current step)
POST   /api/v1/expense-reports/:id/reject       (reject with notes)

# Reporting
GET    /api/v1/expense-reports/summary           (by department, period, category)
```

---

## Effort Estimate

| Phase | Scope | Effort (relative to existing GST codebase) |
|---|---|---|
| Phase 1: Data model + migrations | Departments, policies, expenses, approval steps tables | ~15% |
| Phase 2: Receipt parsing | New prompt template, reuse existing parser infra | ~5% |
| Phase 3: Expense CRUD + reports | Handlers, services, repos for expenses + reports | ~20% |
| Phase 4: Approval chain engine | State machine, role resolution, notifications | ~30% |
| Phase 5: Policy engine | Configurable rules, threshold logic, auto-approve | ~15% |
| Phase 6: Export + reporting | Expense reports, accounting summaries, CSV | ~10% |

**Total: ~40-50% of effort already invested in GST processing side.** Building standalone would be roughly 2x due to reimplementing shared infrastructure.

---

## Risks and Concerns

### 1. Role Model Tension
Current 6-tier linear hierarchy (admin > manager > member > viewer > free > service) doesn't map to an org tree. Expenses need "my manager" relationships, not global role levels. Likely need a parallel department-based permission model alongside the existing collection-based one.

### 2. Notification System Gap
GST processing is admin-driven (small team, synchronous workflow). Expense approval is async and employee-facing, requiring notifications ("You have 3 expenses pending approval", "Your expense report was rejected"). SES exists but there's no notification framework (templates, preferences, batching).

### 3. Mobile Considerations
Expense submission is often mobile-first (photo of receipt at restaurant). Current UI is web-only. API design should be mobile-friendly from day one (single-file upload + parse in one call, camera-friendly).

### 4. Accounting Integration
Expenses need to flow into accounting systems (Tally, SAP, QuickBooks). Tally export exists but expense accounting entries (reimbursement vouchers) differ from purchase vouchers. May need new export formats.

### 5. Multi-Currency
International travel requires currency conversion. Not needed for domestic-only GST invoices. Exchange rate source and conversion timing need design.

### 6. Receiptless Expenses
Some expenses lack paper receipts (per-diem, mileage, parking). Need a "manual entry without file" path. Current document flow assumes a file always exists (file_id is required).

### 7. User Scale
GST processing: 5-20 users per tenant. Expense management: potentially hundreds/thousands of employees. Performance implications for approval queue queries, report aggregation, notification volume.

---

## Open Questions

### Product
1. Is this the same product or a separate product sharing infrastructure?
2. Same pricing or different tier?
3. Same tenant or separate tenant per "mode"?

### Data Model
4. Should departments support a tree hierarchy (nested departments) or flat list?
5. Can an employee belong to multiple departments?
6. Should expense policies be assignable per-department, per-role, or per-user?
7. Do we need cost centers separate from departments?
8. Should expenses support multiple currencies within one report?

### Approval Flow
9. Can approval steps be added/removed after submission (dynamic chain)?
10. Should there be a time-based escalation (auto-approve after N days if approver doesn't act)?
11. Can a rejected report be edited and resubmitted, or must a new one be created?
12. Should approvers see all pending approvals across departments, or only their department?
13. How to handle the "manager approves their own expense" conflict?
14. Should delegation (out-of-office approver) be manual or automatic?

### Integration
15. What accounting export formats are needed beyond Tally?
16. Should there be a payroll/ERP webhook for approved expenses?
17. Do we need OCR as a fallback when LLM parsing fails on poor-quality receipt photos?

### Policy
18. Should policies support "pre-approval" for large expenses (request before spending)?
19. Should there be a global admin override to approve any expense regardless of chain?
20. How granular should category rules be (e.g., different meal limits for different cities)?

### Notifications
21. Email only, or also in-app notifications?
22. Should approvers get daily digests or real-time per-expense notifications?
23. Should submitters be notified at each approval step or only at final resolution?
