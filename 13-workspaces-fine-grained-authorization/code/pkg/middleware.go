package pkg

import (
	"context"
	"fmt"
)

// AuthLog tracks authorization decisions for debugging/demo purposes
type AuthLog struct {
	Entries []AuthLogEntry
}

type AuthLogEntry struct {
	Step     string
	Decision string // "ALLOW", "DENY", "CONTINUE"
	Details  string
}

var globalAuthLog = &AuthLog{}

func GetAuthLog() *AuthLog {
	return globalAuthLog
}

func (l *AuthLog) Clear() {
	l.Entries = nil
}

func (l *AuthLog) Add(step, decision, details string) {
	l.Entries = append(l.Entries, AuthLogEntry{
		Step:     step,
		Decision: decision,
		Details:  details,
	})
}

// CommandMiddleware is a function that wraps command execution
type CommandMiddleware func(ctx context.Context, cmd *Command, next func(context.Context, *Command) *CommandResult) *CommandResult

// QueryMiddleware is a function that wraps query execution
type QueryMiddleware func(ctx context.Context, qry *Query, next func(context.Context, *Query) *QueryResult) *QueryResult

// AuthCommandMiddleware creates the authorization middleware for commands
func AuthCommandMiddleware(store *AuthStore) CommandMiddleware {
	return func(ctx context.Context, cmd *Command, next func(context.Context, *Command) *CommandResult) *CommandResult {
		globalAuthLog.Clear()

		reqCtx := GetRequestContext(ctx)
		if reqCtx == nil {
			globalAuthLog.Add("Context", "DENY", "No request context found")
			return &CommandResult{Success: false, Error: fmt.Errorf("no request context")}
		}

		allow := false

		// Step 1: Check skip authorization flag
		if !allow && (cmd.SkipAuthorization || reqCtx.SkipAuthorization) {
			globalAuthLog.Add("System Bypass", "ALLOW", "SkipAuthorization flag is set")
			allow = true
		}

		// Step 2: Check system admin
		if !allow {
			if reqCtx.IsSystemAdmin || reqCtx.SenderTenantUuid == SystemTenantUuid {
				identity := store.GetIdentity(reqCtx.SenderIdentityUuid)
				if identity != nil {
					for _, gid := range identity.GroupUuids {
						if gid == "system-admin-group" {
							globalAuthLog.Add("System Admin", "ALLOW", fmt.Sprintf("Identity %s is system admin", reqCtx.SenderIdentityUuid))
							allow = true
							break
						}
					}
				}
				if !allow && reqCtx.IsSystemAdmin {
					globalAuthLog.Add("System Admin", "ALLOW", "IsSystemAdmin flag is set")
					allow = true
				}
			}
			if !allow {
				globalAuthLog.Add("System Admin", "CONTINUE", "Not a system admin")
			}
		}

		// Step 3: Cross-tenant check
		if !allow {
			if reqCtx.SenderTenantUuid != cmd.TenantUuid {
				globalAuthLog.Add("Cross-Tenant", "DENY", fmt.Sprintf("Sender tenant %s != target tenant %s", reqCtx.SenderTenantUuid, cmd.TenantUuid))
				return &CommandResult{Success: false, Error: fmt.Errorf("cross-tenant access denied")}
			}
			globalAuthLog.Add("Cross-Tenant", "CONTINUE", "Same tenant")
		}

		// Step 4: Check tenant-level permissions
		if !allow {
			groups := store.GetIdentityTenantGroups(reqCtx.SenderIdentityUuid)
			for _, group := range groups {
				if group.HasPermission(cmd.Domain, cmd.CommandType) {
					globalAuthLog.Add("Tenant Permission", "ALLOW", fmt.Sprintf("Group %s has permission %s.%s", group.Name, cmd.Domain, cmd.CommandType))
					allow = true

					// Validate aggregate ownership if targeting specific aggregate
					if cmd.AggregateUuid != "" {
						if !store.IsValidTenantAggregate(cmd.TenantUuid, cmd.AggregateUuid) {
							globalAuthLog.Add("Aggregate Ownership", "DENY", fmt.Sprintf("Aggregate %s does not belong to tenant %s", cmd.AggregateUuid, cmd.TenantUuid))
							return &CommandResult{Success: false, Error: fmt.Errorf("aggregate does not belong to tenant")}
						}
						globalAuthLog.Add("Aggregate Ownership", "CONTINUE", "Aggregate belongs to tenant")
					}
					break
				}
			}
			if !allow {
				globalAuthLog.Add("Tenant Permission", "CONTINUE", "No tenant-level permission found")
			}
		}

		// Step 5: Check workspace-level permissions (if workspace context present)
		if !allow && cmd.WorkspaceUuid != "" {
			// Check if member of workspace
			if !store.IsWorkspaceMember(cmd.WorkspaceUuid, reqCtx.SenderIdentityUuid) {
				globalAuthLog.Add("Workspace Membership", "DENY", fmt.Sprintf("Identity %s is not a member of workspace %s", reqCtx.SenderIdentityUuid, cmd.WorkspaceUuid))
				return &CommandResult{Success: false, Error: fmt.Errorf("not a workspace member")}
			}
			globalAuthLog.Add("Workspace Membership", "CONTINUE", "Is workspace member")

			// Check workspace permissions
			permissions := store.GetWorkspacePermissions(cmd.WorkspaceUuid, reqCtx.SenderIdentityUuid)
			for _, perm := range permissions {
				if perm.Domain == cmd.Domain && perm.Type == cmd.CommandType {
					globalAuthLog.Add("Workspace Permission", "ALLOW", fmt.Sprintf("Has workspace permission %s.%s", cmd.Domain, cmd.CommandType))
					allow = true

					// Validate workspace aggregate ownership
					if cmd.AggregateUuid != "" {
						if !store.IsValidWorkspaceAggregate(cmd.WorkspaceUuid, cmd.AggregateUuid) {
							globalAuthLog.Add("Workspace Aggregate", "DENY", fmt.Sprintf("Aggregate %s does not belong to workspace %s", cmd.AggregateUuid, cmd.WorkspaceUuid))
							return &CommandResult{Success: false, Error: fmt.Errorf("aggregate does not belong to workspace")}
						}
						globalAuthLog.Add("Workspace Aggregate", "CONTINUE", "Aggregate belongs to workspace")
					}
					break
				}
			}
			if !allow {
				globalAuthLog.Add("Workspace Permission", "DENY", "No workspace permission found")
			}
		}

		// Step 6: Default deny
		if !allow {
			globalAuthLog.Add("Default", "DENY", "No permission granted")
			return &CommandResult{Success: false, Error: fmt.Errorf("permission denied")}
		}

		// Execute the command
		return next(ctx, cmd)
	}
}

// AuthQueryMiddleware creates the authorization middleware for queries
func AuthQueryMiddleware(store *AuthStore) QueryMiddleware {
	return func(ctx context.Context, qry *Query, next func(context.Context, *Query) *QueryResult) *QueryResult {
		globalAuthLog.Clear()

		reqCtx := GetRequestContext(ctx)
		if reqCtx == nil {
			globalAuthLog.Add("Context", "DENY", "No request context found")
			return &QueryResult{Success: false, Error: fmt.Errorf("no request context")}
		}

		allow := false

		// Step 1: Check system admin
		if reqCtx.IsSystemAdmin || reqCtx.SenderTenantUuid == SystemTenantUuid {
			globalAuthLog.Add("System Admin", "ALLOW", "System admin access")
			allow = true
		}

		// Step 2: Cross-tenant check
		if !allow {
			if reqCtx.SenderTenantUuid != qry.TenantUuid {
				globalAuthLog.Add("Cross-Tenant", "DENY", fmt.Sprintf("Sender tenant %s != target tenant %s", reqCtx.SenderTenantUuid, qry.TenantUuid))
				return &QueryResult{Success: false, Error: fmt.Errorf("cross-tenant access denied")}
			}
			globalAuthLog.Add("Cross-Tenant", "CONTINUE", "Same tenant")
		}

		// Step 3: Check tenant-level permissions
		if !allow {
			groups := store.GetIdentityTenantGroups(reqCtx.SenderIdentityUuid)
			for _, group := range groups {
				if group.HasPermission(qry.Domain, qry.QueryType) {
					globalAuthLog.Add("Tenant Permission", "ALLOW", fmt.Sprintf("Group %s has permission %s.%s", group.Name, qry.Domain, qry.QueryType))
					allow = true
					break
				}
			}
			if !allow {
				globalAuthLog.Add("Tenant Permission", "CONTINUE", "No tenant-level permission found")
			}
		}

		// Step 4: Check workspace-level permissions
		if !allow && qry.WorkspaceUuid != "" {
			if store.IsWorkspaceMember(qry.WorkspaceUuid, reqCtx.SenderIdentityUuid) {
				permissions := store.GetWorkspacePermissions(qry.WorkspaceUuid, reqCtx.SenderIdentityUuid)
				for _, perm := range permissions {
					if perm.Domain == qry.Domain && perm.Type == qry.QueryType {
						globalAuthLog.Add("Workspace Permission", "ALLOW", fmt.Sprintf("Has workspace permission %s.%s", qry.Domain, qry.QueryType))
						allow = true
						break
					}
				}
			}
			if !allow {
				globalAuthLog.Add("Workspace Permission", "DENY", "No workspace permission found")
			}
		}

		// Step 5: Default deny
		if !allow {
			globalAuthLog.Add("Default", "DENY", "No permission granted")
			return &QueryResult{Success: false, Error: fmt.Errorf("permission denied")}
		}

		return next(ctx, qry)
	}
}
