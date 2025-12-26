package pkg

import (
	"time"
)

// AuthStore holds all authorization-related data
type AuthStore struct {
	// Entity stores
	Identities   map[string]*Identity
	Groups       map[string]*Group
	Workspaces   map[string]*Workspace
	Tokens       map[string]*ServiceAccountToken
	Events       []*Event

	// Tuple stores for ownership tracking
	TenantAggregateTuples    map[string]bool // "tenant:aggregate" -> exists
	WorkspaceAggregateTuples map[string]bool // "workspace:aggregate" -> exists
}

// NewAuthStore creates a new auth store
func NewAuthStore() *AuthStore {
	return &AuthStore{
		Identities:               make(map[string]*Identity),
		Groups:                   make(map[string]*Group),
		Workspaces:               make(map[string]*Workspace),
		Tokens:                   make(map[string]*ServiceAccountToken),
		Events:                   make([]*Event, 0),
		TenantAggregateTuples:    make(map[string]bool),
		WorkspaceAggregateTuples: make(map[string]bool),
	}
}

// GetIdentity returns an identity by UUID
func (s *AuthStore) GetIdentity(identityUuid string) *Identity {
	return s.Identities[identityUuid]
}

// GetGroup returns a group by UUID
func (s *AuthStore) GetGroup(groupUuid string) *Group {
	return s.Groups[groupUuid]
}

// GetWorkspace returns a workspace by UUID
func (s *AuthStore) GetWorkspace(workspaceUuid string) *Workspace {
	return s.Workspaces[workspaceUuid]
}

// GetServiceAccountToken validates and returns a token
func (s *AuthStore) GetServiceAccountToken(tokenUuid, tokenValue string) *ServiceAccountToken {
	token := s.Tokens[tokenUuid]
	if token != nil && token.TokenValue == tokenValue && !token.IsExpired() {
		return token
	}
	return nil
}

// IsValidTenantAggregate checks if an aggregate belongs to a tenant
func (s *AuthStore) IsValidTenantAggregate(tenantUuid, aggregateUuid string) bool {
	key := tenantUuid + ":" + aggregateUuid
	return s.TenantAggregateTuples[key]
}

// IsValidWorkspaceAggregate checks if an aggregate belongs to a workspace
func (s *AuthStore) IsValidWorkspaceAggregate(workspaceUuid, aggregateUuid string) bool {
	key := workspaceUuid + ":" + aggregateUuid
	return s.WorkspaceAggregateTuples[key]
}

// AddTenantAggregate registers an aggregate as belonging to a tenant
func (s *AuthStore) AddTenantAggregate(tenantUuid, aggregateUuid string) {
	key := tenantUuid + ":" + aggregateUuid
	s.TenantAggregateTuples[key] = true
}

// AddWorkspaceAggregate registers an aggregate as belonging to a workspace
func (s *AuthStore) AddWorkspaceAggregate(workspaceUuid, aggregateUuid string) {
	key := workspaceUuid + ":" + aggregateUuid
	s.WorkspaceAggregateTuples[key] = true
}

// GetIdentityTenantGroups returns all tenant-level groups for an identity
func (s *AuthStore) GetIdentityTenantGroups(identityUuid string) []*Group {
	identity := s.GetIdentity(identityUuid)
	if identity == nil {
		return nil
	}

	groups := make([]*Group, 0)
	for _, groupUuid := range identity.GroupUuids {
		if group := s.GetGroup(groupUuid); group != nil {
			groups = append(groups, group)
		}
	}
	return groups
}

// GetWorkspacePermissions returns all permissions for an identity in a workspace
func (s *AuthStore) GetWorkspacePermissions(workspaceUuid, identityUuid string) []Permission {
	workspace := s.GetWorkspace(workspaceUuid)
	if workspace == nil {
		return nil
	}

	memberGroupUuids := workspace.GetMemberGroups(identityUuid)
	if memberGroupUuids == nil {
		return nil
	}

	permissions := make([]Permission, 0)
	for _, groupUuid := range memberGroupUuids {
		if group := workspace.GetGroup(groupUuid); group != nil {
			permissions = append(permissions, group.Permissions...)
		}
	}
	return permissions
}

// IsWorkspaceMember checks if an identity is a member of a workspace
func (s *AuthStore) IsWorkspaceMember(workspaceUuid, identityUuid string) bool {
	workspace := s.GetWorkspace(workspaceUuid)
	if workspace == nil {
		return false
	}
	return workspace.IsMember(identityUuid)
}

// SaveEvent stores an event
func (s *AuthStore) SaveEvent(event *Event) {
	s.Events = append(s.Events, event)

	// Track tenant-aggregate tuple
	if event.TenantUuid != "" && event.AggregateUuid != "" {
		s.AddTenantAggregate(event.TenantUuid, event.AggregateUuid)
	}

	// Track workspace-aggregate tuple
	if event.WorkspaceUuid != "" && event.AggregateUuid != "" {
		s.AddWorkspaceAggregate(event.WorkspaceUuid, event.AggregateUuid)
	}
}

// LoadEventsForAggregate loads events for a specific aggregate
func (s *AuthStore) LoadEventsForAggregate(tenantUuid, aggregateUuid string) []*Event {
	events := make([]*Event, 0)
	for _, e := range s.Events {
		if e.TenantUuid == tenantUuid && e.AggregateUuid == aggregateUuid {
			events = append(events, e)
		}
	}
	return events
}

// PopulateSampleData creates sample data for testing
func (s *AuthStore) PopulateSampleData() {
	// Create tenant-level groups
	s.Groups["tenant-a-admin"] = &Group{
		GroupUuid: "tenant-a-admin",
		Name:      "Tenant A Admins",
		Permissions: []Permission{
			{Domain: "orders", Type: "PlaceOrderCommand"},
			{Domain: "orders", Type: "ShipOrderCommand"},
			{Domain: "orders", Type: "CancelOrderCommand"},
			{Domain: "orders", Type: "GetOrderQuery"},
			{Domain: "orders", Type: "ListOrdersQuery"},
		},
	}

	s.Groups["tenant-a-member"] = &Group{
		GroupUuid: "tenant-a-member",
		Name:      "Tenant A Members",
		Permissions: []Permission{
			{Domain: "orders", Type: "GetOrderQuery"},
		},
	}

	s.Groups["tenant-b-admin"] = &Group{
		GroupUuid: "tenant-b-admin",
		Name:      "Tenant B Admins",
		Permissions: []Permission{
			{Domain: "orders", Type: "PlaceOrderCommand"},
			{Domain: "orders", Type: "GetOrderQuery"},
		},
	}

	// Create identities
	s.Identities["alice"] = &Identity{
		IdentityUuid: "alice",
		TenantUuid:   "tenant-A",
		AccountUuid:  "account-alice",
		Name:         "Alice (Tenant A Admin)",
		GroupUuids:   []string{"tenant-a-admin"},
	}

	s.Identities["bob"] = &Identity{
		IdentityUuid: "bob",
		TenantUuid:   "tenant-A",
		AccountUuid:  "account-bob",
		Name:         "Bob (Tenant A Member - no tenant permissions)",
		GroupUuids:   []string{}, // No tenant-level permissions
	}

	s.Identities["carol"] = &Identity{
		IdentityUuid: "carol",
		TenantUuid:   "tenant-B",
		AccountUuid:  "account-carol",
		Name:         "Carol (Tenant B Admin)",
		GroupUuids:   []string{"tenant-b-admin"},
	}

	s.Identities["system-admin"] = &Identity{
		IdentityUuid: "system-admin",
		TenantUuid:   SystemTenantUuid,
		AccountUuid:  "system-account",
		Name:         "System Administrator",
		GroupUuids:   []string{"system-admin-group"},
	}

	s.Groups["system-admin-group"] = &Group{
		GroupUuid:   "system-admin-group",
		Name:        "System Administrators",
		Permissions: []Permission{}, // System admins bypass permission checks
	}

	// Create workspaces
	s.Workspaces["workspace-frontend"] = &Workspace{
		WorkspaceUuid:     "workspace-frontend",
		TenantUuid:        "tenant-A",
		Name:              "Frontend Project",
		OwnerIdentityUuid: "alice",
		Groups: []*WorkspaceGroup{
			{
				GroupUuid: "ws-frontend-dev",
				Name:      "Frontend Developers",
				Permissions: []Permission{
					{Domain: "orders", Type: "PlaceOrderCommand"},
					{Domain: "orders", Type: "GetOrderQuery"},
				},
			},
			{
				GroupUuid: "ws-frontend-viewer",
				Name:      "Frontend Viewers",
				Permissions: []Permission{
					{Domain: "orders", Type: "GetOrderQuery"},
				},
			},
		},
		Members: []*WorkspaceMember{
			{IdentityUuid: "alice", GroupUuids: []string{}, JoinedAt: time.Now()},       // Member but no workspace groups (uses tenant perms)
			{IdentityUuid: "bob", GroupUuids: []string{"ws-frontend-dev"}, JoinedAt: time.Now()}, // Has workspace permission
		},
	}

	s.Workspaces["workspace-backend"] = &Workspace{
		WorkspaceUuid:     "workspace-backend",
		TenantUuid:        "tenant-A",
		Name:              "Backend Project",
		OwnerIdentityUuid: "alice",
		Groups: []*WorkspaceGroup{
			{
				GroupUuid: "ws-backend-dev",
				Name:      "Backend Developers",
				Permissions: []Permission{
					{Domain: "orders", Type: "PlaceOrderCommand"},
					{Domain: "orders", Type: "ShipOrderCommand"},
				},
			},
		},
		Members: []*WorkspaceMember{
			{IdentityUuid: "alice", GroupUuids: []string{"ws-backend-dev"}, JoinedAt: time.Now()},
		},
	}

	// Create service account token
	s.Tokens["sa-token-1"] = &ServiceAccountToken{
		TokenUuid:    "sa-token-1",
		TokenValue:   "secret-token-value",
		IdentityUuid: "bob",
		TenantUuid:   "tenant-A",
		ExpiredAt:    nil, // Never expires
	}

	// Create some existing aggregates
	s.AddTenantAggregate("tenant-A", "order-1")
	s.AddTenantAggregate("tenant-A", "order-2")
	s.AddTenantAggregate("tenant-B", "order-3")
	s.AddWorkspaceAggregate("workspace-frontend", "order-1")
	s.AddWorkspaceAggregate("workspace-backend", "order-2")
}
