/*
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package carbon

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2/ec2iface"
	"github.com/aws/aws-sdk-go/service/pricing"
	"github.com/aws/aws-sdk-go/service/pricing/pricingiface"
	"github.com/samber/lo"

	"github.com/aws/karpenter-core/pkg/utils/pretty"
)

// Provider provides actual pricing data to the AWS cloud provider to allow it to make more informed decisions
// regarding which instances to launch.  This is initialized at startup with a periodically updated static price list to
// support running in locations where pricing data is unavailable.  In those cases the static pricing data provides a
// relative ordering that is still more accurate than our previous pricing model.  In the event that a pricing update
// fails, the previous pricing information is retained and used which may be the static initial pricing data if pricing
// updates never succeed.
type Provider struct {
	ec2     ec2iface.EC2API
	pricing pricingiface.PricingAPI
	region  string
	cm      *pretty.ChangeMonitor

	mu                 sync.RWMutex
	onDemandUpdateTime time.Time
	onDemandPrices     map[string]float64
	spotUpdateTime     time.Time
	spotPrices         map[string]zonal
}

// zonalPricing is used to capture the per-zone price
// for spot data as well as the default price
// based on on-demand price when the provisioningController first
// comes up
type zonal struct {
	defaultPrice float64 // Used until we get the spot pricing data
	prices       map[string]float64
}

type Err struct {
	error
	lastUpdateTime time.Time
}

func newZonalPricing(defaultPrice float64) zonal {
	z := zonal{
		prices: map[string]float64{},
	}
	z.defaultPrice = defaultPrice
	return z
}

// NewPricingAPI returns a pricing API configured based on a particular region
func NewAPI(sess *session.Session, region string) pricingiface.PricingAPI {
	if sess == nil {
		return nil
	}
	// pricing API doesn't have an endpoint in all regions
	pricingAPIRegion := "us-east-1"
	if strings.HasPrefix(region, "ap-") {
		pricingAPIRegion = "ap-south-1"
	} else if strings.HasPrefix(region, "cn-") {
		pricingAPIRegion = "cn-northwest-1"
	} else if strings.HasPrefix(region, "eu-") {
		pricingAPIRegion = "eu-central-1"
	}
	return pricing.New(sess, &aws.Config{Region: aws.String(pricingAPIRegion)})
}

func NewProvider(_ context.Context, pricing pricingiface.PricingAPI, ec2Api ec2iface.EC2API, region string) *Provider {
	p := &Provider{
		region:  region,
		ec2:     ec2Api,
		pricing: pricing,
		cm:      pretty.NewChangeMonitor(),
	}
	// sets the pricing data from the static default state for the provider
	p.Reset()

	return p
}

// InstanceTypes returns the list of all instance types for which either a spot or on-demand price is known.
func (p *Provider) InstanceTypes() []string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return lo.Union(lo.Keys(p.onDemandPrices), lo.Keys(p.spotPrices))
}

// OnDemandLastUpdated returns the time that the on-demand pricing was last updated
func (p *Provider) OnDemandLastUpdated() time.Time {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.onDemandUpdateTime
}

// SpotLastUpdated returns the time that the spot pricing was last updated
func (p *Provider) SpotLastUpdated() time.Time {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.spotUpdateTime
}

// TODO @JacobValdemar: should create something like this?
// OnDemandPrice returns the last known on-demand price for a given instance type, returning an error if there is no
// known on-demand pricing for the instance type.
func (p *Provider) OnDemandPrice(instanceType string) (float64, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	price, ok := p.onDemandPrices[instanceType]
	if !ok {
		return 0.0, false
	}
	return price, true
}

// SpotPrice returns the last known spot price for a given instance type and zone, returning an error
// if there is no known spot pricing for that instance type or zone
func (p *Provider) SpotPrice(instanceType string, zone string) (float64, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if val, ok := p.spotPrices[instanceType]; ok {
		if p.spotUpdateTime.Equal(initialPriceUpdate) {
			return val.defaultPrice, true
		}
		if price, ok := p.spotPrices[instanceType].prices[zone]; ok {
			return price, true
		}
		return 0.0, false
	}
	return 0.0, false
}

func (p *Provider) UpdateOnDemandPricing(ctx context.Context) error {
	return nil
}

func (p *Provider) UpdateSpotPricing(ctx context.Context) error {
	return nil
}

func (p *Provider) LivenessProbe(_ *http.Request) error {
	return nil
}

func populateInitialSpotPricing(pricing map[string]float64) map[string]zonal {
	m := map[string]zonal{}
	for it, price := range pricing {
		m[it] = newZonalPricing(price)
	}
	return m
}

func (p *Provider) Reset() {
	// TODO @JacobValdemar: Remove the commented out code below
	// see if we've got region specific pricing data
	// fmt.Printf("\nREGION: %s\n", p.region)
	// var staticPricing *map[string]float64 = nil
	// for region, pricing := range initialOnDemandPrices {
	// 	// e.g. eu-west-1a contains eu-west-1
	// 	if strings.Contains(p.region, region) {
	// 		staticPricing = pricing
	// 		break
	// 	}
	// }
	staticPricing, ok := initialOnDemandPrices[p.region]

	if !ok {
		// fall back to eu-west-1
		staticPricing = initialOnDemandPrices["eu-west-1"]
	}

	p.onDemandPrices = *staticPricing
	// default our spot pricing to the same as the on-demand pricing until a price update
	p.spotPrices = populateInitialSpotPricing(*staticPricing)
	p.onDemandUpdateTime = initialPriceUpdate
	p.spotUpdateTime = initialPriceUpdate
}
