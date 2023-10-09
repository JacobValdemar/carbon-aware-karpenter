package iprovider

import (
	"context"
	"net/http"
	"time"
)

type IPricingProvider interface {
	InstanceTypes() []string
	OnDemandLastUpdated() time.Time
	SpotLastUpdated() time.Time
	OnDemandPrice(string) (float64, bool)
	SpotPrice(string, string) (float64, bool)
	UpdateOnDemandPricing(context.Context) error
	UpdateSpotPricing(context.Context) error
	LivenessProbe(*http.Request) error
	Reset()
}
