# User Groups — Future Feature Design

## Summary

Allow enterprise tenants to create groups of users. Collections can be owned/assigned to groups. Documents can be assigned to and reviewed by group members. Permissions granted to a group cascade to all members.

## Value Proposition

- **Permission management at scale**: Grant a group access to a collection instead of N individual users
- **Onboarding/offboarding**: Add/remove user from group → access to all group collections changes instantly
- **Collective ownership**: Collection owned by a group survives individual user deactivation
- **Review throughput**: Assign document to a group → any member can pick it up (no bottleneck on one reviewer)
- **Org structure alignment**: "Finance Team can edit purchase collections" maps to how admins think

## When to Build

Build when:
- Multiple enterprise tenants have 15+ users
- Admins report spending significant time managing individual permissions
- Customers specifically request team-based access control

Interim solution (build now if needed): bulk permission grants endpoint — multi-select users, one API call, no new tables.

## Data Model

### New Tables

```sql
-- Groups within a tenant
CREATE TABLE groups (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   UUID NOT NULL REFERENCES tenants(id),
    name        VARCHAR(255) NOT NULL,
    description TEXT DEFAULT '',
    created_by  UUID NOT NULL REFERENCES users(id),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(tenant_id, name)
);

-- Group membership
CREATE TABLE group_memberships (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    group_id   UUID NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
    user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    tenant_id  UUID NOT NULL REFERENCES tenants(id),
    added_by   UUID NOT NULL REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(group_id, user_id)
);

-- Group-level collection permissions
CREATE TABLE collection_group_permissions (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    collection_id UUID NOT NULL REFERENCES collections(id) ON DELETE CASCADE,
    group_id      UUID NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
    tenant_id     UUID NOT NULL REFERENCES tenants(id),
    permission    VARCHAR(20) NOT NULL, -- owner, editor, viewer
    granted_by    UUID NOT NULL REFERENCES users(id),
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(collection_id, group_id)
);
```

### Permission Resolution Change

Current: `effective_perm = max(role_implicit, user_explicit)`

With groups: `effective_perm = max(role_implicit, user_explicit, max(all_group_grants))`

This affects every permission check in the system.

## API Endpoints

```
POST   /api/v1/groups                           — Create group (admin)
GET    /api/v1/groups                           — List groups (admin)
GET    /api/v1/groups/:id                       — Get group details
PUT    /api/v1/groups/:id                       — Update group name/description
DELETE /api/v1/groups/:id                       — Delete group (cascades memberships + permissions)

POST   /api/v1/groups/:id/members               — Add member(s) to group
DELETE /api/v1/groups/:id/members/:userId        — Remove member from group
GET    /api/v1/groups/:id/members               — List group members

POST   /api/v1/collections/:id/group-permissions — Grant group permission on collection
DELETE /api/v1/collections/:id/group-permissions/:groupId — Revoke
GET    /api/v1/collections/:id/group-permissions — List group permissions on collection
```

Document assignment: extend `PUT /documents/:id/assign` to accept `group_id` in addition to `user_id`.

## Affected Existing Code

| Area | Impact |
|------|--------|
| `collection_service.go` | EffectivePermission(s) must join through group memberships |
| `document_service.go` | Assignment, review queue, access checks |
| `report_repo.go` | All 7 report queries — viewer scoping subquery needs group join |
| `csvexport/writer.go` | Permission gate before export |
| `tallyexport/writer.go` | Permission gate before export |
| `stats_repo.go` | Role-filtered aggregation scoping |
| `middleware/auth.go` | Possibly inject group IDs into context for faster lookups |
| `handler/response.go` | New error mappings |
| `router.go` | New route group |
| `domain/models.go` | Group, GroupMembership, CollectionGroupPermission models |
| `port/` | New repository interfaces (GroupRepo, GroupMembershipRepo, CollectionGroupPermissionRepo) |
| `domain/enums.go` | Possibly new audit actions (group.created, group.member_added, etc.) |
| `document_audit_log` | Record group context on group-based actions |

Estimated: 15+ modified files, 3+ new files, 1 migration, ~2-3 weeks of work.

## Open Questions — Must Answer Before Implementation

### Permission Model
1. **Max or restrictive?** If a user is in Group A (editor) and Group B (viewer) on the same collection, is effective permission editor (max) or viewer (most restrictive)? Current convention is max — do we keep it?
2. **Can groups be granted permissions on collections they didn't create?** (Presumably yes, same as users.)
3. **Should group permissions override role-based implicit access?** E.g., admin has implicit owner — does a group viewer grant matter? (No, max() means admin stays owner.)

### Group Management
4. **Who can create groups?** Admin only? Managers?
5. **Who can add/remove members?** Only admin? Group creator? Anyone in the group?
6. **Should groups have internal roles?** (e.g., group admin vs group member) Or is membership flat?
7. **Can a user be in multiple groups?** (Presumably yes.)
8. **Nested groups?** Can Group A contain Group B? (Recommend: no, at least initially. Adds recursive resolution complexity.)
9. **Maximum group size?** Any limits?

### Document Assignment
10. **Assign to group semantics**: Does assigning a document to a group mean all members see it in their review queue? Or does one member need to "claim" it?
11. **Claim mechanism**: If we use a claim model, what happens to unclaimed documents? Timeout and re-queue?
12. **Can an individual still be assigned directly?** (Presumably yes — group assignment is additive, not replacing individual assignment.)
13. **ErrAssigneeCannotReview**: If a group member assigned the document, can OTHER members of the same group review it? (Probably yes — the constraint is on the individual assigner, not the group.)

### Review Workflow
14. **Who reviewed?** Audit trail must record the individual user AND the group context. Schema: `reviewed_by=userID, reviewed_as_member_of=groupID`?
15. **Can multiple group members review the same document?** Or does the first review lock it?

### Service Accounts
16. **Can service accounts be group members?** (Recommend: no. SAs have separate permission table and shouldn't be in human groups.)

### Free Tier
17. **Are groups available to free tier?** (Recommend: no. Enterprise-only feature.)

### Performance
18. **Permission check cost**: Every collection access check adds a group membership join. At what scale does this need caching? Index strategy for `group_memberships(user_id, tenant_id)`?
19. **EffectivePermissions batch query**: Currently one query. With groups, need UNION or subquery. Benchmark at 50 groups x 100 collections.

### Frontend
20. **Group picker UX**: How does admin grant group permissions? Dropdown? Autocomplete? Separate tab?
21. **Review queue**: Show group-assigned documents differently? Badge indicating "assigned to your group" vs "assigned to you"?
22. **User profile**: Show group memberships?

### Migration
23. **Existing permissions**: No migration of existing data needed (groups are additive). But EffectivePermission query must remain backward-compatible with group tables empty.

## Interim Alternative: Bulk Permission Grants

If the immediate need is "grant multiple users access quickly" without the full group model:

```
POST /api/v1/collections/:id/permissions/bulk
{
  "user_ids": ["uuid1", "uuid2", "uuid3"],
  "permission": "editor"
}
```

- No new tables
- No permission resolution changes
- No cascade across reports/exports
- ~50 lines of code (handler + service loop)
- Covers 80% of the use case
