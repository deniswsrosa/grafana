package ossaccesscontrol

import (
	"context"
	"errors"

	"github.com/grafana/grafana/pkg/infra/log"
	"github.com/grafana/grafana/pkg/infra/metrics"
	"github.com/grafana/grafana/pkg/infra/usagestats"
	"github.com/grafana/grafana/pkg/models"
	"github.com/grafana/grafana/pkg/services/accesscontrol"
	"github.com/grafana/grafana/pkg/services/accesscontrol/database"
	"github.com/grafana/grafana/pkg/services/sqlstore"
	"github.com/grafana/grafana/pkg/setting"
	"github.com/prometheus/client_golang/prometheus"
)

var _ accesscontrol.AccessControl = &OSSAccessControlService{}

func ProvideService(cfg *setting.Cfg, usageStats usagestats.Service, sql *sqlstore.SQLStore) *OSSAccessControlService {
	s := &OSSAccessControlService{
		Cfg:           cfg,
		UsageStats:    usageStats,
		Log:           log.New("accesscontrol"),
		scopeResolver: accesscontrol.NewScopeResolver(),
		store:         database.ProvideService(sql),
	}
	s.registerUsageMetrics()
	return s
}

// OSSAccessControlService is the service implementing role based access control.
type OSSAccessControlService struct {
	Cfg           *setting.Cfg
	UsageStats    usagestats.Service
	Log           log.Logger
	registrations accesscontrol.RegistrationList
	scopeResolver accesscontrol.ScopeResolver
	store         accesscontrol.ResourceStore
}

func (ac *OSSAccessControlService) IsDisabled() bool {
	if ac.Cfg == nil {
		return true
	}

	_, exists := ac.Cfg.FeatureToggles["accesscontrol"]
	return !exists
}

func (ac *OSSAccessControlService) registerUsageMetrics() {
	ac.UsageStats.RegisterMetricsFunc(func(context.Context) (map[string]interface{}, error) {
		return map[string]interface{}{
			"stats.oss.accesscontrol.enabled.count": ac.getUsageMetrics(),
		}, nil
	})
}

func (ac *OSSAccessControlService) getUsageMetrics() interface{} {
	if ac.IsDisabled() {
		return 0
	}

	return 1
}

// Evaluate evaluates access to the given resources
func (ac *OSSAccessControlService) Evaluate(ctx context.Context, user *models.SignedInUser, evaluator accesscontrol.Evaluator) (bool, error) {
	timer := prometheus.NewTimer(metrics.MAccessEvaluationsSummary)
	defer timer.ObserveDuration()
	metrics.MAccessEvaluationCount.Inc()

	permissions, err := ac.GetUserPermissions(ctx, user)
	if err != nil {
		return false, err
	}

	return evaluator.Evaluate(accesscontrol.GroupScopesByAction(permissions))
}

// GetUserRoles returns user permissions based on built-in roles
func (ac *OSSAccessControlService) GetUserRoles(ctx context.Context, user *models.SignedInUser) ([]*accesscontrol.RoleDTO, error) {
	return nil, errors.New("unsupported function") //OSS users will continue to use builtin roles via GetUserPermissions
}

// GetUserPermissions returns user permissions based on built-in roles
func (ac *OSSAccessControlService) GetUserPermissions(ctx context.Context, user *models.SignedInUser) ([]*accesscontrol.Permission, error) {
	timer := prometheus.NewTimer(metrics.MAccessPermissionsSummary)
	defer timer.ObserveDuration()

	permissions := ac.getFixedPermissions(ctx, ac.GetUserBuiltInRoles(user))
	resolved := make([]*accesscontrol.Permission, 0, len(permissions))
	for _, p := range permissions {
		// if the permission has a keyword in its scope it will be resolved
		permission, err := ac.scopeResolver.ResolveKeyword(user, *p)
		if err != nil {
			return nil, err
		}
		resolved = append(resolved, permission)
	}

	return resolved, nil
}

func (ac *OSSAccessControlService) getFixedPermissions(ctx context.Context, builtinRoles []string) []*accesscontrol.Permission {
	permissions := make([]*accesscontrol.Permission, 0)
	for _, builtin := range builtinRoles {
		if roleNames, ok := accesscontrol.FixedRoleGrants[builtin]; ok {
			for _, name := range roleNames {
				role, exists := accesscontrol.FixedRoles[name]
				if !exists {
					continue
				}
				for i := range role.Permissions {
					permissions = append(permissions, &role.Permissions[i])
				}
			}
		}
	}

	return permissions
}

func (ac *OSSAccessControlService) GetUserBuiltInRoles(user *models.SignedInUser) []string {
	roles := []string{string(user.OrgRole)}
	for _, role := range user.OrgRole.Children() {
		roles = append(roles, string(role))
	}
	if user.IsGrafanaAdmin {
		roles = append(roles, accesscontrol.RoleGrafanaAdmin)
	}

	return roles
}

func (ac *OSSAccessControlService) saveFixedRole(role accesscontrol.RoleDTO) {
	if storedRole, ok := accesscontrol.FixedRoles[role.Name]; ok {
		// If a package wants to override another package's role, the version
		// needs to be increased. Hence, we don't overwrite a role with a
		// greater version.
		if storedRole.Version >= role.Version {
			ac.Log.Debug("the has already been stored in a greater version, skipping registration", "role", role.Name)
			return
		}
	}
	// Save role
	accesscontrol.FixedRoles[role.Name] = role
}

func (ac *OSSAccessControlService) assignFixedRole(role accesscontrol.RoleDTO, builtInRoles []string) {
	for _, builtInRole := range builtInRoles {
		// Only record new assignments
		alreadyAssigned := false
		assignments, ok := accesscontrol.FixedRoleGrants[builtInRole]
		if ok {
			for _, assignedRole := range assignments {
				if assignedRole == role.Name {
					ac.Log.Debug("the role has already been assigned", "rolename", role.Name, "build_in_role", builtInRole)
					alreadyAssigned = true
				}
			}
		}
		if !alreadyAssigned {
			assignments = append(assignments, role.Name)
			accesscontrol.FixedRoleGrants[builtInRole] = assignments
		}
	}
}

// RegisterFixedRoles registers all declared roles in RAM
func (ac *OSSAccessControlService) RegisterFixedRoles() error {
	// If accesscontrol is disabled no need to register roles
	if ac.IsDisabled() {
		return nil
	}
	var err error
	ac.registrations.Range(func(registration accesscontrol.RoleRegistration) bool {
		ac.registerFixedRole(registration.Role, registration.Grants)
		return true
	})
	return err
}

// RegisterFixedRole saves a fixed role and assigns it to built-in roles
func (ac *OSSAccessControlService) registerFixedRole(role accesscontrol.RoleDTO, builtInRoles []string) {
	ac.saveFixedRole(role)
	ac.assignFixedRole(role, builtInRoles)
}

// DeclareFixedRoles allow the caller to declare, to the service, fixed roles and their assignments
// to organization roles ("Viewer", "Editor", "Admin") or "Grafana Admin"
func (ac *OSSAccessControlService) DeclareFixedRoles(registrations ...accesscontrol.RoleRegistration) error {
	// If accesscontrol is disabled no need to register roles
	if ac.IsDisabled() {
		return nil
	}

	for _, r := range registrations {
		err := accesscontrol.ValidateFixedRole(r.Role)
		if err != nil {
			return err
		}

		err = accesscontrol.ValidateBuiltInRoles(r.Grants)
		if err != nil {
			return err
		}

		ac.registrations.Append(r)
	}

	return nil
}

// GetResourcesMetadata returns a map of accesscontrol metadata, listing for each resource, users available actions
func (ac *OSSAccessControlService) GetResourcesMetadata(ctx context.Context, user *models.SignedInUser, resource string, resourceIDs []string) (map[string]accesscontrol.Metadata, error) {
	builtInRoles := ac.GetUserBuiltInRoles(user)

	fixedPermissions := ac.getFixedPermissions(ctx, builtInRoles)

	storedPermissions, err := ac.store.GetUserResourcePermissions(ctx, user.OrgId, user.UserId, accesscontrol.GetUserResourcesPermissionsQuery{
		BuiltInRoles: builtInRoles,
		Resource:     resource,
		ResourceIDs:  resourceIDs,
	})
	if err != nil {
		return nil, err
	}

	userPermissions := append(fixedPermissions, storedPermissions...)

	allScope := accesscontrol.GetResourceAllScope(resource)
	allIDScope := accesscontrol.GetResourceAllIDScope(resource)

	result := map[string]accesscontrol.Metadata{}
	for _, r := range resourceIDs {
		scope := accesscontrol.GetResourceScope(resource, r)
		for _, p := range userPermissions {
			if p.Scope == "*" || p.Scope == allScope || p.Scope == allIDScope || p.Scope == scope {
				metadata, initialized := result[r]
				if !initialized {
					metadata = accesscontrol.Metadata{}
				}
				metadata[p.Action] = true
				result[r] = metadata
			}
		}
	}

	return result, nil
}
