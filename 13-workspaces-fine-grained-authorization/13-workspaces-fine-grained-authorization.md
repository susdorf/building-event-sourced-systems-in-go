# Workspaces: Fine-Grained Authorization

*This is Part 13 of the "Building Event-Sourced Systems in Go" series. This series originated from the development of comby – an application framework developed using the principles of Event Sourcing and Command Query Responsibility Segregation (CQRS) - which ist still closed source today. Code with concepts and techniques is presented, but unlike comby, it is not always suitable for product use.*

---

## Introduction

In the previous post, we implemented multi-tenancy to isolate organizations from each other. But real-world applications often require more granular access control within an organization. Consider a software company where different teams work on different projects, or a consulting firm where client engagements must remain strictly separated even though they belong to the same organization.

**Workspaces** provide this additional authorization layer within tenants. They enable multi-project and multi-team scenarios without the overhead of managing separate tenants for each logical boundary. This post explores how to implement workspace-based authorization while maintaining backwards compatibility with simpler tenant-only setups.

## The Problem: Tenant-Only Isn't Enough

Multi-tenancy gives us organization-level isolation, but many applications need finer control. Consider a software company using your platform:

- **Tenant**: Acme Corp (the organization)
- **Teams**: Frontend, Backend, DevOps, QA
- **Projects**: Website Redesign, Mobile App, Infrastructure Migration

With only tenant-level permissions, you face an uncomfortable choice:

1. **Everyone sees everything**: All employees can access all projects, all documents, all data. This violates the principle of least privilege and creates compliance headaches.

2. **Separate tenants per team**: Each team gets its own tenant, but now cross-team collaboration becomes awkward. Shared resources require duplication or complex inter-tenant communication.

3. **Custom permission logic**: You implement ad-hoc permission checks throughout your codebase, leading to inconsistent enforcement and security gaps.

Workspaces solve this elegantly by creating isolated contexts within a tenant. A user can belong to multiple workspaces with different roles in each, while tenant administrators retain oversight of all workspaces.

## Workspace Hierarchy

The authorization hierarchy forms a tree structure with clear boundaries at each level:

```
System
└── Tenant (Acme Corp)
    ├── Tenant-Level Groups (Admin, Member)
    ├── Identities (Alice, Bob, Carol)
    │
    ├── Workspace: Frontend Project
    │   ├── Groups (ProjectAdmin, Developer, Viewer)
    │   ├── Members (Alice, Bob)
    │   └── Aggregates (tasks, documents)
    │
    ├── Workspace: Backend Project
    │   ├── Groups (ProjectAdmin, Developer)
    │   ├── Members (Bob, Carol)
    │   └── Aggregates (tasks, documents)
    │
    └── Workspace: Infrastructure
        ├── Groups (Admin, Operator)
        ├── Members (Carol)
        └── Aggregates (servers, deployments)
```

This hierarchy enables several important patterns:

- **Alice** can only access the Frontend Project
- **Bob** works on both Frontend and Backend, potentially with different roles in each
- **Carol** manages Backend and Infrastructure
- **Tenant Admins** can access all workspaces without explicit membership

Each workspace maintains its own groups with their own permissions. A "Developer" in the Frontend workspace might have different capabilities than a "Developer" in Infrastructure.

## Permission Levels: A Three-Tier Model

Before diving into workspace-specific authorization, it's important to understand the broader permission model. Our system recognizes three distinct permission levels, each building on the previous:

### Level 1: Anonymous

No authentication required. The user has no session, no account, no identity. This level is appropriate for truly public operations like viewing a landing page or accessing public API documentation.

```go
var AnonymousCmdFn = func(ctx context.Context, cmd Command) error {
    return nil  // Always allow
}
```

### Level 2: Authenticated

The user has logged in with valid credentials and has an active session, but hasn't selected a specific identity or tenant context. This level suits operations that require a logged-in user but don't need organizational context—like listing available tenants or updating account preferences.

```go
var AuthenticatedCmdFn = func(ctx context.Context, cmd Command) error {
    reqCtx := cmd.GetReqCtx()

    // Must have both account and session
    if len(reqCtx.SenderAccountUuid) > 0 && len(reqCtx.SenderSessionUuid) > 0 {
        return nil
    }
    return ErrPermissionDenied
}
```

### Level 3: Authorized

The user is logged in AND has selected an active identity within a specific tenant. This is the most common level for business operations—the user has full context including tenant, identity, groups, and permissions.

```go
var AuthorizedCmdFn = func(ctx context.Context, cmd Command) error {
    reqCtx := cmd.GetReqCtx()

    // Session-based access
    if len(reqCtx.SenderAccountUuid) > 0 && len(reqCtx.SenderSessionUuid) > 0 {
        return nil
    }
    // Identity-based access (includes service accounts)
    if len(reqCtx.SenderTenantUuid) > 0 && len(reqCtx.SenderIdentityUuid) > 0 {
        return nil
    }
    return ErrPermissionDenied
}
```

These three levels form the foundation. Workspace permissions layer on top of the Authorized level, providing additional granularity for users who have full tenant context.

## Workspace Data Model

The workspace aggregate captures all the information needed for fine-grained authorization:

```go
type Workspace struct {
    *comby.BaseAggregate

    TenantUuid        string              // Parent tenant
    Name              string              // Human-readable name
    Description       string              // Purpose of this workspace
    OwnerIdentityUuid string              // Who created/owns the workspace
    Groups            []*WorkspaceGroup   // Permission groups within workspace
    Members           []*WorkspaceMember  // Identities with access
    Attributes        *comby.Attributes   // Extensible metadata
}

type WorkspaceGroup struct {
    GroupUuid       string   `json:"groupUuid"`
    Name            string   `json:"name"`
    PermissionNames []string `json:"permissionNames"`
    CreatedAt       int64    `json:"createdAt"`
}

type WorkspaceMember struct {
    IdentityUuid string   `json:"identityUuid"`
    GroupUuids   []string `json:"groupUuids"`  // Groups within THIS workspace
    JoinedAt     int64    `json:"joinedAt"`
}
```

Several design decisions are worth noting:

**Workspace groups are independent of tenant groups.** An identity might be a "Member" at the tenant level but a "ProjectAdmin" within a specific workspace. The permissions don't conflict—they're additive.

**Members explicitly track group membership.** Rather than deriving permissions through complex joins, each member record directly lists which workspace groups they belong to. This makes permission checks fast and the data model easy to understand.

**The owner is tracked separately from membership.** Ownership represents who created or is responsible for the workspace, but it doesn't automatically grant permissions. This separation allows ownership transfer without changing access rights and maintains a clear audit trail.

## The Additive Permission Model

A fundamental design principle guides our authorization: **permissions are additive, never subtractive**. When checking if a user can perform an action:

```
Access Granted = (Tenant Permission) OR (Workspace Permission)
```

This means:

- **Tenant administrators can access ALL workspaces** in their tenant without explicit membership. Their tenant-level "admin" permission supersedes workspace boundaries.

- **Workspace members access only their workspaces** unless they also have tenant-level permissions. A "Developer" in the Frontend workspace can't see Backend project data.

- **Permissions are never subtracted.** You cannot create a workspace rule that removes a tenant-level permission. If someone has tenant admin rights, workspace restrictions don't apply to them.

This additive model has important implications:

1. **Simpler mental model**: Users never wonder "why can't I do X even though I have Y permission?" Permissions only ever add capabilities.

2. **Graceful degradation**: If workspace checks fail or are misconfigured, users with tenant permissions still function correctly.

3. **Clear audit trails**: When investigating access, you check tenant permissions first, then workspace permissions. The logic is straightforward.

## Authorization as Handler Middleware

The real power of this system comes from implementing authorization as handler middleware. Rather than scattering permission checks throughout command handlers, we wrap all handlers with authorization logic:

```go
func AuthCommandHandlerFunc(fc *comby.Facade) comby.CommandHandlerMiddlewareFunc {
    return func(ctx context.Context, fn DomainCommandHandlerFunc) DomainCommandHandlerFunc {
        return func(ctx context.Context, cmd Command, domainCmd DomainCmd) ([]Event, error) {
            reqCtx := cmd.GetReqCtx()
            allow := false

            // Step 1: Check system-level bypass
            if val := reqCtx.Attributes.Get(ExecuteSkipAuthorization); val == true {
                allow = true
            }

            // Step 2: Check custom permissions (domain-specific logic)
            if !allow {
                for _, perm := range runtime.RuntimePermissionList(ctx, fc) {
                    if cmd.GetDomain() == perm.PermissionDomain &&
                       cmd.GetDomainCmdName() == perm.PermissionType {
                        if perm.CmdFunc != nil {
                            if err := perm.CmdFunc(ctx, cmd); err == nil {
                                allow = true
                                break
                            }
                        }
                    }
                }
            }

            // Step 3: Check RBAC (tenant + workspace permissions)
            if !allow {
                allow = checkRBACPermissions(ctx, cmd, reqCtx)
            }

            if allow {
                return fn(ctx, cmd, domainCmd)
            }
            return nil, fmt.Errorf("%w - denied by authorization layer", ErrPermissionDenied)
        }
    }
}
```

The middleware is registered once during application startup:

```go
fc.AddCommandHandlerMiddleware(auth.AuthCommandHandlerFunc(fc))
fc.AddQueryHandlerMiddleware(auth.AuthQueryHandlerFunc(fc))
```

This approach provides several benefits:

**Centralized enforcement.** Every command and query passes through the same authorization logic. There's no way to accidentally skip a permission check.

**Separation of concerns.** Domain handlers focus on business logic. They don't need to know about permissions—that's handled at a higher level.

**Composable middleware.** Authorization is just one middleware in a chain. You can add logging, metrics, rate limiting, or other cross-cutting concerns without modifying business logic.

**Testable isolation.** You can test authorization logic independently from domain logic, and vice versa.

## The Complete Authorization Flow

Let's trace through the complete authorization flow when a command arrives:

```
1. SYSTEM BYPASS CHECK
   └─ Is ExecuteSkipAuthorization attribute set to true?
      └─ YES → ALLOW (used for system bootstrap, migrations)
      └─ NO  → Continue

2. CUSTOM PERMISSION CHECK
   └─ Is there a registered custom permission function for this command?
      └─ YES → Execute the custom function
         └─ Function returns nil → ALLOW
         └─ Function returns error → Continue to RBAC
      └─ NO → Continue to RBAC

3. SYSTEM ADMIN CHECK
   └─ Is sender a member of SYSTEM_TENANT_GROUP_ADMIN?
      └─ YES → ALLOW (god mode, bypasses all other checks)
      └─ NO  → Continue

4. CROSS-TENANT CHECK
   └─ Does sender's tenant match the target tenant?
      └─ NO  → DENY (cross-tenant access forbidden)
      └─ YES → Continue

5. TENANT-LEVEL PERMISSION CHECK
   └─ Does sender's identity belong to a tenant group with this permission?
      └─ YES → Check aggregate ownership
         └─ Is target aggregate owned by this tenant?
            └─ YES → ALLOW
            └─ NO  → DENY
      └─ NO → Continue to workspace check

6. WORKSPACE-LEVEL PERMISSION CHECK (if workspace context present)
   ├─ Is sender a member of the target workspace?
   │  └─ NO → DENY
   ├─ Does sender's workspace group have this permission?
   │  └─ NO → DENY
   └─ Is target aggregate owned by this workspace?
      └─ NO → DENY
      └─ YES → ALLOW

7. DEFAULT
   └─ DENY (if we reach here, no permission was granted)
```

This flow ensures defense in depth—multiple checks must pass, and the default is always to deny.

## Custom Permission Functions

While RBAC handles most cases, some commands need special treatment. Registration should be open to anonymous users. Password reset requires authentication but not authorization. Certain queries should only return data the user owns.

Custom permission functions handle these cases:

```go
// Anonymous users can register accounts
runtime.RegisterPermission(
    runtime.NewPermissionCmdFunc("Account", &AccountRegisterCommand{},
        "Allow anonymous account registration",
        runtime.AnonymousCmdFn),
)

// Authenticated users (no identity needed) can list their identities
runtime.RegisterPermission(
    runtime.NewPermissionQryFunc("Identity", &IdentityListByAccountQuery{},
        "List identities for current account",
        runtime.AuthenticatedQryFn),
)

// Custom logic: users can only query their own profile
runtime.RegisterPermission(
    runtime.NewPermissionQryFunc("Identity", &IdentityQueryModel{},
        "Query identity details",
        func(ctx context.Context, qry Query) error {
            reqCtx := qry.GetReqCtx()
            domainQry := qry.GetDomainQry().(*IdentityQueryModel)

            // Users can query their own identity
            if domainQry.IdentityUuid == reqCtx.SenderIdentityUuid {
                return nil
            }
            // Otherwise, fall through to RBAC
            return ErrPermissionDenied
        }),
)
```

Custom functions are checked before RBAC. If a custom function allows access, RBAC is skipped. If it denies access, RBAC still gets a chance. This lets you implement "allow if X, otherwise check normal permissions" patterns.

## Service Account Tokens

Not all API access comes from interactive users. Automated systems, CI/CD pipelines, and integrations need programmatic access. Service account tokens provide this:

```go
type TokenCtxModel struct {
    TokenUuid    string
    TokenValue   string  // bcrypt-hashed
    IdentityUuid string  // Associated identity
    TenantUuid   string  // Tenant context
    ExpiredAt    int64   // Optional expiration
}
```

Service accounts authenticate differently but authorize the same way:

```go
func AuthAuthorizationCtx(rm AuthCtxReadmodel) func(ctx huma.Context, next func(huma.Context)) {
    return func(ctx huma.Context, next func(huma.Context)) {
        authorization := ctx.Header("Authorization")

        // Try session-based auth first
        sessionUuid := SessionUuidFromBearerToken(authorization)
        sessionKey := SessionKeyFromBearerToken(authorization)
        if len(sessionUuid) > 0 && len(sessionKey) > 0 {
            // ... session validation logic
        }

        // Try service account token
        tokenUuid := ServiceAccountTokenUuidFromBearerToken(authorization)
        tokenKey := ServiceAccountTokenKeyFromBearerToken(authorization)
        if len(tokenUuid) > 0 && len(tokenKey) > 0 {
            ctxToken, _ := rm.GetServiceAccount(tokenUuid, tokenKey)

            if ctxToken != nil && !isExpired(ctxToken) {
                ctx = huma.WithValue(ctx, CTX_KEY_IDENTITY_UUID, ctxToken.IdentityUuid)
                ctx = huma.WithValue(ctx, CTX_KEY_TENANT_UUID, ctxToken.TenantUuid)
            }
        }

        next(ctx)
    }
}
```

The Authorization header supports multiple formats:

```
# User session with identity
Authorization: Bearer session=<uuid>|<key>, identity=<uuid>

# Service account token
Authorization: Bearer sa=<tokenUuid>|<tokenKey>
```

Once authenticated, service accounts flow through the same authorization middleware as interactive users. They have an associated identity with group memberships, so workspace permissions work identically.

## Skipping Authorization for System Operations

Some operations must bypass authorization entirely. Database migrations, system initialization, scheduled maintenance tasks—these need to execute regardless of user context.

The `ExecuteSkipAuthorization` attribute provides this escape hatch:

```go
const ExecuteSkipAuthorization = "ExecuteSkipAuthorization"

// In your system bootstrap code:
cmd, _ := comby.NewCommand("Tenant", &TenantCreateCommand{
    TenantUuid: "system-tenant-uuid",
    Name:       "System",
})
reqCtx := comby.NewRequestContext()
reqCtx.Attributes.Set(auth.ExecuteSkipAuthorization, true)
reqCtx.ExecuteWaitToFinish = true
cmd.SetReqCtx(reqCtx)

fc.DispatchCommand(ctx, cmd)
```

Use this sparingly and only in controlled contexts:

- Application startup and initialization
- Database migrations
- Scheduled system maintenance
- Event replay and recovery operations

Never expose this capability to end users or external APIs.

## Aggregate Ownership Tracking

Permissions alone aren't enough—we must also verify that aggregates belong to the right tenant and workspace. A user might have "order:read" permission, but they should only read orders from their own context.

The AuthReadmodel tracks these relationships automatically:

```go
type AccountCtxReadmodel struct {
    // ... other stores

    // Tuple stores for ownership tracking
    tenantAggregateTuples    *ReadmodelStore[bool]  // tenant:aggregate -> exists
    workspaceAggregateTuples *ReadmodelStore[bool]  // workspace:aggregate -> exists
}

// Event handlers automatically populate these
func (rm *AccountCtxReadmodel) OnHandleEventForTenantAggregateTuples(
    ctx context.Context, evt Event, domainEvt interface{},
) error {
    // Every event creates a tenant-aggregate tuple
    key := fmt.Sprintf("%s:%s", evt.TenantUuid, evt.AggregateUuid)
    rm.tenantAggregateTuples.Set(ctx, key, true)
    return nil
}
```

When authorizing access to a specific aggregate, we verify ownership:

```go
func (rm *AccountCtxReadmodel) IsValidTenantAggregate(tenantUuid, aggregateUuid string) bool {
    key := fmt.Sprintf("%s:%s", tenantUuid, aggregateUuid)
    if exists, err := rm.tenantAggregateTuples.Get(ctx, key); err == nil {
        return exists
    }
    return false
}

func (rm *AccountCtxReadmodel) IsValidWorkspaceAggregate(workspaceUuid, aggregateUuid string) bool {
    key := fmt.Sprintf("%s:%s", workspaceUuid, aggregateUuid)
    if exists, err := rm.workspaceAggregateTuples.Get(ctx, key); err == nil {
        return exists
    }
    return false
}
```

This prevents a subtle but critical attack: a user discovering aggregate UUIDs from another tenant/workspace and attempting to access them directly.

## Workspace Commands

Managing workspaces requires its own set of commands:

```go
// Workspace lifecycle
type WorkspaceCommandCreate struct {
    WorkspaceUuid     string
    TenantUuid        string
    Name              string
    Description       string
    OwnerIdentityUuid string
}

type WorkspaceCommandUpdate struct {
    WorkspaceUuid string
    Name          string
    Description   string
}

type WorkspaceCommandRemove struct {
    WorkspaceUuid string
}

// Member management
type WorkspaceCommandAddMember struct {
    WorkspaceUuid string
    IdentityUuid  string
    GroupUuids    []string  // Initial group assignments
}

type WorkspaceCommandRemoveMember struct {
    WorkspaceUuid string
    IdentityUuid  string
}

// Group management
type WorkspaceCommandAddGroup struct {
    WorkspaceUuid string
    GroupUuid     string
    Name          string
    Permissions   []string  // Permission names like "Order.OrderPlaceCommand"
}

type WorkspaceCommandUpdateGroup struct {
    WorkspaceUuid string
    GroupUuid     string
    Name          string
    Permissions   []string
}

type WorkspaceCommandRemoveGroup struct {
    WorkspaceUuid string
    GroupUuid     string
}
```

Each command produces corresponding events that update both the workspace aggregate and the AuthReadmodel used for permission checks.

## Workspace Events

The events capture the full history of workspace changes:

```go
type WorkspaceCreatedEvent struct {
    WorkspaceUuid     string
    TenantUuid        string
    Name              string
    Description       string
    OwnerIdentityUuid string
}

type WorkspaceMemberAddedEvent struct {
    WorkspaceUuid string
    IdentityUuid  string
    GroupUuids    []string
}

type WorkspaceMemberRemovedEvent struct {
    WorkspaceUuid string
    IdentityUuid  string
}

type WorkspaceGroupAddedEvent struct {
    WorkspaceUuid string
    GroupUuid     string
    Name          string
    Permissions   []string
}

type WorkspaceGroupUpdatedEvent struct {
    WorkspaceUuid string
    GroupUuid     string
    Name          string
    Permissions   []string
}

type WorkspaceGroupRemovedEvent struct {
    WorkspaceUuid string
    GroupUuid     string
}
```

These events serve multiple purposes:

1. **Rebuild aggregate state** during rehydration
2. **Update the AuthReadmodel** for real-time permission checks
3. **Provide audit trail** for security reviews
4. **Enable event replay** for data recovery or migration

## Workspace Context in Requests

The request context carries workspace information through the system:

```go
type RequestContext struct {
    // Sender information
    SenderTenantUuid    string
    SenderIdentityUuid  string
    SenderAccountUuid   string
    SenderSessionUuid   string

    // Target information
    TargetWorkspaceUuid  string  // The workspace being accessed
    TargetAggregateUuid  string  // Specific aggregate being accessed

    // Computed permissions (optional caching)
    Permissions []string

    // Extensible attributes
    Attributes *Attributes
}
```

The workspace context flows from the HTTP layer through commands to domain handlers:

```go
func WorkspaceContextMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        workspaceUuid := chi.URLParam(r, "workspaceUuid")

        reqCtx := GetReqCtxFromContext(r.Context())
        if reqCtx != nil && workspaceUuid != "" {
            reqCtx.TargetWorkspaceUuid = workspaceUuid
        }

        next.ServeHTTP(w, r)
    })
}
```

When creating workspace-scoped aggregates, the workspace is set from this context:

```go
func (ch *cmdHandler) PlaceOrder(ctx context.Context, cmd Command, domainCmd *PlaceOrderCommand) ([]Event, error) {
    reqCtx := GetReqCtxFromContext(ctx)

    order := NewOrder(domainCmd.OrderUuid)
    order.TenantUuid = reqCtx.SenderTenantUuid
    order.WorkspaceUuid = reqCtx.TargetWorkspaceUuid  // Inherited from context

    if err := order.Place(domainCmd.CustomerID, domainCmd.Items); err != nil {
        return nil, err
    }

    return order.GetUncommittedEvents(), nil
}
```

## Ownership vs Membership: An Important Distinction

A subtle but critical design choice: **ownership does not grant permissions**.

```go
// Creating a workspace sets the owner
workspace.OwnerIdentityUuid = "alice-uuid"

// But Alice has NO permissions in this workspace yet!
// She must be explicitly added as a member:
fc.DispatchCommand(ctx, &WorkspaceCommandAddMember{
    WorkspaceUuid: workspace.WorkspaceUuid,
    IdentityUuid:  "alice-uuid",
    GroupUuids:    []string{"workspace-admin-group-uuid"},
})
```

This separation enables several important patterns:

**Ownership transfer without permission changes.** A project can be handed off to a new owner without disrupting existing access rights.

**Delegated ownership.** An executive might "own" a workspace for organizational purposes while delegating all actual work to their team.

**Clear audit separation.** "Who created this?" and "Who can access this?" are tracked independently, making security reviews clearer.

**Consistent permission model.** Permissions always flow through group membership, never through special ownership rules. This makes the system easier to reason about.

## The AuthReadmodel: Central Permission Store

All authorization decisions flow through a single, optimized readmodel:

```go
type AccountCtxReadmodel struct {
    *comby.BaseReadmodel

    // Core entity stores
    tenants              *ReadmodelStore[*TenantCtxModel]
    accounts             *ReadmodelStore[*AccountCtxModel]
    identities           *ReadmodelStore[*IdentityCtxModel]
    groups               *ReadmodelStore[*GroupCtxModel]
    sessions             *ReadmodelStore[*SessionCtxModel]
    serviceAccountTokens *ReadmodelStore[*TokenCtxModel]
    workspaces           *ReadmodelStore[*WorkspaceCtxModel]

    // Relationship tracking
    tenantAggregateTuples    *ReadmodelStore[bool]
    workspaceIdentityTuples  *ReadmodelStore[bool]
    workspaceAggregateTuples *ReadmodelStore[bool]
}
```

This readmodel is populated by event handlers and provides fast lookup methods:

```go
// Identity and permission lookups
func (rm *AccountCtxReadmodel) GetIdentity(identityUuid string) (*IdentityCtxModel, error)
func (rm *AccountCtxReadmodel) GetGroup(groupUuid string) (*GroupCtxModel, error)

// Session validation
func (rm *AccountCtxReadmodel) GetSession(sessionUuid, sessionKey string) (*SessionCtxModel, error)

// Service account validation
func (rm *AccountCtxReadmodel) GetServiceAccount(tokenUuid, tokenKey string) (*TokenCtxModel, error)

// Workspace membership
func (rm *AccountCtxReadmodel) IsWorkspaceMemberInList(workspaceUuid, identityUuid string) bool
func (rm *AccountCtxReadmodel) GetWorkspacePermissions(workspaceUuid, identityUuid string) []*PermissionModel

// Ownership validation
func (rm *AccountCtxReadmodel) IsValidTenantAggregate(tenantUuid, aggregateUuid string) bool
func (rm *AccountCtxReadmodel) IsValidWorkspaceAggregate(workspaceUuid, aggregateUuid string) bool
```

The readmodel is registered globally and accessed through a singleton pattern:

```go
var authReadmodel *AccountCtxReadmodel

func SetAuthReadmodel(rm *AccountCtxReadmodel) {
    authReadmodel = rm
}

func GetAuthReadmodel() *AccountCtxReadmodel {
    return authReadmodel
}
```

This ensures all authorization checks use the same, consistent view of permissions.

## Querying Workspaces

Users need to discover which workspaces they can access:

```go
type WorkspaceQueryHandler struct {
    readmodel *WorkspaceReadmodel
}

// List all workspaces in a tenant (for admins)
func (qh *WorkspaceQueryHandler) ListByTenant(ctx context.Context, qry *WorkspaceListQuery) ([]*WorkspaceView, error) {
    reqCtx := GetReqCtxFromContext(ctx)

    // Tenant admins see all workspaces
    if qh.authorizer.hasTenantAdminPermission(reqCtx) {
        return qh.readmodel.ListByTenant(reqCtx.SenderTenantUuid)
    }

    // Regular users only see workspaces they're members of
    return qh.readmodel.ListByIdentity(reqCtx.SenderTenantUuid, reqCtx.SenderIdentityUuid)
}

// List workspaces for a specific identity
func (qh *WorkspaceQueryHandler) ListByIdentity(ctx context.Context, qry *WorkspaceListByIdentityQuery) ([]*WorkspaceView, error) {
    return qh.readmodel.ListByIdentity(qry.TenantUuid, qry.IdentityUuid)
}
```

The workspace readmodel maintains multiple indexes for efficient queries:

```go
type WorkspaceReadmodel struct {
    *comby.BaseReadmodel
    workspaces *ReadmodelStore[*WorkspaceModel]
}

func (rm *WorkspaceReadmodel) ListByTenant(tenantUuid string) []*WorkspaceView {
    var results []*WorkspaceView
    rm.workspaces.Range(func(key string, workspace *WorkspaceModel) bool {
        if workspace.TenantUuid == tenantUuid {
            results = append(results, workspace.ToView())
        }
        return true
    })
    return results
}

func (rm *WorkspaceReadmodel) ListByIdentity(tenantUuid, identityUuid string) []*WorkspaceView {
    var results []*WorkspaceView
    rm.workspaces.Range(func(key string, workspace *WorkspaceModel) bool {
        if workspace.TenantUuid == tenantUuid && workspace.HasMember(identityUuid) {
            results = append(results, workspace.ToView())
        }
        return true
    })
    return results
}
```

## API Endpoints

The REST API follows a nested resource pattern that reflects the authorization hierarchy:

```
# Workspace management
POST   /api/tenants/{tenantUuid}/workspaces
GET    /api/tenants/{tenantUuid}/workspaces
GET    /api/tenants/{tenantUuid}/workspaces/{workspaceUuid}
PATCH  /api/tenants/{tenantUuid}/workspaces/{workspaceUuid}
DELETE /api/tenants/{tenantUuid}/workspaces/{workspaceUuid}

# Member management
GET    /api/tenants/{tenantUuid}/workspaces/{workspaceUuid}/members
POST   /api/tenants/{tenantUuid}/workspaces/{workspaceUuid}/members
DELETE /api/tenants/{tenantUuid}/workspaces/{workspaceUuid}/members/{identityUuid}

# Group management
GET    /api/tenants/{tenantUuid}/workspaces/{workspaceUuid}/groups
POST   /api/tenants/{tenantUuid}/workspaces/{workspaceUuid}/groups
PATCH  /api/tenants/{tenantUuid}/workspaces/{workspaceUuid}/groups/{groupUuid}
DELETE /api/tenants/{tenantUuid}/workspaces/{workspaceUuid}/groups/{groupUuid}

# Workspace-scoped resources
GET    /api/tenants/{tenantUuid}/workspaces/{workspaceUuid}/orders
POST   /api/tenants/{tenantUuid}/workspaces/{workspaceUuid}/orders
GET    /api/tenants/{tenantUuid}/workspaces/{workspaceUuid}/orders/{orderUuid}
```

The URL structure makes authorization intuitive: the middleware extracts `tenantUuid` and `workspaceUuid` from the path, validates the user has appropriate access, then passes the request to handlers.

## HTTP Middleware Chain

The complete middleware chain for a workspace-scoped endpoint:

```go
router.Route("/api/tenants/{tenantUuid}/workspaces/{workspaceUuid}/orders", func(r chi.Router) {
    // 1. Extract and validate tenant context
    r.Use(TenantContextMiddleware)

    // 2. Extract workspace context
    r.Use(WorkspaceContextMiddleware)

    // 3. Check specific permissions for each operation
    r.With(WorkspaceAuthMiddleware("order:create")).Post("/", createOrder)
    r.With(WorkspaceAuthMiddleware("order:read")).Get("/", listOrders)
    r.With(WorkspaceAuthMiddleware("order:read")).Get("/{orderUuid}", getOrder)
    r.With(WorkspaceAuthMiddleware("order:update")).Patch("/{orderUuid}", updateOrder)
    r.With(WorkspaceAuthMiddleware("order:delete")).Delete("/{orderUuid}", deleteOrder)
})
```

Each middleware layer adds context and validates permissions:

```go
func WorkspaceAuthMiddleware(permission string) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            ctx := r.Context()

            // The authorization check uses the additive model:
            // tenant permission OR workspace permission
            if err := authorizer.CheckPermission(ctx, permission); err != nil {
                http.Error(w, "forbidden", http.StatusForbidden)
                return
            }

            next.ServeHTTP(w, r)
        })
    }
}
```

## Benefits of Workspaces

This workspace model provides substantial benefits for real-world applications:

### 1. Team Isolation
Teams see only their projects. The Frontend team can't accidentally (or intentionally) access Backend infrastructure. This reduces both mistakes and security risks.

### 2. Project Organization
Related resources stay together. All tasks, documents, and discussions for a project live in one workspace, making navigation intuitive and permissions simple.

### 3. Flexible Permissions
Different roles per workspace reflect organizational reality. A senior developer might be "Admin" on their main project but just "Viewer" on another team's work they're consulting on.

### 4. Audit Clarity
Security reviews become straightforward. You can answer "who accessed what, where?" by examining workspace membership and event logs. The hierarchical model makes reports comprehensible.

### 5. Backwards Compatibility
Existing applications that don't need workspaces continue working unchanged. Tenant-level permissions still function exactly as before. Workspaces are an opt-in enhancement, not a mandatory complication.

### 6. Programmatic Access
Service accounts integrate cleanly. A CI/CD pipeline can have a token scoped to a specific workspace, unable to affect other projects even if compromised.

## Testing the Authorization System

Comprehensive testing is crucial for security-critical code. The test suite covers all combinations:

```go
func TestAuthWorkspaceUltimateCommands(t *testing.T) {
    scenarios := []commandScenario{
        // Tenant-only (backwards compatibility)
        {
            Name:               "T000-Admin -> T000 (no workspace) -> CREATE -> OK",
            SenderTenantUuid:   test.TENANT_000_UUID,
            SenderIdentityUuid: test.TENANT_000_IDENTITY_ADMIN_UUID,
            TargetTenantUuid:   test.TENANT_000_UUID,
            ExpectError:        false,
            Description:        "Tenant admin with permission can create without workspace",
        },

        // Workspace permission without tenant permission
        {
            Name:                "T000-Pathetic -> WS000A -> CREATE -> OK",
            SenderTenantUuid:    test.TENANT_000_UUID,
            SenderIdentityUuid:  test.TENANT_000_IDENTITY_PATHETIC_UUID,
            TargetWorkspaceUuid: workspace000A,
            ExpectError:         false,
            Description:         "ADDITIVE: Workspace permission alone is sufficient",
        },

        // Cross-tenant denial
        {
            Name:               "T000-Admin -> T001 -> CREATE -> DENIED",
            SenderTenantUuid:   test.TENANT_000_UUID,
            SenderIdentityUuid: test.TENANT_000_IDENTITY_ADMIN_UUID,
            TargetTenantUuid:   test.TENANT_001_UUID,
            ExpectError:        true,
            Description:        "Cross-tenant requests are always denied",
        },

        // System admin override
        {
            Name:               "SYSTEM-Admin -> WS000A -> CREATE -> OK",
            SenderTenantUuid:   comby.SYSTEM_TENANT_UUID,
            SenderIdentityUuid: comby.SYSTEM_TENANT_IDENTITY_UUID,
            TargetTenantUuid:   test.TENANT_000_UUID,
            ExpectError:        false,
            Description:        "System admin bypasses all checks",
        },

        // ... many more scenarios
    }

    for _, scenario := range scenarios {
        t.Run(scenario.Name, func(t *testing.T) {
            // Execute command and verify expected outcome
        })
    }
}
```

Key scenarios to test:

- **Tenant-only operations** (no workspace context)
- **Workspace member access** (with and without specific permissions)
- **Owner without membership** (edge case: ownership ≠ access)
- **Cross-tenant denial** (fundamental security boundary)
- **Cross-workspace denial** (within same tenant)
- **System admin override** (god mode works everywhere)
- **Anonymous denial** (default deny for unauthenticated)
- **Aggregate ownership validation** (can't access other workspace's aggregates)

## Running an Example

Source: https://github.com/susdorf/building-event-sourced-systems-in-go/tree/main/13-workspaces-fine-grained-authorization/code

When you run the complete example, you'll see the workspace authorization system in action:

```
=== Workspace Fine-Grained Authorization Demo ===

1. Setting up authorization store and facade:
   - Created identities: Alice (Tenant A Admin), Bob (no tenant perms), Carol (Tenant B)
   - Created workspaces: Frontend Project, Backend Project
   - Bob is member of Frontend workspace with Developer permissions
   - Alice is member of both workspaces

=== TENANT-ONLY SCENARIOS ===

2. Alice (Tenant Admin) creates order WITHOUT workspace context:
   -> ALLOWED: Command executed successfully
   -> Created event: PlaceOrderCommandExecuted
   Authorization flow:
      → [CONTINUE] System Admin: Not a system admin
      → [CONTINUE] Cross-Tenant: Same tenant
      ✓ [ALLOW] Tenant Permission: Group Tenant A Admins has permission orders.PlaceOrderCommand

3. Bob (no tenant permissions) tries to create order WITHOUT workspace:
   -> DENIED: permission denied
   Authorization flow:
      → [CONTINUE] System Admin: Not a system admin
      → [CONTINUE] Cross-Tenant: Same tenant
      → [CONTINUE] Tenant Permission: No tenant-level permission found
      ✗ [DENY] Default: No permission granted

=== WORKSPACE SCENARIOS ===

4. Bob creates order IN Frontend workspace (workspace permission):
   -> ADDITIVE MODEL: Bob has no tenant permission but HAS workspace permission
   -> ALLOWED: Command executed successfully
   -> Created event: PlaceOrderCommandExecuted
   Authorization flow:
      → [CONTINUE] System Admin: Not a system admin
      → [CONTINUE] Cross-Tenant: Same tenant
      → [CONTINUE] Tenant Permission: No tenant-level permission found
      → [CONTINUE] Workspace Membership: Is workspace member
      ✓ [ALLOW] Workspace Permission: Has workspace permission orders.PlaceOrderCommand

5. Bob tries to create order IN Backend workspace (not a member):
   -> DENIED: not a workspace member
   Authorization flow:
      → [CONTINUE] System Admin: Not a system admin
      → [CONTINUE] Cross-Tenant: Same tenant
      → [CONTINUE] Tenant Permission: No tenant-level permission found
      ✗ [DENY] Workspace Membership: Identity bob is not a member of workspace workspace-backend

6. Alice creates order in Frontend workspace (tenant permission works in workspace):
   -> Alice has tenant-level permission, so she can access ANY workspace
   -> ALLOWED: Command executed successfully
   Authorization flow:
      → [CONTINUE] System Admin: Not a system admin
      → [CONTINUE] Cross-Tenant: Same tenant
      ✓ [ALLOW] Tenant Permission: Group Tenant A Admins has permission orders.PlaceOrderCommand

=== AGGREGATE OWNERSHIP SCENARIOS ===

8. Bob modifies order-1 (belongs to Frontend workspace):
   -> ALLOWED: Command executed successfully
   Authorization flow:
      → [CONTINUE] Workspace Membership: Is workspace member
      ✓ [ALLOW] Workspace Permission: Has workspace permission orders.PlaceOrderCommand
      → [CONTINUE] Workspace Aggregate: Aggregate belongs to workspace

9. Bob tries to access order-2 (belongs to Backend workspace) from Frontend:
   -> DENIED: aggregate does not belong to workspace
   Authorization flow:
      → [CONTINUE] Workspace Membership: Is workspace member
      ✓ [ALLOW] Workspace Permission: Has workspace permission orders.PlaceOrderCommand
      ✗ [DENY] Workspace Aggregate: Aggregate order-2 does not belong to workspace workspace-frontend

=== CROSS-TENANT SCENARIOS ===

10. Alice (Tenant A) tries to access Tenant B:
   -> DENIED: cross-tenant access denied
   Authorization flow:
      → [CONTINUE] System Admin: Not a system admin
      ✗ [DENY] Cross-Tenant: Sender tenant tenant-A != target tenant tenant-B

=== SYSTEM ADMIN SCENARIOS ===

11. System Admin accesses Tenant B workspace:
   -> ALLOWED: Command executed successfully
   Authorization flow:
      ✓ [ALLOW] System Admin: IsSystemAdmin flag is set

=== SKIP AUTHORIZATION SCENARIOS ===

12. System operation with SkipAuthorization flag:
   -> ALLOWED: Command executed successfully
   Authorization flow:
      ✓ [ALLOW] System Bypass: SkipAuthorization flag is set

=== AUTHORIZATION SUMMARY ===

The Additive Permission Model:
  Access = (Tenant Permission) OR (Workspace Permission)

Key Points Demonstrated:
  1. Tenant admins can access ALL workspaces without explicit membership
  2. Workspace permissions work for users WITHOUT tenant permissions
  3. Workspace membership is REQUIRED for workspace-level permissions
  4. Aggregate ownership is validated at both tenant and workspace level
  5. Cross-tenant access is ALWAYS denied (except system admin)
  6. System admin bypasses ALL authorization checks
  7. SkipAuthorization flag allows system operations to bypass checks
  8. Ownership != Membership: Owners need explicit group assignment for permissions

=== Demo Complete ===
```

## Summary

Workspaces extend multi-tenancy with fine-grained access control while maintaining simplicity:

1. **Additive model**: Tenant OR workspace permission grants access—never subtractive, easy to reason about.

2. **Handler middleware**: Authorization wraps all operations centrally, impossible to bypass accidentally.

3. **Three permission levels**: Anonymous → Authenticated → Authorized provides graduated access control.

4. **Membership required**: Being a workspace member is prerequisite for workspace permissions. Ownership alone grants nothing.

5. **Aggregate tracking**: Automatic ownership validation prevents cross-boundary data access.

6. **Service accounts**: Programmatic access integrates seamlessly with the same permission model.

7. **Custom functions**: Domain-specific authorization logic can override RBAC when needed.

8. **Backwards compatible**: Existing tenant-only applications continue working unchanged.

This model handles real organizational complexity—multiple teams, multiple projects, varying roles—while keeping the core authorization logic simple enough to audit and trust.

---

## What's Next

In the next post, we'll explore **Domain Registration** — how to organize and register domains, aggregates, handlers, and readmodels for a clean, modular architecture. We'll see how all the pieces we've built—commands, events, queries, authorization—come together in a coherent registration pattern.
