package pkg

import "time"

// Permission levels
const (
	PermissionLevelAnonymous     = "anonymous"
	PermissionLevelAuthenticated = "authenticated"
	PermissionLevelAuthorized    = "authorized"
)

// System constants
const (
	SystemTenantUuid    = "system-tenant"
	SystemIdentityUuid  = "system-identity"
	AnonymousTenantUuid = "anonymous"
	AnonymousIdentity   = "anonymous"
)

// Permission represents a permission that can be granted to groups
type Permission struct {
	Domain string // e.g., "orders"
	Type   string // e.g., "PlaceOrderCommand"
}

func (p Permission) String() string {
	return p.Domain + "." + p.Type
}

// Group represents a group with permissions (can be tenant-level or workspace-level)
type Group struct {
	GroupUuid   string
	Name        string
	Permissions []Permission
}

func (g *Group) HasPermission(domain, permType string) bool {
	for _, p := range g.Permissions {
		if p.Domain == domain && p.Type == permType {
			return true
		}
	}
	return false
}

// Identity represents a user identity within a tenant
type Identity struct {
	IdentityUuid string
	TenantUuid   string
	AccountUuid  string
	Name         string
	GroupUuids   []string // Tenant-level groups
}

// Workspace represents an isolated context within a tenant
type Workspace struct {
	WorkspaceUuid     string
	TenantUuid        string
	Name              string
	OwnerIdentityUuid string
	Groups            []*WorkspaceGroup
	Members           []*WorkspaceMember
}

type WorkspaceGroup struct {
	GroupUuid   string
	Name        string
	Permissions []Permission
}

type WorkspaceMember struct {
	IdentityUuid string
	GroupUuids   []string // Groups within this workspace
	JoinedAt     time.Time
}

func (w *Workspace) IsMember(identityUuid string) bool {
	for _, m := range w.Members {
		if m.IdentityUuid == identityUuid {
			return true
		}
	}
	return false
}

func (w *Workspace) GetMemberGroups(identityUuid string) []string {
	for _, m := range w.Members {
		if m.IdentityUuid == identityUuid {
			return m.GroupUuids
		}
	}
	return nil
}

func (w *Workspace) GetGroup(groupUuid string) *WorkspaceGroup {
	for _, g := range w.Groups {
		if g.GroupUuid == groupUuid {
			return g
		}
	}
	return nil
}

// ServiceAccountToken represents a programmatic access token
type ServiceAccountToken struct {
	TokenUuid    string
	TokenValue   string // In reality, this would be hashed
	IdentityUuid string
	TenantUuid   string
	ExpiredAt    *time.Time
}

func (t *ServiceAccountToken) IsExpired() bool {
	if t.ExpiredAt == nil {
		return false
	}
	return time.Now().After(*t.ExpiredAt)
}

// Event represents a domain event
type Event struct {
	EventUuid     string
	TenantUuid    string
	WorkspaceUuid string
	AggregateUuid string
	Domain        string
	EventType     string
	Timestamp     time.Time
	Data          map[string]interface{}
}

// Command represents a domain command
type Command struct {
	CommandUuid       string
	TenantUuid        string
	WorkspaceUuid     string
	AggregateUuid     string
	Domain            string
	CommandType       string
	SkipAuthorization bool
	Data              map[string]interface{}
}

// Query represents a domain query
type Query struct {
	TenantUuid    string
	WorkspaceUuid string
	AggregateUuid string
	Domain        string
	QueryType     string
}

// CommandResult holds the result of command execution
type CommandResult struct {
	Success bool
	Events  []*Event
	Error   error
}

// QueryResult holds the result of query execution
type QueryResult struct {
	Success bool
	Data    interface{}
	Error   error
}
