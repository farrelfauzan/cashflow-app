//go:build integration

package testenv

import (
	"context"

	"github.com/google/uuid"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/authz"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/listing"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/member"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/property"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/tenant"
)

// The All helpers read every page of a list with no filter, for tests about what a principal
// sees rather than about paging.

// AllTenants is every tenant p may see, by name.
func (e *Env) AllTenants(ctx context.Context, p authz.Principal) ([]tenant.Tenant, error) {
	return listing.All(func(pg listing.Page) (listing.Result[tenant.Tenant], error) {
		return e.Tenants.List(ctx, p, tenant.ListQuery{Page: pg})
	})
}

// AllMembers is every member of p's organization, by name.
func (e *Env) AllMembers(ctx context.Context, p authz.Principal) ([]member.Member, error) {
	return listing.All(func(pg listing.Page) (listing.Result[member.Member], error) {
		return e.Members.List(ctx, p, member.ListQuery{Page: pg})
	})
}

// AllProperties is every property p may see, by name.
func (e *Env) AllProperties(ctx context.Context, p authz.Principal) ([]property.Property, error) {
	return listing.All(func(pg listing.Page) (listing.Result[property.Property], error) {
		return e.Properties.List(ctx, p, property.ListQuery{Page: pg})
	})
}

// AllUnits is every unit of a property, by label.
func (e *Env) AllUnits(ctx context.Context, p authz.Principal, propertyID uuid.UUID) ([]property.Unit, error) {
	return listing.All(func(pg listing.Page) (listing.Result[property.Unit], error) {
		return e.Properties.ListUnits(ctx, p, propertyID, property.UnitQuery{Page: pg})
	})
}
