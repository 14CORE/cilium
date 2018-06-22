// Copyright 2018 Authors of Cilium
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package resolver

import (
	"fmt"
	"testing"

	"github.com/cilium/cilium/pkg/comparator"
	"github.com/cilium/cilium/pkg/envoy/cilium"
	"github.com/cilium/cilium/pkg/envoy/envoy/api/v2/core"
	"github.com/gogo/protobuf/sortkeys"
	"github.com/golang/protobuf/ptypes/wrappers"

	envoy_api_v2_route "github.com/cilium/cilium/pkg/envoy/envoy/api/v2/route"
	"github.com/cilium/cilium/pkg/identity"
	"github.com/cilium/cilium/pkg/labels"
	"github.com/cilium/cilium/pkg/policy/api"
	. "gopkg.in/check.v1"
)

// Hook up gocheck into the "go test" runner.
func Test(t *testing.T) {
	TestingT(t)
}

type ResolverTestSuite struct{}

var _ = Suite(&ResolverTestSuite{})

var (
	identity11             = identity.NumericIdentity(11)
	identity30             = identity.NumericIdentity(30)
	identity35             = identity.NumericIdentity(35)
	identitySplashBrothers = identity.NumericIdentity(41)
	identityWarriors       = identity.NumericIdentity(73)

	endpointSelectorDurant     = api.NewESFromLabels(labels.ParseSelectLabel(fmt.Sprintf("%s:%s", labels.LabelSourceK8s, "durant")))
	endpointSelectorSteph      = api.NewESFromLabels(labels.ParseSelectLabel(fmt.Sprintf("%s:%s", labels.LabelSourceK8s, "steph")))
	endpointSelectorKlay       = api.NewESFromLabels(labels.ParseSelectLabel(fmt.Sprintf("%s:%s", labels.LabelSourceK8s, "klay")))
	endpointSelectorSplashBros = api.NewESFromLabels(
		labels.ParseSelectLabel(fmt.Sprintf("%s:%s", labels.LabelSourceK8s, "steph")),
		labels.ParseSelectLabel(fmt.Sprintf("%s:%s", labels.LabelSourceK8s, "klay")))
	endpointSelectorA = api.NewESFromLabels(labels.ParseSelectLabel("id=a"))

	stephLabel  = labels.NewLabel("steph", "", labels.LabelSourceK8s)
	durantLabel = labels.NewLabel("durant", "", labels.LabelSourceK8s)
	klayLabel   = labels.NewLabel("klay", "", labels.LabelSourceK8s)

	identity11Labels = labels.LabelArray{
		klayLabel,
	}

	identity30Labels = labels.LabelArray{
		stephLabel,
	}

	identity35Labels = labels.LabelArray{
		durantLabel,
	}

	identityWarriorsLabels = labels.LabelArray{
		durantLabel,
		stephLabel,
		klayLabel,
	}

	identitySplashBrothersLabels = labels.LabelArray{
		stephLabel,
		klayLabel,
	}
)

func initIdentityCache() identity.IdentityCache {
	identityCache := identity.IdentityCache{}

	identityCache[identity11] = identity11Labels
	identityCache[identity30] = identity30Labels
	identityCache[identity35] = identity35Labels
	identityCache[identityWarriors] = identityWarriorsLabels
	identityCache[identitySplashBrothers] = identitySplashBrothersLabels

	return identityCache
}

func (ds *ResolverTestSuite) TestAllowedDeniedIdentitySets(c *C) {

	allowedIngressIdentities := identity.IdentityCache{}
	deniedIngressIdentities := identity.IdentityCache{}

	rules := api.Rules{&api.Rule{
		EndpointSelector: endpointSelectorDurant,
		Ingress: []api.IngressRule{
			{
				FromRequires: []api.EndpointSelector{
					api.NewESFromLabels(labels.ParseSelectLabel("k8s:klay")),
				},
			},
			{
				FromRequires: []api.EndpointSelector{
					api.NewESFromLabels(labels.ParseSelectLabel("k8s:steph")),
				},
			},
			{
				FromRequires: []api.EndpointSelector{
					api.NewESFromLabels(labels.ParseSelectLabel("k8s:durant")),
				},
			},
		},
	}}

	identityCache := identity.IdentityCache{}
	identityCache[identity11] = identity11Labels
	identityCache[identity30] = identity30Labels
	identityCache[identity35] = identity35Labels
	identityCache[identityWarriors] = identityWarriorsLabels

	for remoteIdentity, remoteIdentityLabels := range identityCache {
		//fmt.Printf("remoteIdentity=%d, remoteIdentityLabels=%s\n", remoteIdentity, remoteIdentityLabels)
		for _, rule := range rules {
			for _, ingressRule := range rule.Ingress {
				//fmt.Printf("\t testing rule: %v\n", ingressRule)
				for _, fromRequires := range ingressRule.FromRequires {
					computeAllowedAndDeniedIdentitySets(fromRequires, remoteIdentity, remoteIdentityLabels, allowedIngressIdentities, deniedIngressIdentities)
				}
			}
		}
	}

	expectedAllowedIngressIdentities := identity.IdentityCache{
		identityWarriors: identityWarriorsLabels,
	}

	expectedDeniedIngressIdentities := identity.IdentityCache{
		identity11: identity11Labels,
		identity30: identity30Labels,
		identity35: identity35Labels,
	}

	c.Assert(allowedIngressIdentities, comparator.DeepEquals, expectedAllowedIngressIdentities)
	c.Assert(deniedIngressIdentities, comparator.DeepEquals, expectedDeniedIngressIdentities)

}
func (ds *ResolverTestSuite) TestComputeRemotePolicies(c *C) {
	endpointSelectorDurant := api.NewESFromLabels(labels.ParseSelectLabel(fmt.Sprintf("%s:%s", labels.LabelSourceK8s, "durant")))
	uint64Identity35 := uint64(35)
	numericIdentity35 := identity.NumericIdentity(35)
	numericIdentity23 := identity.NumericIdentity(23)

	// Case 1: endpoint selector selects all at L3, and there are no denied
	// identities; can be allowed at L3. Allow-all is treated as an empty list
	// of remote policies.
	remotePolicies := computeRemotePolicies(api.WildcardEndpointSelector, numericIdentity35, identity.IdentityCache{})
	c.Assert(len(remotePolicies), Equals, 0)

	// Case 2: Despite wildcarding at L3, still need to specify identity
	// explicitly due to presence of denied identities.
	remotePolicies = computeRemotePolicies(api.WildcardEndpointSelector, numericIdentity35, identity.IdentityCache{numericIdentity23: labels.LabelArray{}})
	c.Assert(len(remotePolicies), Equals, 1)
	c.Assert(remotePolicies[0], Equals, uint64Identity35)

	// Case 3: no wildcarding at L3, and no denied identities; must specify that
	// only remote policy which is allowed is the one provided to the function.
	remotePolicies = computeRemotePolicies(endpointSelectorDurant, numericIdentity35, identity.IdentityCache{})
	c.Assert(len(remotePolicies), Equals, 1)
	c.Assert(remotePolicies[0], Equals, uint64Identity35)

	// Case 4: no wildcarding at L3, and denied identities; must specify that
	// only remote policy which is allowed is the one provided to the function.
	remotePolicies = computeRemotePolicies(endpointSelectorDurant, numericIdentity35, identity.IdentityCache{numericIdentity23: labels.LabelArray{}})
	c.Assert(len(remotePolicies), Equals, 1)
	c.Assert(remotePolicies[0], Equals, uint64Identity35)
}
func (ds *ResolverTestSuite) TestResolveIdentityPolicyL3Only(c *C) {
	identityCache := initIdentityCache()

	rules := api.Rules{
		&api.Rule{
			EndpointSelector: endpointSelectorSplashBros,
			Ingress: []api.IngressRule{
				{
					FromEndpoints: []api.EndpointSelector{
						api.NewESFromLabels(
							stephLabel,
							klayLabel,
						),
					},
				},
			},
		},
	}
	splashBrothersPolicy := ResolveIdentityPolicy(rules, identityCache, identitySplashBrothers)
	expectedPolicy := &cilium.NetworkPolicy{
		Policy: uint64(identitySplashBrothers),
		IngressPerPortPolicies: []*cilium.PortNetworkPolicy{
			{
				Rules: []*cilium.PortNetworkPolicyRule{
					{
						RemotePolicies: []uint64{uint64(identitySplashBrothers)},
					},
				},
				ProtocolWildcard: true,
			},
			{
				Rules: []*cilium.PortNetworkPolicyRule{
					{
						RemotePolicies: []uint64{uint64(identityWarriors)},
					},
				},
				ProtocolWildcard: true,
			},
		},
	}
	c.Assert(expectedPolicy, comparator.DeepEquals, splashBrothersPolicy)

	OptimizeNetworkPolicy(splashBrothersPolicy)
	expectedPolicy = &cilium.NetworkPolicy{
		Policy: uint64(identitySplashBrothers),
		IngressPerPortPolicies: []*cilium.PortNetworkPolicy{
			{
				Rules: []*cilium.PortNetworkPolicyRule{
					{
						RemotePolicies: []uint64{uint64(identitySplashBrothers), uint64(identityWarriors)},
					},
				},
				ProtocolWildcard: true,
			},
		},
	}
	c.Assert(splashBrothersPolicy, comparator.DeepEquals, expectedPolicy)

	rules = api.Rules{
		&api.Rule{
			EndpointSelector: endpointSelectorSplashBros,
			Ingress: []api.IngressRule{
				{
					FromEndpoints: []api.EndpointSelector{
						api.NewESFromLabels(
							stephLabel,
							klayLabel,
						),
					},
				},
				{
					FromEndpoints: []api.EndpointSelector{
						api.NewESFromLabels(
							durantLabel,
						),
					},
				},
			},
		},
	}
	splashBrothersPolicy = ResolveIdentityPolicy(rules, identityCache, identitySplashBrothers)
	expectedPolicy = &cilium.NetworkPolicy{
		Policy: uint64(identitySplashBrothers),
		IngressPerPortPolicies: []*cilium.PortNetworkPolicy{
			{
				Rules: []*cilium.PortNetworkPolicyRule{
					{
						RemotePolicies: []uint64{uint64(identity35)},
					},
				},
				ProtocolWildcard: true,
			},
			{
				Rules: []*cilium.PortNetworkPolicyRule{
					{
						RemotePolicies: []uint64{uint64(identitySplashBrothers)},
					},
				},
				ProtocolWildcard: true,
			},
			{
				Rules: []*cilium.PortNetworkPolicyRule{
					{
						RemotePolicies: []uint64{uint64(identityWarriors)},
					},
				},
				ProtocolWildcard: true,
			},
			{
				Rules: []*cilium.PortNetworkPolicyRule{
					{
						RemotePolicies: []uint64{uint64(identityWarriors)},
					},
				},
				ProtocolWildcard: true,
			},
		},
	}
	c.Assert(splashBrothersPolicy, comparator.DeepEquals, expectedPolicy)

	expectedPolicy = &cilium.NetworkPolicy{
		Policy: uint64(identitySplashBrothers),
		IngressPerPortPolicies: []*cilium.PortNetworkPolicy{
			{
				Rules: []*cilium.PortNetworkPolicyRule{
					{
						RemotePolicies: []uint64{uint64(identity35), uint64(identitySplashBrothers), uint64(identityWarriors)},
					},
				},
				ProtocolWildcard: true,
			},
		},
	}

	OptimizeNetworkPolicy(splashBrothersPolicy)
	c.Assert(splashBrothersPolicy, comparator.DeepEquals, expectedPolicy)
}
func (ds *ResolverTestSuite) TestResolveIdentityPolicyL4Only(c *C) {
	identityCache := initIdentityCache()

	rules := api.Rules{
		&api.Rule{
			EndpointSelector: endpointSelectorSplashBros,
			Ingress: []api.IngressRule{
				{
					ToPorts: []api.PortRule{{
						Ports: []api.PortProtocol{
							{Port: "80", Protocol: api.ProtoTCP},
						},
					}},
				},
			},
		},
	}
	splashBrothersPolicy := ResolveIdentityPolicy(rules, identityCache, identitySplashBrothers)
	expectedPolicy := &cilium.NetworkPolicy{
		Policy: uint64(identitySplashBrothers),
		IngressPerPortPolicies: []*cilium.PortNetworkPolicy{
			{
				Port:     80,
				Protocol: core.SocketAddress_TCP,
				Rules:    []*cilium.PortNetworkPolicyRule{},
			},
		},
	}
	c.Assert(splashBrothersPolicy, comparator.DeepEquals, expectedPolicy)

	err := OptimizeNetworkPolicy(splashBrothersPolicy)
	c.Assert(err, IsNil)
	expectedPolicy = &cilium.NetworkPolicy{
		Policy: uint64(identitySplashBrothers),
		IngressPerPortPolicies: []*cilium.PortNetworkPolicy{
			{
				Port:     80,
				Protocol: core.SocketAddress_TCP,
				Rules:    []*cilium.PortNetworkPolicyRule{},
			},
		},
	}
	c.Assert(splashBrothersPolicy, comparator.DeepEquals, expectedPolicy)

	rules = api.Rules{
		&api.Rule{
			EndpointSelector: endpointSelectorSplashBros,
			Ingress: []api.IngressRule{
				{
					ToPorts: []api.PortRule{{
						Ports: []api.PortProtocol{
							{Port: "80", Protocol: api.ProtoTCP},
							{Port: "53", Protocol: api.ProtoUDP},
							{Port: "8080", Protocol: api.ProtoAny},
						},
					}},
				},
			},
		},
	}
	splashBrothersPolicy = ResolveIdentityPolicy(rules, identityCache, identitySplashBrothers)
	expectedPolicy = &cilium.NetworkPolicy{
		Policy: uint64(identitySplashBrothers),
		IngressPerPortPolicies: []*cilium.PortNetworkPolicy{
			{
				Port:     80,
				Protocol: core.SocketAddress_TCP,
				Rules:    []*cilium.PortNetworkPolicyRule{},
			},
			{
				Port:     8080,
				Protocol: core.SocketAddress_TCP,
				Rules:    []*cilium.PortNetworkPolicyRule{},
			},
			{
				Port:     53,
				Protocol: core.SocketAddress_UDP,
				Rules:    []*cilium.PortNetworkPolicyRule{},
			},
			{
				Port:     8080,
				Protocol: core.SocketAddress_UDP,
				Rules:    []*cilium.PortNetworkPolicyRule{},
			},
		},
	}
	c.Assert(splashBrothersPolicy, comparator.DeepEquals, expectedPolicy)

	err = OptimizeNetworkPolicy(splashBrothersPolicy)
	c.Assert(err, IsNil)
	expectedPolicy = &cilium.NetworkPolicy{
		Policy: uint64(identitySplashBrothers),
		IngressPerPortPolicies: []*cilium.PortNetworkPolicy{
			{
				Port:     80,
				Protocol: core.SocketAddress_TCP,
				Rules:    []*cilium.PortNetworkPolicyRule{},
			},
			{
				Port:     8080,
				Protocol: core.SocketAddress_TCP,
				Rules:    []*cilium.PortNetworkPolicyRule{},
			},
			{
				Port:     53,
				Protocol: core.SocketAddress_UDP,
				Rules:    []*cilium.PortNetworkPolicyRule{},
			},
			{
				Port:     8080,
				Protocol: core.SocketAddress_UDP,
				Rules:    []*cilium.PortNetworkPolicyRule{},
			},
		},
	}
	c.Assert(splashBrothersPolicy, comparator.DeepEquals, expectedPolicy)

	// pkg/policy/l4_filter_test.go:TestMergeAllowAllL3AndAllowAllL7 Case 1A
	rules = api.Rules{
		&api.Rule{
			EndpointSelector: endpointSelectorSplashBros,
			Ingress: []api.IngressRule{
				{
					FromEndpoints: []api.EndpointSelector{api.WildcardEndpointSelector},
					ToPorts: []api.PortRule{{
						Ports: []api.PortProtocol{
							{Port: "80", Protocol: api.ProtoTCP},
						},
					}},
				},
				{
					FromEndpoints: []api.EndpointSelector{api.WildcardEndpointSelector},
					ToPorts: []api.PortRule{{
						Ports: []api.PortProtocol{
							{Port: "80", Protocol: api.ProtoTCP},
						},
					}},
				},
			},
		},
	}

	splashBrothersPolicy = ResolveIdentityPolicy(rules, identityCache, identitySplashBrothers)
	expectedPolicy = &cilium.NetworkPolicy{
		Policy: uint64(identitySplashBrothers),
		IngressPerPortPolicies: []*cilium.PortNetworkPolicy{
			{
				Port:     80,
				Protocol: core.SocketAddress_TCP,
				Rules:    []*cilium.PortNetworkPolicyRule{},
			},
			{
				Port:     80,
				Protocol: core.SocketAddress_TCP,
				Rules:    []*cilium.PortNetworkPolicyRule{},
			},
		},
	}
	c.Assert(splashBrothersPolicy, comparator.DeepEquals, expectedPolicy)

	err = OptimizeNetworkPolicy(splashBrothersPolicy)
	c.Assert(err, IsNil)
	expectedPolicy = &cilium.NetworkPolicy{
		Policy: uint64(identitySplashBrothers),
		IngressPerPortPolicies: []*cilium.PortNetworkPolicy{
			{
				Port:     80,
				Protocol: core.SocketAddress_TCP,
				Rules:    []*cilium.PortNetworkPolicyRule{},
			},
		},
	}
	c.Assert(splashBrothersPolicy, comparator.DeepEquals, expectedPolicy)

	// pkg/policy/l4_filter_test.go:TestMergeAllowAllL3AndAllowAllL7 Case 1B
	rules = api.Rules{
		&api.Rule{
			EndpointSelector: endpointSelectorSplashBros,
			Ingress: []api.IngressRule{
				{
					FromEndpoints: []api.EndpointSelector{},
					ToPorts: []api.PortRule{{
						Ports: []api.PortProtocol{
							{Port: "80", Protocol: api.ProtoTCP},
						},
					}},
				},
				{
					FromEndpoints: []api.EndpointSelector{},
					ToPorts: []api.PortRule{{
						Ports: []api.PortProtocol{
							{Port: "80", Protocol: api.ProtoTCP},
						},
					}},
				},
			},
		},
	}
	splashBrothersPolicy = ResolveIdentityPolicy(rules, identityCache, identitySplashBrothers)
	expectedPolicy = &cilium.NetworkPolicy{
		Policy: uint64(identitySplashBrothers),
		IngressPerPortPolicies: []*cilium.PortNetworkPolicy{
			{
				Port:     80,
				Protocol: core.SocketAddress_TCP,
				Rules:    []*cilium.PortNetworkPolicyRule{},
			},
			{
				Port:     80,
				Protocol: core.SocketAddress_TCP,
				Rules:    []*cilium.PortNetworkPolicyRule{},
			},
		},
	}
	c.Assert(splashBrothersPolicy, comparator.DeepEquals, expectedPolicy)
	err = OptimizeNetworkPolicy(splashBrothersPolicy)
	c.Assert(err, IsNil)
	expectedPolicy = &cilium.NetworkPolicy{
		Policy: uint64(identitySplashBrothers),
		IngressPerPortPolicies: []*cilium.PortNetworkPolicy{
			{
				Port:     80,
				Protocol: core.SocketAddress_TCP,
				Rules:    []*cilium.PortNetworkPolicyRule{},
			},
		},
	}
	c.Assert(splashBrothersPolicy, comparator.DeepEquals, expectedPolicy)
}
func (ds *ResolverTestSuite) TestMergeAllowAllL3AndAllowAllL7(c *C) {
	identityCache := initIdentityCache()

	// pkg/policy/l4_filter_test.go:TestMergeAllowAllL3AndAllowAllL7 Case 2A
	rules := api.Rules{
		&api.Rule{
			EndpointSelector: endpointSelectorSplashBros,
			Ingress: []api.IngressRule{
				{
					FromEndpoints: []api.EndpointSelector{api.WildcardEndpointSelector},
					ToPorts: []api.PortRule{{
						Ports: []api.PortProtocol{
							{Port: "80", Protocol: api.ProtoTCP},
						},
					}},
				},
				{
					FromEndpoints: []api.EndpointSelector{api.WildcardEndpointSelector},
					ToPorts: []api.PortRule{{
						Ports: []api.PortProtocol{
							{Port: "80", Protocol: api.ProtoTCP},
						},
						Rules: &api.L7Rules{
							HTTP: []api.PortRuleHTTP{
								{Method: "GET", Path: "/"},
							},
						},
					}},
				},
			},
		},
	}
	splashBrothersPolicy := ResolveIdentityPolicy(rules, identityCache, identitySplashBrothers)
	expectedPolicy := &cilium.NetworkPolicy{
		Policy: uint64(identitySplashBrothers),
		IngressPerPortPolicies: []*cilium.PortNetworkPolicy{
			{
				Port:     80,
				Protocol: core.SocketAddress_TCP,
				Rules:    []*cilium.PortNetworkPolicyRule{},
			},
			{
				Port:     80,
				Protocol: core.SocketAddress_TCP,
				Rules: []*cilium.PortNetworkPolicyRule{
					{
						L7Rules: &cilium.PortNetworkPolicyRule_HttpRules{
							HttpRules: &cilium.HttpNetworkPolicyRules{
								HttpRules: []*cilium.HttpNetworkPolicyRule{
									{
										Headers: []*envoy_api_v2_route.HeaderMatcher{
											{
												Name:  ":method",
												Value: "GET",
												Regex: &wrappers.BoolValue{Value: true},
											},
											{
												Name:  ":path",
												Value: "/",
												Regex: &wrappers.BoolValue{Value: true},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
	c.Assert(splashBrothersPolicy, comparator.DeepEquals, expectedPolicy)
	err := OptimizeNetworkPolicy(splashBrothersPolicy)
	c.Assert(err, IsNil)
	expectedPolicy = &cilium.NetworkPolicy{
		Policy: uint64(identitySplashBrothers),
		IngressPerPortPolicies: []*cilium.PortNetworkPolicy{
			{
				Port:     80,
				Protocol: core.SocketAddress_TCP,
				Rules: []*cilium.PortNetworkPolicyRule{
					{
						L7Rules: &cilium.PortNetworkPolicyRule_HttpRules{
							HttpRules: &cilium.HttpNetworkPolicyRules{
								HttpRules: []*cilium.HttpNetworkPolicyRule{
									{},
								},
							},
						},
					},
				},
			},
		},
	}
	c.Assert(splashBrothersPolicy, comparator.DeepEquals, expectedPolicy)

	// pkg/policy/l4_filter_test.go:TestMergeAllowAllL3AndAllowAllL7 Case 2B,
	// for HTTP.
	rules = api.Rules{
		&api.Rule{
			EndpointSelector: endpointSelectorSplashBros,
			Ingress: []api.IngressRule{
				{
					FromEndpoints: []api.EndpointSelector{api.WildcardEndpointSelector},
					ToPorts: []api.PortRule{{
						Ports: []api.PortProtocol{
							{Port: "80", Protocol: api.ProtoTCP},
						},
						Rules: &api.L7Rules{
							HTTP: []api.PortRuleHTTP{
								{Method: "GET", Path: "/"},
							},
						},
					}},
				},
				{
					FromEndpoints: []api.EndpointSelector{api.WildcardEndpointSelector},
					ToPorts: []api.PortRule{{
						Ports: []api.PortProtocol{
							{Port: "80", Protocol: api.ProtoTCP},
						},
					}},
				},
			},
		},
	}
	splashBrothersPolicy = ResolveIdentityPolicy(rules, identityCache, identitySplashBrothers)
	expectedPolicy = &cilium.NetworkPolicy{
		Policy: uint64(identitySplashBrothers),
		IngressPerPortPolicies: []*cilium.PortNetworkPolicy{
			{
				Port:     80,
				Protocol: core.SocketAddress_TCP,
				Rules:    []*cilium.PortNetworkPolicyRule{},
			},
			{
				Port:     80,
				Protocol: core.SocketAddress_TCP,
				Rules: []*cilium.PortNetworkPolicyRule{
					{
						L7Rules: &cilium.PortNetworkPolicyRule_HttpRules{
							HttpRules: &cilium.HttpNetworkPolicyRules{
								HttpRules: []*cilium.HttpNetworkPolicyRule{
									{
										Headers: []*envoy_api_v2_route.HeaderMatcher{
											{
												Name:  ":method",
												Value: "GET",
												Regex: &wrappers.BoolValue{Value: true},
											},
											{
												Name:  ":path",
												Value: "/",
												Regex: &wrappers.BoolValue{Value: true},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
	c.Assert(splashBrothersPolicy, comparator.DeepEquals, expectedPolicy)
	err = OptimizeNetworkPolicy(splashBrothersPolicy)
	c.Assert(err, IsNil)
	expectedPolicy = &cilium.NetworkPolicy{
		Policy: uint64(identitySplashBrothers),
		IngressPerPortPolicies: []*cilium.PortNetworkPolicy{
			{
				Port:     80,
				Protocol: core.SocketAddress_TCP,
				Rules: []*cilium.PortNetworkPolicyRule{
					{
						L7Rules: &cilium.PortNetworkPolicyRule_HttpRules{
							HttpRules: &cilium.HttpNetworkPolicyRules{
								HttpRules: []*cilium.HttpNetworkPolicyRule{
									{},
								},
							},
						},
					},
				},
			},
		},
	}
	c.Assert(splashBrothersPolicy, comparator.DeepEquals, expectedPolicy)

	// pkg/policy/l4_filter_test.go:TestMergeAllowAllL3AndAllowAllL7 Case 2B,
	// for Kafka.
	rules = api.Rules{
		&api.Rule{
			EndpointSelector: endpointSelectorSplashBros,
			Ingress: []api.IngressRule{
				{
					FromEndpoints: []api.EndpointSelector{api.WildcardEndpointSelector},
					ToPorts: []api.PortRule{{
						Ports: []api.PortProtocol{
							{Port: "80", Protocol: api.ProtoTCP},
						},
						Rules: &api.L7Rules{
							Kafka: []api.PortRuleKafka{
								{
									APIVersion: "1",
									APIKey:     "createtopics",
									Topic:      "foo",
								},
							},
						},
					}},
				},
				{
					FromEndpoints: []api.EndpointSelector{api.WildcardEndpointSelector},
					ToPorts: []api.PortRule{{
						Ports: []api.PortProtocol{
							{Port: "80", Protocol: api.ProtoTCP},
						},
					}},
				},
			},
		},
	}
	splashBrothersPolicy = ResolveIdentityPolicy(rules, identityCache, identitySplashBrothers)
	expectedPolicy = &cilium.NetworkPolicy{
		Policy: uint64(identitySplashBrothers),
		IngressPerPortPolicies: []*cilium.PortNetworkPolicy{
			{
				Port:     80,
				Protocol: core.SocketAddress_TCP,
				Rules:    []*cilium.PortNetworkPolicyRule{},
			},
			{
				Port:     80,
				Protocol: core.SocketAddress_TCP,
				Rules: []*cilium.PortNetworkPolicyRule{
					{
						L7Rules: &cilium.PortNetworkPolicyRule_KafkaRules{
							KafkaRules: &cilium.KafkaNetworkPolicyRules{
								KafkaRules: []*cilium.KafkaNetworkPolicyRule{
									{
										ApiVersion: 1,
										ApiKey:     19,
										Topic:      "foo",
										ClientId:   "",
									},
								},
							},
						},
					},
				},
			},
		},
	}
	c.Assert(splashBrothersPolicy, comparator.DeepEquals, expectedPolicy)
	err = OptimizeNetworkPolicy(splashBrothersPolicy)
	c.Assert(err, IsNil)
	expectedPolicy = &cilium.NetworkPolicy{
		Policy: uint64(identitySplashBrothers),
		IngressPerPortPolicies: []*cilium.PortNetworkPolicy{
			{
				Port:     80,
				Protocol: core.SocketAddress_TCP,
				Rules: []*cilium.PortNetworkPolicyRule{
					{
						L7Rules: &cilium.PortNetworkPolicyRule_KafkaRules{
							KafkaRules: &cilium.KafkaNetworkPolicyRules{
								KafkaRules: []*cilium.KafkaNetworkPolicyRule{
									{},
								},
							},
						},
					},
				},
			},
		},
	}
	c.Assert(splashBrothersPolicy, comparator.DeepEquals, expectedPolicy)
}

// Case 3: allow all at L3 in both rules. Both rules have same parser type and
// same API resource specified at L7 for HTTP.
func (ds *ResolverTestSuite) TestMergeIdenticalAllowAllL3AndRestrictedL7HTTP(c *C) {
	identityCache := initIdentityCache()

	rules := api.Rules{
		&api.Rule{
			EndpointSelector: endpointSelectorSplashBros,
			Ingress: []api.IngressRule{
				{
					FromEndpoints: []api.EndpointSelector{api.WildcardEndpointSelector},
					ToPorts: []api.PortRule{{
						Ports: []api.PortProtocol{
							{Port: "80", Protocol: api.ProtoTCP},
						},
						Rules: &api.L7Rules{
							HTTP: []api.PortRuleHTTP{
								{Method: "GET", Path: "/"},
							},
						},
					}},
				},
				{
					FromEndpoints: []api.EndpointSelector{api.WildcardEndpointSelector},
					ToPorts: []api.PortRule{{
						Ports: []api.PortProtocol{
							{Port: "80", Protocol: api.ProtoTCP},
						},
						Rules: &api.L7Rules{
							HTTP: []api.PortRuleHTTP{
								{Method: "GET", Path: "/"},
							},
						},
					}},
				},
			},
		},
	}
	splashBrothersPolicy := ResolveIdentityPolicy(rules, identityCache, identitySplashBrothers)
	expectedPolicy := &cilium.NetworkPolicy{
		Policy: uint64(identitySplashBrothers),
		IngressPerPortPolicies: []*cilium.PortNetworkPolicy{
			{
				Port:     80,
				Protocol: core.SocketAddress_TCP,
				Rules: []*cilium.PortNetworkPolicyRule{
					{
						L7Rules: &cilium.PortNetworkPolicyRule_HttpRules{
							HttpRules: &cilium.HttpNetworkPolicyRules{
								HttpRules: []*cilium.HttpNetworkPolicyRule{
									{
										Headers: []*envoy_api_v2_route.HeaderMatcher{
											{
												Name:  ":method",
												Value: "GET",
												Regex: &wrappers.BoolValue{Value: true},
											},
											{
												Name:  ":path",
												Value: "/",
												Regex: &wrappers.BoolValue{Value: true},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			{
				Port:     80,
				Protocol: core.SocketAddress_TCP,
				Rules: []*cilium.PortNetworkPolicyRule{
					{
						L7Rules: &cilium.PortNetworkPolicyRule_HttpRules{
							HttpRules: &cilium.HttpNetworkPolicyRules{
								HttpRules: []*cilium.HttpNetworkPolicyRule{
									{
										Headers: []*envoy_api_v2_route.HeaderMatcher{
											{
												Name:  ":method",
												Value: "GET",
												Regex: &wrappers.BoolValue{Value: true},
											},
											{
												Name:  ":path",
												Value: "/",
												Regex: &wrappers.BoolValue{Value: true},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
	c.Assert(splashBrothersPolicy, comparator.DeepEquals, expectedPolicy)
	err := OptimizeNetworkPolicy(splashBrothersPolicy)
	c.Assert(err, IsNil)
	expectedPolicy = &cilium.NetworkPolicy{
		Policy: uint64(identitySplashBrothers),
		IngressPerPortPolicies: []*cilium.PortNetworkPolicy{
			{
				Port:     80,
				Protocol: core.SocketAddress_TCP,
				Rules: []*cilium.PortNetworkPolicyRule{
					{
						L7Rules: &cilium.PortNetworkPolicyRule_HttpRules{
							HttpRules: &cilium.HttpNetworkPolicyRules{
								HttpRules: []*cilium.HttpNetworkPolicyRule{
									{
										Headers: []*envoy_api_v2_route.HeaderMatcher{
											{
												Name:  ":method",
												Value: "GET",
												Regex: &wrappers.BoolValue{Value: true},
											},
											{
												Name:  ":path",
												Value: "/",
												Regex: &wrappers.BoolValue{Value: true},
											},
										},
									},
								},
							},
						},
					},
					{
						L7Rules: &cilium.PortNetworkPolicyRule_HttpRules{
							HttpRules: &cilium.HttpNetworkPolicyRules{
								HttpRules: []*cilium.HttpNetworkPolicyRule{
									{
										Headers: []*envoy_api_v2_route.HeaderMatcher{
											{
												Name:  ":method",
												Value: "GET",
												Regex: &wrappers.BoolValue{Value: true},
											},
											{
												Name:  ":path",
												Value: "/",
												Regex: &wrappers.BoolValue{Value: true},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
	c.Assert(splashBrothersPolicy, comparator.DeepEquals, expectedPolicy)
}

// Case 4: identical allow all at L3 with identical restrictions on Kafka.
func (ds *ResolverTestSuite) TestMergeIdenticalAllowAllL3AndRestrictedL7Kafka(c *C) {
	identityCache := initIdentityCache()

	rules := api.Rules{
		&api.Rule{
			EndpointSelector: endpointSelectorSplashBros,
			Ingress: []api.IngressRule{
				{
					FromEndpoints: api.EndpointSelectorSlice{api.WildcardEndpointSelector},
					ToPorts: []api.PortRule{{
						Ports: []api.PortProtocol{
							{Port: "9092", Protocol: api.ProtoTCP},
						},
						Rules: &api.L7Rules{
							Kafka: []api.PortRuleKafka{
								{Topic: "foo"},
							},
						},
					}},
				},
				{
					FromEndpoints: api.EndpointSelectorSlice{api.WildcardEndpointSelector},
					ToPorts: []api.PortRule{{
						Ports: []api.PortProtocol{
							{Port: "9092", Protocol: api.ProtoTCP},
						},
						Rules: &api.L7Rules{
							Kafka: []api.PortRuleKafka{
								{Topic: "foo"},
							},
						},
					}},
				},
			},
		},
	}
	splashBrothersPolicy := ResolveIdentityPolicy(rules, identityCache, identitySplashBrothers)
	expectedPolicy := &cilium.NetworkPolicy{
		Policy: uint64(identitySplashBrothers),
		IngressPerPortPolicies: []*cilium.PortNetworkPolicy{
			{
				Port:     9092,
				Protocol: core.SocketAddress_TCP,
				Rules: []*cilium.PortNetworkPolicyRule{
					{
						L7Rules: &cilium.PortNetworkPolicyRule_KafkaRules{
							KafkaRules: &cilium.KafkaNetworkPolicyRules{
								KafkaRules: []*cilium.KafkaNetworkPolicyRule{
									{
										ApiVersion: -1,
										ApiKey:     -1,
										// Topic is empty because all requests
										// are allowed if APIKey is not specified
										// in api.PortRuleKafka.
										Topic:    "",
										ClientId: "",
									},
								},
							},
						},
					},
				},
			},
			{
				Port:     9092,
				Protocol: core.SocketAddress_TCP,
				Rules: []*cilium.PortNetworkPolicyRule{
					{
						L7Rules: &cilium.PortNetworkPolicyRule_KafkaRules{
							KafkaRules: &cilium.KafkaNetworkPolicyRules{
								KafkaRules: []*cilium.KafkaNetworkPolicyRule{
									{
										ApiVersion: -1,
										ApiKey:     -1,
										Topic:      "",
										ClientId:   "",
									},
								},
							},
						},
					},
				},
			},
		},
	}
	c.Assert(splashBrothersPolicy, comparator.DeepEquals, expectedPolicy)
	err := OptimizeNetworkPolicy(splashBrothersPolicy)
	c.Assert(err, IsNil)
	expectedPolicy = &cilium.NetworkPolicy{
		Policy: uint64(identitySplashBrothers),
		IngressPerPortPolicies: []*cilium.PortNetworkPolicy{
			{
				Port:     9092,
				Protocol: core.SocketAddress_TCP,
				Rules: []*cilium.PortNetworkPolicyRule{
					{
						L7Rules: &cilium.PortNetworkPolicyRule_KafkaRules{
							KafkaRules: &cilium.KafkaNetworkPolicyRules{
								KafkaRules: []*cilium.KafkaNetworkPolicyRule{
									{
										ApiVersion: -1,
										ApiKey:     -1,
										Topic:      "",
										ClientId:   "",
									},
								},
							},
						},
					},
					{
						L7Rules: &cilium.PortNetworkPolicyRule_KafkaRules{
							KafkaRules: &cilium.KafkaNetworkPolicyRules{
								KafkaRules: []*cilium.KafkaNetworkPolicyRule{
									{
										ApiVersion: -1,
										ApiKey:     -1,
										Topic:      "",
										ClientId:   "",
									},
								},
							},
						},
					},
				},
			},
		},
	}
	c.Assert(splashBrothersPolicy, comparator.DeepEquals, expectedPolicy)

	rules = api.Rules{
		&api.Rule{
			EndpointSelector: endpointSelectorSplashBros,
			Ingress: []api.IngressRule{
				{
					FromEndpoints: api.EndpointSelectorSlice{api.WildcardEndpointSelector},
					ToPorts: []api.PortRule{{
						Ports: []api.PortProtocol{
							{Port: "9092", Protocol: api.ProtoTCP},
						},
						Rules: &api.L7Rules{
							Kafka: []api.PortRuleKafka{
								{
									APIKey: "produce",
									Topic:  "foo",
								},
							},
						},
					}},
				},
				{
					FromEndpoints: api.EndpointSelectorSlice{api.WildcardEndpointSelector},
					ToPorts: []api.PortRule{{
						Ports: []api.PortProtocol{
							{Port: "9092", Protocol: api.ProtoTCP},
						},
						Rules: &api.L7Rules{
							Kafka: []api.PortRuleKafka{
								{
									APIKey: "produce",
									Topic:  "foo",
								},
							},
						},
					}},
				},
			},
		},
	}
	splashBrothersPolicy = ResolveIdentityPolicy(rules, identityCache, identitySplashBrothers)
	expectedPolicy = &cilium.NetworkPolicy{
		Policy: uint64(identitySplashBrothers),
		IngressPerPortPolicies: []*cilium.PortNetworkPolicy{
			{
				Port:     9092,
				Protocol: core.SocketAddress_TCP,
				Rules: []*cilium.PortNetworkPolicyRule{
					{
						L7Rules: &cilium.PortNetworkPolicyRule_KafkaRules{
							KafkaRules: &cilium.KafkaNetworkPolicyRules{
								KafkaRules: []*cilium.KafkaNetworkPolicyRule{
									{
										ApiVersion: -1,
										ApiKey:     0,
										// Topic is empty because all requests
										// are allowed if APIKey is not specified
										// in api.PortRuleKafka.
										Topic:    "foo",
										ClientId: "",
									},
								},
							},
						},
					},
				},
			},
			{
				Port:     9092,
				Protocol: core.SocketAddress_TCP,
				Rules: []*cilium.PortNetworkPolicyRule{
					{
						L7Rules: &cilium.PortNetworkPolicyRule_KafkaRules{
							KafkaRules: &cilium.KafkaNetworkPolicyRules{
								KafkaRules: []*cilium.KafkaNetworkPolicyRule{
									{
										ApiVersion: -1,
										ApiKey:     0,
										Topic:      "foo",
										ClientId:   "",
									},
								},
							},
						},
					},
				},
			},
		},
	}
	c.Assert(splashBrothersPolicy, comparator.DeepEquals, expectedPolicy)
	err = OptimizeNetworkPolicy(splashBrothersPolicy)
	c.Assert(err, IsNil)
	expectedPolicy = &cilium.NetworkPolicy{
		Policy: uint64(identitySplashBrothers),
		IngressPerPortPolicies: []*cilium.PortNetworkPolicy{
			{
				Port:     9092,
				Protocol: core.SocketAddress_TCP,
				Rules: []*cilium.PortNetworkPolicyRule{
					{
						L7Rules: &cilium.PortNetworkPolicyRule_KafkaRules{
							KafkaRules: &cilium.KafkaNetworkPolicyRules{
								KafkaRules: []*cilium.KafkaNetworkPolicyRule{
									{
										ApiVersion: -1,
										ApiKey:     0,
										Topic:      "foo",
										ClientId:   "",
									},
								},
							},
						},
					},
					{
						L7Rules: &cilium.PortNetworkPolicyRule_KafkaRules{
							KafkaRules: &cilium.KafkaNetworkPolicyRules{
								KafkaRules: []*cilium.KafkaNetworkPolicyRule{
									{
										ApiVersion: -1,
										ApiKey:     0,
										Topic:      "foo",
										ClientId:   "",
									},
								},
							},
						},
					},
				},
			},
		},
	}
	c.Assert(splashBrothersPolicy, comparator.DeepEquals, expectedPolicy)
}

// Case 5: use conflicting protocols on the same port in different rules. This
// is not supported, so return an error.
func (ds *ResolverTestSuite) TestConflictingL7OnSamePort(c *C) {
	identityCache := initIdentityCache()

	rules := api.Rules{
		&api.Rule{
			EndpointSelector: endpointSelectorSplashBros,
			Ingress: []api.IngressRule{
				{
					FromEndpoints: []api.EndpointSelector{endpointSelectorSplashBros},
					ToPorts: []api.PortRule{{
						Ports: []api.PortProtocol{
							{Port: "80", Protocol: api.ProtoTCP},
						},
						Rules: &api.L7Rules{
							Kafka: []api.PortRuleKafka{
								{Topic: "foo"},
							},
						},
					}},
				},
				{
					FromEndpoints: []api.EndpointSelector{api.WildcardEndpointSelector},
					ToPorts: []api.PortRule{{
						Ports: []api.PortProtocol{
							{Port: "80", Protocol: api.ProtoTCP},
						},
						Rules: &api.L7Rules{
							HTTP: []api.PortRuleHTTP{
								{Method: "GET", Path: "/"},
							},
						},
					}},
				},
			},
		},
	}
	splashBrothersPolicy := ResolveIdentityPolicy(rules, identityCache, identitySplashBrothers)
	err := OptimizeNetworkPolicy(splashBrothersPolicy)
	c.Assert(err, Not(IsNil))

	// Test reverse order to ensure that we error out if HTTP rule has already
	// been parsed, and then we hit a Kafka rule applying at the same port.
	rules = api.Rules{
		&api.Rule{
			EndpointSelector: endpointSelectorSplashBros,
			Ingress: []api.IngressRule{
				{
					FromEndpoints: []api.EndpointSelector{api.WildcardEndpointSelector},
					ToPorts: []api.PortRule{{
						Ports: []api.PortProtocol{
							{Port: "80", Protocol: api.ProtoTCP},
						},
						Rules: &api.L7Rules{
							HTTP: []api.PortRuleHTTP{
								{Method: "GET", Path: "/"},
							},
						},
					}},
				},
				{
					FromEndpoints: []api.EndpointSelector{endpointSelectorSplashBros},
					ToPorts: []api.PortRule{{
						Ports: []api.PortProtocol{
							{Port: "80", Protocol: api.ProtoTCP},
						},
						Rules: &api.L7Rules{
							Kafka: []api.PortRuleKafka{
								{Topic: "foo"},
							},
						},
					}},
				},
			},
		},
	}
	splashBrothersPolicy = ResolveIdentityPolicy(rules, identityCache, identitySplashBrothers)
	err = OptimizeNetworkPolicy(splashBrothersPolicy)
	c.Assert(err, Not(IsNil))

}

// Case 6: allow all at L3/L7 in one rule, and select an endpoint and allow all on L7
// in another rule. Should resolve to just allowing all on L3/L7 (first rule
// shadows the second).
func (ds *ResolverTestSuite) TestL3RuleShadowedByL3AllowAll(c *C) {
	identityCache := initIdentityCache()

	// Case 6A: Specify WildcardEndpointSelector explicitly.
	rules := api.Rules{
		&api.Rule{
			EndpointSelector: endpointSelectorSplashBros,
			Ingress: []api.IngressRule{
				{
					FromEndpoints: []api.EndpointSelector{endpointSelectorDurant},
					ToPorts: []api.PortRule{{
						Ports: []api.PortProtocol{
							{Port: "80", Protocol: api.ProtoTCP},
						},
					}},
				},
				{
					FromEndpoints: []api.EndpointSelector{api.WildcardEndpointSelector},
					ToPorts: []api.PortRule{{
						Ports: []api.PortProtocol{
							{Port: "80", Protocol: api.ProtoTCP},
						},
					}},
				},
			},
		},
	}

	splashBrothersPolicy := ResolveIdentityPolicy(rules, identityCache, identitySplashBrothers)
	expectedPolicy := &cilium.NetworkPolicy{
		Policy: uint64(identitySplashBrothers),
		IngressPerPortPolicies: []*cilium.PortNetworkPolicy{
			{
				Port:     80,
				Protocol: core.SocketAddress_TCP,
				Rules:    []*cilium.PortNetworkPolicyRule{},
			},
			{
				Port:     80,
				Protocol: core.SocketAddress_TCP,
				Rules: []*cilium.PortNetworkPolicyRule{
					{
						RemotePolicies: []uint64{uint64(identity35)},
					},
				},
			},
			{
				Port:     80,
				Protocol: core.SocketAddress_TCP,
				Rules: []*cilium.PortNetworkPolicyRule{
					{
						RemotePolicies: []uint64{uint64(identityWarriors)},
					},
				},
			},
		},
	}
	c.Assert(splashBrothersPolicy, comparator.DeepEquals, expectedPolicy)
	err := OptimizeNetworkPolicy(splashBrothersPolicy)
	c.Assert(err, IsNil)
	expectedPolicy = &cilium.NetworkPolicy{
		Policy: uint64(identitySplashBrothers),
		IngressPerPortPolicies: []*cilium.PortNetworkPolicy{
			{
				Port:     80,
				Protocol: core.SocketAddress_TCP,
				Rules:    []*cilium.PortNetworkPolicyRule{},
			},
		},
	}
	c.Assert(splashBrothersPolicy, comparator.DeepEquals, expectedPolicy)

}

// Case 7: allow all at L3/L7 in one rule, and in another rule, select an endpoint
// which restricts on L7. Should resolve to just allowing all on L3/L7 (first rule
// shadows the second), but setting traffic to the HTTP proxy.
func (ds *ResolverTestSuite) TestL3RuleWithL7RulePartiallyShadowedByL3AllowAll(c *C) {
	identityCache := initIdentityCache()

	rules := api.Rules{
		&api.Rule{
			EndpointSelector: endpointSelectorSplashBros,
			Ingress: []api.IngressRule{
				{
					FromEndpoints: []api.EndpointSelector{endpointSelectorSplashBros},
					ToPorts: []api.PortRule{{
						Ports: []api.PortProtocol{
							{Port: "80", Protocol: api.ProtoTCP},
						},
						Rules: &api.L7Rules{
							HTTP: []api.PortRuleHTTP{
								{Method: "GET", Path: "/"},
							},
						},
					}},
				},
				{
					FromEndpoints: []api.EndpointSelector{api.WildcardEndpointSelector},
					ToPorts: []api.PortRule{{
						Ports: []api.PortProtocol{
							{Port: "80", Protocol: api.ProtoTCP},
						},
					}},
				},
			},
		},
	}

	splashBrothersPolicy := ResolveIdentityPolicy(rules, identityCache, identitySplashBrothers)
	expectedPolicy := &cilium.NetworkPolicy{
		Policy: uint64(identitySplashBrothers),
		IngressPerPortPolicies: []*cilium.PortNetworkPolicy{
			{
				Port:     80,
				Protocol: core.SocketAddress_TCP,
				Rules:    []*cilium.PortNetworkPolicyRule{},
			},
			{
				Port:     80,
				Protocol: core.SocketAddress_TCP,
				Rules: []*cilium.PortNetworkPolicyRule{
					{
						RemotePolicies: []uint64{uint64(identitySplashBrothers)},
						L7Rules: &cilium.PortNetworkPolicyRule_HttpRules{
							HttpRules: &cilium.HttpNetworkPolicyRules{
								HttpRules: []*cilium.HttpNetworkPolicyRule{
									{
										Headers: []*envoy_api_v2_route.HeaderMatcher{
											{
												Name:  ":method",
												Value: "GET",
												Regex: &wrappers.BoolValue{Value: true},
											},
											{
												Name:  ":path",
												Value: "/",
												Regex: &wrappers.BoolValue{Value: true},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			{
				Port:     80,
				Protocol: core.SocketAddress_TCP,
				Rules: []*cilium.PortNetworkPolicyRule{
					{
						RemotePolicies: []uint64{uint64(identityWarriors)},
						L7Rules: &cilium.PortNetworkPolicyRule_HttpRules{
							HttpRules: &cilium.HttpNetworkPolicyRules{
								HttpRules: []*cilium.HttpNetworkPolicyRule{
									{
										Headers: []*envoy_api_v2_route.HeaderMatcher{
											{
												Name:  ":method",
												Value: "GET",
												Regex: &wrappers.BoolValue{Value: true},
											},
											{
												Name:  ":path",
												Value: "/",
												Regex: &wrappers.BoolValue{Value: true},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
	c.Assert(splashBrothersPolicy, comparator.DeepEquals, expectedPolicy)
	err := OptimizeNetworkPolicy(splashBrothersPolicy)
	c.Assert(err, IsNil)
	expectedPolicy = &cilium.NetworkPolicy{
		Policy: uint64(identitySplashBrothers),
		IngressPerPortPolicies: []*cilium.PortNetworkPolicy{
			{
				Port:     80,
				Protocol: core.SocketAddress_TCP,
				Rules: []*cilium.PortNetworkPolicyRule{
					{
						L7Rules: &cilium.PortNetworkPolicyRule_HttpRules{
							HttpRules: &cilium.HttpNetworkPolicyRules{
								HttpRules: []*cilium.HttpNetworkPolicyRule{
									{},
								},
							},
						},
					},
				},
			},
		},
	}
	c.Assert(splashBrothersPolicy, comparator.DeepEquals, expectedPolicy)

	rules = api.Rules{
		&api.Rule{
			EndpointSelector: endpointSelectorSplashBros,
			Ingress: []api.IngressRule{
				{
					FromEndpoints: []api.EndpointSelector{api.WildcardEndpointSelector},
					ToPorts: []api.PortRule{{
						Ports: []api.PortProtocol{
							{Port: "80", Protocol: api.ProtoTCP},
						},
					}},
				},
				{
					FromEndpoints: []api.EndpointSelector{endpointSelectorSplashBros},
					ToPorts: []api.PortRule{{
						Ports: []api.PortProtocol{
							{Port: "80", Protocol: api.ProtoTCP},
						},
						Rules: &api.L7Rules{
							HTTP: []api.PortRuleHTTP{
								{Method: "GET", Path: "/"},
							},
						},
					}},
				},
			},
		},
	}

	splashBrothersPolicy = ResolveIdentityPolicy(rules, identityCache, identitySplashBrothers)
	expectedPolicy = &cilium.NetworkPolicy{
		Policy: uint64(identitySplashBrothers),
		IngressPerPortPolicies: []*cilium.PortNetworkPolicy{
			{
				Port:     80,
				Protocol: core.SocketAddress_TCP,
				Rules:    []*cilium.PortNetworkPolicyRule{},
			},
			{
				Port:     80,
				Protocol: core.SocketAddress_TCP,
				Rules: []*cilium.PortNetworkPolicyRule{
					{
						RemotePolicies: []uint64{uint64(identitySplashBrothers)},
						L7Rules: &cilium.PortNetworkPolicyRule_HttpRules{
							HttpRules: &cilium.HttpNetworkPolicyRules{
								HttpRules: []*cilium.HttpNetworkPolicyRule{
									{
										Headers: []*envoy_api_v2_route.HeaderMatcher{
											{
												Name:  ":method",
												Value: "GET",
												Regex: &wrappers.BoolValue{Value: true},
											},
											{
												Name:  ":path",
												Value: "/",
												Regex: &wrappers.BoolValue{Value: true},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			{
				Port:     80,
				Protocol: core.SocketAddress_TCP,
				Rules: []*cilium.PortNetworkPolicyRule{
					{
						RemotePolicies: []uint64{uint64(identityWarriors)},
						L7Rules: &cilium.PortNetworkPolicyRule_HttpRules{
							HttpRules: &cilium.HttpNetworkPolicyRules{
								HttpRules: []*cilium.HttpNetworkPolicyRule{
									{
										Headers: []*envoy_api_v2_route.HeaderMatcher{
											{
												Name:  ":method",
												Value: "GET",
												Regex: &wrappers.BoolValue{Value: true},
											},
											{
												Name:  ":path",
												Value: "/",
												Regex: &wrappers.BoolValue{Value: true},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
	c.Assert(splashBrothersPolicy, comparator.DeepEquals, expectedPolicy)
	err = OptimizeNetworkPolicy(splashBrothersPolicy)
	c.Assert(err, IsNil)
	expectedPolicy = &cilium.NetworkPolicy{
		Policy: uint64(identitySplashBrothers),
		IngressPerPortPolicies: []*cilium.PortNetworkPolicy{
			{
				Port:     80,
				Protocol: core.SocketAddress_TCP,
				Rules: []*cilium.PortNetworkPolicyRule{
					{
						L7Rules: &cilium.PortNetworkPolicyRule_HttpRules{
							HttpRules: &cilium.HttpNetworkPolicyRules{
								HttpRules: []*cilium.HttpNetworkPolicyRule{
									{},
								},
							},
						},
					},
				},
			},
		},
	}
	c.Assert(splashBrothersPolicy, comparator.DeepEquals, expectedPolicy)
}

// Case 8: allow all at L3 and restricts on L7 in one rule, and in another rule,
// select an endpoint which restricts the same as the first rule on L7.
// Should resolve to just allowing all on L3, but restricting on L7 for both
// wildcard and the specified endpoint.
func (ds *ResolverTestSuite) TestL3RuleWithL7RuleShadowedByL3AllowAll(c *C) {
	identityCache := initIdentityCache()

	rules := api.Rules{
		&api.Rule{
			EndpointSelector: endpointSelectorSplashBros,
			Ingress: []api.IngressRule{
				{
					FromEndpoints: []api.EndpointSelector{endpointSelectorSplashBros},
					ToPorts: []api.PortRule{{
						Ports: []api.PortProtocol{
							{Port: "80", Protocol: api.ProtoTCP},
						},
						Rules: &api.L7Rules{
							HTTP: []api.PortRuleHTTP{
								{Method: "GET", Path: "/"},
							},
						},
					}},
				},
				{
					FromEndpoints: []api.EndpointSelector{api.WildcardEndpointSelector},
					ToPorts: []api.PortRule{{
						Ports: []api.PortProtocol{
							{Port: "80", Protocol: api.ProtoTCP},
						},
						Rules: &api.L7Rules{
							HTTP: []api.PortRuleHTTP{
								{Method: "GET", Path: "/"},
							},
						},
					}},
				},
			},
		},
	}

	splashBrothersPolicy := ResolveIdentityPolicy(rules, identityCache, identitySplashBrothers)
	expectedPolicy := &cilium.NetworkPolicy{
		Policy: uint64(identitySplashBrothers),
		IngressPerPortPolicies: []*cilium.PortNetworkPolicy{
			{
				Port:     80,
				Protocol: core.SocketAddress_TCP,
				Rules: []*cilium.PortNetworkPolicyRule{
					{
						L7Rules: &cilium.PortNetworkPolicyRule_HttpRules{
							HttpRules: &cilium.HttpNetworkPolicyRules{
								HttpRules: []*cilium.HttpNetworkPolicyRule{
									{
										Headers: []*envoy_api_v2_route.HeaderMatcher{
											{
												Name:  ":method",
												Value: "GET",
												Regex: &wrappers.BoolValue{Value: true},
											},
											{
												Name:  ":path",
												Value: "/",
												Regex: &wrappers.BoolValue{Value: true},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			{
				Port:     80,
				Protocol: core.SocketAddress_TCP,
				Rules: []*cilium.PortNetworkPolicyRule{
					{
						RemotePolicies: []uint64{uint64(identitySplashBrothers)},
						L7Rules: &cilium.PortNetworkPolicyRule_HttpRules{
							HttpRules: &cilium.HttpNetworkPolicyRules{
								HttpRules: []*cilium.HttpNetworkPolicyRule{
									{
										Headers: []*envoy_api_v2_route.HeaderMatcher{
											{
												Name:  ":method",
												Value: "GET",
												Regex: &wrappers.BoolValue{Value: true},
											},
											{
												Name:  ":path",
												Value: "/",
												Regex: &wrappers.BoolValue{Value: true},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			{
				Port:     80,
				Protocol: core.SocketAddress_TCP,
				Rules: []*cilium.PortNetworkPolicyRule{
					{
						RemotePolicies: []uint64{uint64(identityWarriors)},
						L7Rules: &cilium.PortNetworkPolicyRule_HttpRules{
							HttpRules: &cilium.HttpNetworkPolicyRules{
								HttpRules: []*cilium.HttpNetworkPolicyRule{
									{
										Headers: []*envoy_api_v2_route.HeaderMatcher{
											{
												Name:  ":method",
												Value: "GET",
												Regex: &wrappers.BoolValue{Value: true},
											},
											{
												Name:  ":path",
												Value: "/",
												Regex: &wrappers.BoolValue{Value: true},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
	c.Assert(splashBrothersPolicy, comparator.DeepEquals, expectedPolicy)
	err := OptimizeNetworkPolicy(splashBrothersPolicy)
	c.Assert(err, IsNil)
	expectedPolicy = &cilium.NetworkPolicy{
		Policy: uint64(identitySplashBrothers),
		IngressPerPortPolicies: []*cilium.PortNetworkPolicy{
			{
				Port:     80,
				Protocol: core.SocketAddress_TCP,
				Rules: []*cilium.PortNetworkPolicyRule{
					{
						L7Rules: &cilium.PortNetworkPolicyRule_HttpRules{
							HttpRules: &cilium.HttpNetworkPolicyRules{
								HttpRules: []*cilium.HttpNetworkPolicyRule{
									{
										Headers: []*envoy_api_v2_route.HeaderMatcher{
											{
												Name:  ":method",
												Value: "GET",
												Regex: &wrappers.BoolValue{Value: true},
											},
											{
												Name:  ":path",
												Value: "/",
												Regex: &wrappers.BoolValue{Value: true},
											},
										},
									},
								},
							},
						},
					},
					{
						RemotePolicies: []uint64{uint64(identitySplashBrothers)},
						L7Rules: &cilium.PortNetworkPolicyRule_HttpRules{
							HttpRules: &cilium.HttpNetworkPolicyRules{
								HttpRules: []*cilium.HttpNetworkPolicyRule{
									{
										Headers: []*envoy_api_v2_route.HeaderMatcher{
											{
												Name:  ":method",
												Value: "GET",
												Regex: &wrappers.BoolValue{Value: true},
											},
											{
												Name:  ":path",
												Value: "/",
												Regex: &wrappers.BoolValue{Value: true},
											},
										},
									},
								},
							},
						},
					},
					{
						RemotePolicies: []uint64{uint64(identityWarriors)},
						L7Rules: &cilium.PortNetworkPolicyRule_HttpRules{
							HttpRules: &cilium.HttpNetworkPolicyRules{
								HttpRules: []*cilium.HttpNetworkPolicyRule{
									{
										Headers: []*envoy_api_v2_route.HeaderMatcher{
											{
												Name:  ":method",
												Value: "GET",
												Regex: &wrappers.BoolValue{Value: true},
											},
											{
												Name:  ":path",
												Value: "/",
												Regex: &wrappers.BoolValue{Value: true},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
	c.Assert(splashBrothersPolicy, comparator.DeepEquals, expectedPolicy)
}

// Case 9: allow all at L3 and restricts on L7 in one rule, and in another rule,
// select an endpoint which restricts on different L7 protocol.
// Should fail as cannot have conflicting parsers on same port.
func (ds *ResolverTestSuite) TestL3SelectingEndpointAndL3AllowAllMergeConflictingL7(c *C) {
	identityCache := initIdentityCache()
	rules := api.Rules{
		&api.Rule{
			EndpointSelector: endpointSelectorSplashBros,
			Ingress: []api.IngressRule{
				{
					FromEndpoints: []api.EndpointSelector{endpointSelectorSplashBros},
					ToPorts: []api.PortRule{{
						Ports: []api.PortProtocol{
							{Port: "80", Protocol: api.ProtoTCP},
						},
						Rules: &api.L7Rules{
							Kafka: []api.PortRuleKafka{
								{Topic: "foo"},
							},
						},
					}},
				},
				{
					FromEndpoints: []api.EndpointSelector{api.WildcardEndpointSelector},
					ToPorts: []api.PortRule{{
						Ports: []api.PortProtocol{
							{Port: "80", Protocol: api.ProtoTCP},
						},
						Rules: &api.L7Rules{
							HTTP: []api.PortRuleHTTP{
								{Method: "GET", Path: "/"},
							},
						},
					}},
				},
			},
		},
	}

	splashBrothersPolicy := ResolveIdentityPolicy(rules, identityCache, identitySplashBrothers)
	expectedPolicy := &cilium.NetworkPolicy{
		Policy: uint64(identitySplashBrothers),
		IngressPerPortPolicies: []*cilium.PortNetworkPolicy{
			{
				Port:     80,
				Protocol: core.SocketAddress_TCP,
				Rules: []*cilium.PortNetworkPolicyRule{
					{
						RemotePolicies: []uint64{uint64(identitySplashBrothers)},
						L7Rules: &cilium.PortNetworkPolicyRule_KafkaRules{
							KafkaRules: &cilium.KafkaNetworkPolicyRules{
								KafkaRules: []*cilium.KafkaNetworkPolicyRule{
									{
										ApiKey:     -1,
										ApiVersion: -1,
										Topic:      "",
										ClientId:   "",
									},
								},
							},
						},
					},
				},
			},
			{
				Port:     80,
				Protocol: core.SocketAddress_TCP,
				Rules: []*cilium.PortNetworkPolicyRule{
					{
						RemotePolicies: []uint64{uint64(identityWarriors)},
						L7Rules: &cilium.PortNetworkPolicyRule_KafkaRules{
							KafkaRules: &cilium.KafkaNetworkPolicyRules{
								KafkaRules: []*cilium.KafkaNetworkPolicyRule{
									{
										ApiKey:     -1,
										ApiVersion: -1,
										Topic:      "",
										ClientId:   "",
									},
								},
							},
						},
					},
				},
			},
			{
				Port:     80,
				Protocol: core.SocketAddress_TCP,
				Rules: []*cilium.PortNetworkPolicyRule{
					{
						L7Rules: &cilium.PortNetworkPolicyRule_HttpRules{
							HttpRules: &cilium.HttpNetworkPolicyRules{
								HttpRules: []*cilium.HttpNetworkPolicyRule{
									{
										Headers: []*envoy_api_v2_route.HeaderMatcher{
											{
												Name:  ":method",
												Value: "GET",
												Regex: &wrappers.BoolValue{Value: true},
											},
											{
												Name:  ":path",
												Value: "/",
												Regex: &wrappers.BoolValue{Value: true},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
	c.Assert(splashBrothersPolicy, comparator.DeepEquals, expectedPolicy)
	err := OptimizeNetworkPolicy(splashBrothersPolicy)
	c.Assert(err, Not(IsNil))

	rules = api.Rules{
		&api.Rule{
			EndpointSelector: endpointSelectorSplashBros,
			Ingress: []api.IngressRule{
				{
					FromEndpoints: []api.EndpointSelector{api.WildcardEndpointSelector},
					ToPorts: []api.PortRule{{
						Ports: []api.PortProtocol{
							{Port: "80", Protocol: api.ProtoTCP},
						},
						Rules: &api.L7Rules{
							HTTP: []api.PortRuleHTTP{
								{Method: "GET", Path: "/"},
							},
						},
					}},
				},
				{
					FromEndpoints: []api.EndpointSelector{endpointSelectorSplashBros},
					ToPorts: []api.PortRule{{
						Ports: []api.PortProtocol{
							{Port: "80", Protocol: api.ProtoTCP},
						},
						Rules: &api.L7Rules{
							Kafka: []api.PortRuleKafka{
								{Topic: "foo"},
							},
						},
					}},
				},
			},
		},
	}

	splashBrothersPolicy = ResolveIdentityPolicy(rules, identityCache, identitySplashBrothers)
	expectedPolicy = &cilium.NetworkPolicy{
		Policy: uint64(identitySplashBrothers),
		IngressPerPortPolicies: []*cilium.PortNetworkPolicy{
			{
				Port:     80,
				Protocol: core.SocketAddress_TCP,
				Rules: []*cilium.PortNetworkPolicyRule{
					{
						RemotePolicies: []uint64{uint64(identitySplashBrothers)},
						L7Rules: &cilium.PortNetworkPolicyRule_KafkaRules{
							KafkaRules: &cilium.KafkaNetworkPolicyRules{
								KafkaRules: []*cilium.KafkaNetworkPolicyRule{
									{
										ApiKey:     -1,
										ApiVersion: -1,
										Topic:      "",
										ClientId:   "",
									},
								},
							},
						},
					},
				},
			},
			{
				Port:     80,
				Protocol: core.SocketAddress_TCP,
				Rules: []*cilium.PortNetworkPolicyRule{
					{
						RemotePolicies: []uint64{uint64(identityWarriors)},
						L7Rules: &cilium.PortNetworkPolicyRule_KafkaRules{
							KafkaRules: &cilium.KafkaNetworkPolicyRules{
								KafkaRules: []*cilium.KafkaNetworkPolicyRule{
									{
										ApiKey:     -1,
										ApiVersion: -1,
										Topic:      "",
										ClientId:   "",
									},
								},
							},
						},
					},
				},
			},
			{
				Port:     80,
				Protocol: core.SocketAddress_TCP,
				Rules: []*cilium.PortNetworkPolicyRule{
					{
						L7Rules: &cilium.PortNetworkPolicyRule_HttpRules{
							HttpRules: &cilium.HttpNetworkPolicyRules{
								HttpRules: []*cilium.HttpNetworkPolicyRule{
									{
										Headers: []*envoy_api_v2_route.HeaderMatcher{
											{
												Name:  ":method",
												Value: "GET",
												Regex: &wrappers.BoolValue{Value: true},
											},
											{
												Name:  ":path",
												Value: "/",
												Regex: &wrappers.BoolValue{Value: true},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
	c.Assert(splashBrothersPolicy, comparator.DeepEquals, expectedPolicy)
	err = OptimizeNetworkPolicy(splashBrothersPolicy)
	c.Assert(err, Not(IsNil))

}

// Case 10: restrict same path / method on L7 in both rules,
// but select different endpoints in each rule.
func (ds *ResolverTestSuite) TestMergingWithDifferentEndpointsSelectedAllowSameL7(c *C) {
	identityCache := identity.IdentityCache{}

	identityCache[identity11] = identity11Labels
	identityCache[identity30] = identity30Labels
	identityCache[identity35] = identity35Labels

	rules := api.Rules{
		&api.Rule{
			EndpointSelector: endpointSelectorKlay,
			Ingress: []api.IngressRule{
				{
					FromEndpoints: []api.EndpointSelector{endpointSelectorDurant},
					ToPorts: []api.PortRule{{
						Ports: []api.PortProtocol{
							{Port: "80", Protocol: api.ProtoTCP},
						},
						Rules: &api.L7Rules{
							HTTP: []api.PortRuleHTTP{
								{Method: "GET", Path: "/"},
							},
						},
					}},
				},
				{
					FromEndpoints: []api.EndpointSelector{endpointSelectorSteph},
					ToPorts: []api.PortRule{{
						Ports: []api.PortProtocol{
							{Port: "80", Protocol: api.ProtoTCP},
						},
						Rules: &api.L7Rules{
							HTTP: []api.PortRuleHTTP{
								{Method: "GET", Path: "/"},
							},
						},
					}},
				},
			},
		},
	}

	klayPolicy := ResolveIdentityPolicy(rules, identityCache, identity11)
	expectedPolicy := &cilium.NetworkPolicy{
		Policy: uint64(identity11),
		IngressPerPortPolicies: []*cilium.PortNetworkPolicy{
			{
				Port:     80,
				Protocol: core.SocketAddress_TCP,
				Rules: []*cilium.PortNetworkPolicyRule{
					{
						RemotePolicies: []uint64{uint64(identity30)},
						L7Rules: &cilium.PortNetworkPolicyRule_HttpRules{
							HttpRules: &cilium.HttpNetworkPolicyRules{
								HttpRules: []*cilium.HttpNetworkPolicyRule{
									{
										Headers: []*envoy_api_v2_route.HeaderMatcher{
											{
												Name:  ":method",
												Value: "GET",
												Regex: &wrappers.BoolValue{Value: true},
											},
											{
												Name:  ":path",
												Value: "/",
												Regex: &wrappers.BoolValue{Value: true},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			{
				Port:     80,
				Protocol: core.SocketAddress_TCP,
				Rules: []*cilium.PortNetworkPolicyRule{
					{
						RemotePolicies: []uint64{uint64(identity35)},
						L7Rules: &cilium.PortNetworkPolicyRule_HttpRules{
							HttpRules: &cilium.HttpNetworkPolicyRules{
								HttpRules: []*cilium.HttpNetworkPolicyRule{
									{
										Headers: []*envoy_api_v2_route.HeaderMatcher{
											{
												Name:  ":method",
												Value: "GET",
												Regex: &wrappers.BoolValue{Value: true},
											},
											{
												Name:  ":path",
												Value: "/",
												Regex: &wrappers.BoolValue{Value: true},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
	c.Assert(klayPolicy, comparator.DeepEquals, expectedPolicy)

	err := OptimizeNetworkPolicy(klayPolicy)
	c.Assert(err, IsNil)
	expectedPolicy = &cilium.NetworkPolicy{
		Policy: uint64(identity11),
		IngressPerPortPolicies: []*cilium.PortNetworkPolicy{
			{
				Port:     80,
				Protocol: core.SocketAddress_TCP,
				Rules: []*cilium.PortNetworkPolicyRule{
					{
						RemotePolicies: []uint64{uint64(identity30)},
						L7Rules: &cilium.PortNetworkPolicyRule_HttpRules{
							HttpRules: &cilium.HttpNetworkPolicyRules{
								HttpRules: []*cilium.HttpNetworkPolicyRule{
									{
										Headers: []*envoy_api_v2_route.HeaderMatcher{
											{
												Name:  ":method",
												Value: "GET",
												Regex: &wrappers.BoolValue{Value: true},
											},
											{
												Name:  ":path",
												Value: "/",
												Regex: &wrappers.BoolValue{Value: true},
											},
										},
									},
								},
							},
						},
					},
					{
						RemotePolicies: []uint64{uint64(identity35)},
						L7Rules: &cilium.PortNetworkPolicyRule_HttpRules{
							HttpRules: &cilium.HttpNetworkPolicyRules{
								HttpRules: []*cilium.HttpNetworkPolicyRule{
									{
										Headers: []*envoy_api_v2_route.HeaderMatcher{
											{
												Name:  ":method",
												Value: "GET",
												Regex: &wrappers.BoolValue{Value: true},
											},
											{
												Name:  ":path",
												Value: "/",
												Regex: &wrappers.BoolValue{Value: true},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
	c.Assert(klayPolicy, comparator.DeepEquals, expectedPolicy)
}

// Case 11: allow all on L7 in both rules, but select different endpoints in each rule.
func (ds *ResolverTestSuite) TestMergingWithDifferentEndpointSelectedAllowAllL7(c *C) {
	identityCache := initIdentityCache()

	rules := api.Rules{
		&api.Rule{
			EndpointSelector: endpointSelectorSplashBros,
			Ingress: []api.IngressRule{
				{
					FromEndpoints: []api.EndpointSelector{endpointSelectorSteph},
					ToPorts: []api.PortRule{{
						Ports: []api.PortProtocol{
							{Port: "80", Protocol: api.ProtoTCP},
						},
					}},
				},
				{
					FromEndpoints: []api.EndpointSelector{endpointSelectorDurant},
					ToPorts: []api.PortRule{{
						Ports: []api.PortProtocol{
							{Port: "80", Protocol: api.ProtoTCP},
						},
					}},
				},
			},
		},
	}

	splashBrothersPolicy := ResolveIdentityPolicy(rules, identityCache, identitySplashBrothers)
	expectedPolicy := &cilium.NetworkPolicy{
		Policy: uint64(identitySplashBrothers),
		IngressPerPortPolicies: []*cilium.PortNetworkPolicy{
			{
				Port:     80,
				Protocol: core.SocketAddress_TCP,
				Rules: []*cilium.PortNetworkPolicyRule{
					{
						RemotePolicies: []uint64{uint64(identity30)},
					},
				},
			},
			{
				Port:     80,
				Protocol: core.SocketAddress_TCP,
				Rules: []*cilium.PortNetworkPolicyRule{
					{
						RemotePolicies: []uint64{uint64(identity35)},
					},
				},
			},
			{
				Port:     80,
				Protocol: core.SocketAddress_TCP,
				Rules: []*cilium.PortNetworkPolicyRule{
					{
						RemotePolicies: []uint64{uint64(identitySplashBrothers)},
					},
				},
			},
			{
				Port:     80,
				Protocol: core.SocketAddress_TCP,
				Rules: []*cilium.PortNetworkPolicyRule{
					{
						RemotePolicies: []uint64{uint64(identityWarriors)},
					},
				},
			},
			{
				Port:     80,
				Protocol: core.SocketAddress_TCP,
				Rules: []*cilium.PortNetworkPolicyRule{
					{
						RemotePolicies: []uint64{uint64(identityWarriors)},
					},
				},
			},
		},
	}
	c.Assert(splashBrothersPolicy, comparator.DeepEquals, expectedPolicy)
	err := OptimizeNetworkPolicy(splashBrothersPolicy)
	c.Assert(err, IsNil)
	expectedPolicy = &cilium.NetworkPolicy{
		Policy: uint64(identitySplashBrothers),
		IngressPerPortPolicies: []*cilium.PortNetworkPolicy{
			{
				Port:     80,
				Protocol: core.SocketAddress_TCP,
				Rules: []*cilium.PortNetworkPolicyRule{
					{
						RemotePolicies: []uint64{uint64(identity30)},
					},
					{
						RemotePolicies: []uint64{uint64(identity35)},
					},
					{
						RemotePolicies: []uint64{uint64(identitySplashBrothers)},
					},
					{
						RemotePolicies: []uint64{uint64(identityWarriors)},
					},
					{
						RemotePolicies: []uint64{uint64(identityWarriors)},
					},
				},
			},
		},
	}
	c.Assert(splashBrothersPolicy, comparator.DeepEquals, expectedPolicy)

}

func (ds *ResolverTestSuite) TestResolveIdentityPolicyL3L4(c *C) {
	identityCache := initIdentityCache()

	rules := api.Rules{
		&api.Rule{
			EndpointSelector: endpointSelectorSplashBros,
			Ingress: []api.IngressRule{
				{
					FromEndpoints: []api.EndpointSelector{
						api.NewESFromLabels(
							stephLabel,
							klayLabel,
						),
					},
					ToPorts: []api.PortRule{{
						Ports: []api.PortProtocol{
							{Port: "80", Protocol: api.ProtoTCP},
						},
					}},
				},
			},
		},
	}
	splashBrothersPolicy := ResolveIdentityPolicy(rules, identityCache, identitySplashBrothers)
	expectedPolicy := &cilium.NetworkPolicy{
		Policy: uint64(identitySplashBrothers),
		IngressPerPortPolicies: []*cilium.PortNetworkPolicy{
			{
				Port:     80,
				Protocol: core.SocketAddress_TCP,
				Rules: []*cilium.PortNetworkPolicyRule{
					{
						RemotePolicies: []uint64{uint64(identitySplashBrothers)},
					},
				},
			},
			{
				Port:     80,
				Protocol: core.SocketAddress_TCP,
				Rules: []*cilium.PortNetworkPolicyRule{
					{
						RemotePolicies: []uint64{uint64(identityWarriors)},
					},
				},
			},
		},
	}
	c.Assert(splashBrothersPolicy, comparator.DeepEquals, expectedPolicy)

	err := OptimizeNetworkPolicy(splashBrothersPolicy)
	c.Assert(err, IsNil)
	expectedPolicy = &cilium.NetworkPolicy{
		Policy: uint64(identitySplashBrothers),
		IngressPerPortPolicies: []*cilium.PortNetworkPolicy{
			{
				Port:     80,
				Protocol: core.SocketAddress_TCP,
				Rules: []*cilium.PortNetworkPolicyRule{
					{
						RemotePolicies: []uint64{uint64(identitySplashBrothers)},
					},
					{
						RemotePolicies: []uint64{uint64(identityWarriors)},
					},
				},
			},
		},
	}
	c.Assert(splashBrothersPolicy, comparator.DeepEquals, expectedPolicy)

	rules = api.Rules{
		&api.Rule{
			EndpointSelector: endpointSelectorSplashBros,
			Ingress: []api.IngressRule{
				{
					FromEndpoints: []api.EndpointSelector{
						api.NewESFromLabels(
							stephLabel,
							klayLabel,
						),
					},
					ToPorts: []api.PortRule{{
						Ports: []api.PortProtocol{
							{Port: "80", Protocol: api.ProtoTCP},
						},
					}},
				},
				{
					FromEndpoints: []api.EndpointSelector{
						api.NewESFromLabels(
							durantLabel,
						),
					},
					ToPorts: []api.PortRule{{
						Ports: []api.PortProtocol{
							{Port: "80", Protocol: api.ProtoTCP},
						},
					}},
				},
			},
		},
	}
	splashBrothersPolicy = ResolveIdentityPolicy(rules, identityCache, identitySplashBrothers)
	expectedPolicy = &cilium.NetworkPolicy{
		Policy: uint64(identitySplashBrothers),
		IngressPerPortPolicies: []*cilium.PortNetworkPolicy{
			{
				Port:     80,
				Protocol: core.SocketAddress_TCP,
				Rules: []*cilium.PortNetworkPolicyRule{
					{
						RemotePolicies: []uint64{uint64(identity35)},
					},
				},
			},
			{
				Port:     80,
				Protocol: core.SocketAddress_TCP,
				Rules: []*cilium.PortNetworkPolicyRule{
					{
						RemotePolicies: []uint64{uint64(identitySplashBrothers)},
					},
				},
			},
			{
				Port:     80,
				Protocol: core.SocketAddress_TCP,
				Rules: []*cilium.PortNetworkPolicyRule{
					{
						RemotePolicies: []uint64{uint64(identityWarriors)},
					},
				},
			},
			{
				// This duplicate rule appears twice because both rules match
				// the labels for identityWarriors.
				Port:     80,
				Protocol: core.SocketAddress_TCP,
				Rules: []*cilium.PortNetworkPolicyRule{
					{
						RemotePolicies: []uint64{uint64(identityWarriors)},
					},
				},
			},
		},
	}
	c.Assert(splashBrothersPolicy, comparator.DeepEquals, expectedPolicy)

	err = OptimizeNetworkPolicy(splashBrothersPolicy)
	c.Assert(err, IsNil)
	expectedPolicy = &cilium.NetworkPolicy{
		Policy: uint64(identitySplashBrothers),
		IngressPerPortPolicies: []*cilium.PortNetworkPolicy{
			{
				Port:     80,
				Protocol: core.SocketAddress_TCP,
				Rules: []*cilium.PortNetworkPolicyRule{
					{
						RemotePolicies: []uint64{uint64(identity35)},
					},
					{
						RemotePolicies: []uint64{uint64(identitySplashBrothers)},
					},
					{
						RemotePolicies: []uint64{uint64(identityWarriors)},
					},
					{
						RemotePolicies: []uint64{uint64(identityWarriors)},
					},
				},
			},
		},
	}
	c.Assert(splashBrothersPolicy, comparator.DeepEquals, expectedPolicy)

	rules = api.Rules{
		&api.Rule{
			EndpointSelector: endpointSelectorSplashBros,
			Ingress: []api.IngressRule{
				{
					FromEndpoints: []api.EndpointSelector{
						api.NewESFromLabels(
							stephLabel,
							klayLabel,
						),
					},
					ToPorts: []api.PortRule{{
						Ports: []api.PortProtocol{
							{Port: "80", Protocol: api.ProtoTCP},
						},
					}},
				},
				{
					FromEndpoints: []api.EndpointSelector{
						api.NewESFromLabels(
							durantLabel,
						),
					},
					ToPorts: []api.PortRule{{
						Ports: []api.PortProtocol{
							{Port: "80", Protocol: api.ProtoTCP},
						},
					}},
				},
			},
		},
	}
	splashBrothersPolicy = ResolveIdentityPolicy(rules, identityCache, identitySplashBrothers)
	expectedPolicy = &cilium.NetworkPolicy{
		Policy: uint64(identitySplashBrothers),
		IngressPerPortPolicies: []*cilium.PortNetworkPolicy{
			{
				Port:     80,
				Protocol: core.SocketAddress_TCP,
				Rules: []*cilium.PortNetworkPolicyRule{
					{
						RemotePolicies: []uint64{uint64(identity35)},
					},
				},
			},
			{
				Port:     80,
				Protocol: core.SocketAddress_TCP,
				Rules: []*cilium.PortNetworkPolicyRule{
					{
						RemotePolicies: []uint64{uint64(identitySplashBrothers)},
					},
				},
			},
			{
				Port:     80,
				Protocol: core.SocketAddress_TCP,
				Rules: []*cilium.PortNetworkPolicyRule{
					{
						RemotePolicies: []uint64{uint64(identityWarriors)},
					},
				},
			},
			{
				// This duplicate rule appears twice because both rules match
				// the labels for identityWarriors.
				Port:     80,
				Protocol: core.SocketAddress_TCP,
				Rules: []*cilium.PortNetworkPolicyRule{
					{
						RemotePolicies: []uint64{uint64(identityWarriors)},
					},
				},
			},
		},
	}
	c.Assert(splashBrothersPolicy, comparator.DeepEquals, expectedPolicy)

	rules = api.Rules{
		&api.Rule{
			EndpointSelector: endpointSelectorSplashBros,
			Ingress: []api.IngressRule{
				{
					FromEndpoints: []api.EndpointSelector{
						api.NewESFromLabels(
							stephLabel,
							klayLabel,
						),
					},
					ToPorts: []api.PortRule{{
						Ports: []api.PortProtocol{
							{Port: "80", Protocol: api.ProtoTCP},
						},
					}},
				},
				{
					FromEndpoints: []api.EndpointSelector{
						api.NewESFromLabels(
							durantLabel,
						),
					},
					ToPorts: []api.PortRule{{
						Ports: []api.PortProtocol{
							{Port: "81", Protocol: api.ProtoTCP},
						},
					}},
				},
			},
		},
	}
	splashBrothersPolicy = ResolveIdentityPolicy(rules, identityCache, identitySplashBrothers)
	expectedPolicy = &cilium.NetworkPolicy{
		Policy: uint64(identitySplashBrothers),
		IngressPerPortPolicies: []*cilium.PortNetworkPolicy{
			{
				Port:     80,
				Protocol: core.SocketAddress_TCP,
				Rules: []*cilium.PortNetworkPolicyRule{
					{
						RemotePolicies: []uint64{uint64(identitySplashBrothers)},
					},
				},
			},
			{
				Port:     80,
				Protocol: core.SocketAddress_TCP,
				Rules: []*cilium.PortNetworkPolicyRule{
					{
						RemotePolicies: []uint64{uint64(identityWarriors)},
					},
				},
			},
			{
				Port:     81,
				Protocol: core.SocketAddress_TCP,
				Rules: []*cilium.PortNetworkPolicyRule{
					{
						RemotePolicies: []uint64{uint64(identity35)},
					},
				},
			},
			{
				Port:     81,
				Protocol: core.SocketAddress_TCP,
				Rules: []*cilium.PortNetworkPolicyRule{
					{
						RemotePolicies: []uint64{uint64(identityWarriors)},
					},
				},
			},
		},
	}
	c.Assert(splashBrothersPolicy, comparator.DeepEquals, expectedPolicy)

	rules = api.Rules{
		&api.Rule{
			EndpointSelector: endpointSelectorSplashBros,
			Ingress: []api.IngressRule{
				{
					ToPorts: []api.PortRule{{
						Ports: []api.PortProtocol{
							{Port: "80", Protocol: api.ProtoTCP},
						},
					}},
				},
				{
					FromEndpoints: []api.EndpointSelector{
						api.NewESFromLabels(
							durantLabel,
						),
					},
					ToPorts: []api.PortRule{{
						Ports: []api.PortProtocol{
							{Port: "81", Protocol: api.ProtoTCP},
						},
					}},
				},
			},
		},
	}
	splashBrothersPolicy = ResolveIdentityPolicy(rules, identityCache, identitySplashBrothers)
	expectedPolicy = &cilium.NetworkPolicy{
		Policy: uint64(identitySplashBrothers),
		IngressPerPortPolicies: []*cilium.PortNetworkPolicy{
			{
				Port:     80,
				Protocol: core.SocketAddress_TCP,
				Rules:    []*cilium.PortNetworkPolicyRule{},
			},
			{
				Port:     81,
				Protocol: core.SocketAddress_TCP,
				Rules: []*cilium.PortNetworkPolicyRule{
					{
						RemotePolicies: []uint64{uint64(identity35)},
					},
				},
			},
			{
				Port:     81,
				Protocol: core.SocketAddress_TCP,
				Rules: []*cilium.PortNetworkPolicyRule{
					{
						RemotePolicies: []uint64{uint64(identityWarriors)},
					},
				},
			},
		},
	}
	c.Assert(splashBrothersPolicy, comparator.DeepEquals, expectedPolicy)

	rules = api.Rules{
		&api.Rule{
			EndpointSelector: endpointSelectorSplashBros,
			Ingress: []api.IngressRule{
				{
					FromEndpoints: []api.EndpointSelector{
						api.NewESFromLabels(
							stephLabel,
							klayLabel,
						),
					},
				},
				{
					FromEndpoints: []api.EndpointSelector{
						api.NewESFromLabels(
							durantLabel,
						),
					},
					ToPorts: []api.PortRule{{
						Ports: []api.PortProtocol{
							{Port: "81", Protocol: api.ProtoTCP},
						},
					}},
				},
			},
		},
	}
	splashBrothersPolicy = ResolveIdentityPolicy(rules, identityCache, identitySplashBrothers)
	expectedPolicy = &cilium.NetworkPolicy{
		Policy: uint64(identitySplashBrothers),
		IngressPerPortPolicies: []*cilium.PortNetworkPolicy{
			{
				Rules: []*cilium.PortNetworkPolicyRule{
					{
						RemotePolicies: []uint64{uint64(identitySplashBrothers)},
					},
				},
				ProtocolWildcard: true,
			},

			{
				Rules: []*cilium.PortNetworkPolicyRule{
					{
						RemotePolicies: []uint64{uint64(identityWarriors)},
					},
				},
				ProtocolWildcard: true,
			},
			{
				Port:     81,
				Protocol: core.SocketAddress_TCP,
				Rules: []*cilium.PortNetworkPolicyRule{
					{
						RemotePolicies: []uint64{uint64(identity35)},
					},
				},
			},
			{
				Port:     81,
				Protocol: core.SocketAddress_TCP,
				Rules: []*cilium.PortNetworkPolicyRule{
					{
						RemotePolicies: []uint64{uint64(identityWarriors)},
					},
				},
			},
		},
	}
	c.Assert(splashBrothersPolicy, comparator.DeepEquals, expectedPolicy)
}
func (ds *ResolverTestSuite) TestResolveIdentityPolicyL7(c *C) {
	identityCache := initIdentityCache()

	rules := api.Rules{
		&api.Rule{
			EndpointSelector: endpointSelectorSplashBros,
			Ingress: []api.IngressRule{
				{
					FromEndpoints: []api.EndpointSelector{
						api.NewESFromLabels(
							stephLabel,
							klayLabel,
						),
					},
					ToPorts: []api.PortRule{{
						Ports: []api.PortProtocol{
							{Port: "80", Protocol: api.ProtoTCP},
						},
						Rules: &api.L7Rules{
							HTTP: []api.PortRuleHTTP{
								{
									Method:  "GET",
									Path:    "/foo",
									Host:    "foo.cilium.io",
									Headers: []string{"header2 value", "header1"},
								},
							},
						},
					}},
				},
			},
		},
	}
	splashBrothersPolicy := ResolveIdentityPolicy(rules, identityCache, identitySplashBrothers)
	expectedPolicy := &cilium.NetworkPolicy{
		Policy: uint64(identitySplashBrothers),
		IngressPerPortPolicies: []*cilium.PortNetworkPolicy{
			{
				Port:     80,
				Protocol: core.SocketAddress_TCP,
				Rules: []*cilium.PortNetworkPolicyRule{
					{
						RemotePolicies: []uint64{uint64(identitySplashBrothers)},
						L7Rules: &cilium.PortNetworkPolicyRule_HttpRules{
							HttpRules: &cilium.HttpNetworkPolicyRules{
								HttpRules: []*cilium.HttpNetworkPolicyRule{
									{Headers: []*envoy_api_v2_route.HeaderMatcher{
										{
											Name:  ":authority",
											Value: "foo.cilium.io",
											Regex: &wrappers.BoolValue{Value: true},
										},
										{
											Name:  ":method",
											Value: "GET",
											Regex: &wrappers.BoolValue{Value: true},
										},
										{
											Name:  ":path",
											Value: "/foo",
											Regex: &wrappers.BoolValue{Value: true},
										},
										{
											Name: "header1",
										},
										{
											Name:  "header2",
											Value: "value",
										},
									}},
								},
							},
						},
					},
				},
			},
			{
				Port:     80,
				Protocol: core.SocketAddress_TCP,
				Rules: []*cilium.PortNetworkPolicyRule{
					{
						RemotePolicies: []uint64{uint64(identityWarriors)},
						L7Rules: &cilium.PortNetworkPolicyRule_HttpRules{
							HttpRules: &cilium.HttpNetworkPolicyRules{
								HttpRules: []*cilium.HttpNetworkPolicyRule{
									{Headers: []*envoy_api_v2_route.HeaderMatcher{
										{
											Name:  ":authority",
											Value: "foo.cilium.io",
											Regex: &wrappers.BoolValue{Value: true},
										},
										{
											Name:  ":method",
											Value: "GET",
											Regex: &wrappers.BoolValue{Value: true},
										},
										{
											Name:  ":path",
											Value: "/foo",
											Regex: &wrappers.BoolValue{Value: true},
										},
										{
											Name: "header1",
										},
										{
											Name:  "header2",
											Value: "value",
										},
									}},
								},
							},
						},
					},
				},
			},
		},
	}
	c.Assert(splashBrothersPolicy, comparator.DeepEquals, expectedPolicy)

	rules = api.Rules{
		&api.Rule{
			EndpointSelector: endpointSelectorSplashBros,
			Ingress: []api.IngressRule{
				{
					FromEndpoints: []api.EndpointSelector{
						api.NewESFromLabels(
							stephLabel,
							klayLabel,
						),
					},
					ToPorts: []api.PortRule{{
						Ports: []api.PortProtocol{
							{Port: "80", Protocol: api.ProtoTCP},
						},
						Rules: &api.L7Rules{
							HTTP: []api.PortRuleHTTP{
								{
									Path:    "/foo",
									Method:  "GET",
									Host:    "foo.cilium.io",
									Headers: []string{"header2 value", "header1"},
								},
								{
									Path:   "/bar",
									Method: "PUT",
								},
							},
						},
					}},
				},
			},
		},
	}
	splashBrothersPolicy = ResolveIdentityPolicy(rules, identityCache, identitySplashBrothers)
	expectedPolicy = &cilium.NetworkPolicy{
		Policy: uint64(identitySplashBrothers),
		IngressPerPortPolicies: []*cilium.PortNetworkPolicy{
			{
				Port:     80,
				Protocol: core.SocketAddress_TCP,
				Rules: []*cilium.PortNetworkPolicyRule{
					{
						RemotePolicies: []uint64{uint64(identitySplashBrothers)},
						L7Rules: &cilium.PortNetworkPolicyRule_HttpRules{
							HttpRules: &cilium.HttpNetworkPolicyRules{
								HttpRules: []*cilium.HttpNetworkPolicyRule{
									{
										Headers: []*envoy_api_v2_route.HeaderMatcher{
											{
												Name:  ":method",
												Value: "PUT",
												Regex: &wrappers.BoolValue{Value: true},
											},
											{
												Name:  ":path",
												Value: "/bar",
												Regex: &wrappers.BoolValue{Value: true},
											},
										},
									},
									{
										Headers: []*envoy_api_v2_route.HeaderMatcher{
											{
												Name:  ":authority",
												Value: "foo.cilium.io",
												Regex: &wrappers.BoolValue{Value: true},
											},
											{
												Name:  ":method",
												Value: "GET",
												Regex: &wrappers.BoolValue{Value: true},
											},
											{
												Name:  ":path",
												Value: "/foo",
												Regex: &wrappers.BoolValue{Value: true},
											},
											{
												Name: "header1",
											},
											{
												Name:  "header2",
												Value: "value",
											},
										},
									},
								},
							},
						},
					},
				},
			},
			{
				Port:     80,
				Protocol: core.SocketAddress_TCP,
				Rules: []*cilium.PortNetworkPolicyRule{
					{
						RemotePolicies: []uint64{uint64(identityWarriors)},
						L7Rules: &cilium.PortNetworkPolicyRule_HttpRules{
							HttpRules: &cilium.HttpNetworkPolicyRules{
								HttpRules: []*cilium.HttpNetworkPolicyRule{
									{
										Headers: []*envoy_api_v2_route.HeaderMatcher{
											{
												Name:  ":method",
												Value: "PUT",
												Regex: &wrappers.BoolValue{Value: true},
											},
											{
												Name:  ":path",
												Value: "/bar",
												Regex: &wrappers.BoolValue{Value: true},
											},
										},
									},
									{
										Headers: []*envoy_api_v2_route.HeaderMatcher{
											{
												Name:  ":authority",
												Value: "foo.cilium.io",
												Regex: &wrappers.BoolValue{Value: true},
											},
											{
												Name:  ":method",
												Value: "GET",
												Regex: &wrappers.BoolValue{Value: true},
											},
											{
												Name:  ":path",
												Value: "/foo",
												Regex: &wrappers.BoolValue{Value: true},
											},
											{
												Name: "header1",
											},
											{
												Name:  "header2",
												Value: "value",
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
	c.Assert(splashBrothersPolicy, comparator.DeepEquals, expectedPolicy)
}

func (ds *ResolverTestSuite) TestDaemonPolicyTest(c *C) {

	identityCache := identity.IdentityCache{}

	lblProd := labels.ParseLabel("Prod")
	lblQA := labels.ParseLabel("QA")
	lblFoo := labels.ParseLabel("foo")
	lblBar := labels.ParseLabel("bar")
	lblJoe := labels.ParseLabel("user=joe")
	lblPete := labels.ParseLabel("user=pete")

	identityQABar := identity.NumericIdentity(25)
	identityProdBar := identity.NumericIdentity(26)
	identityQAFoo := identity.NumericIdentity(27)
	identityProdFoo := identity.NumericIdentity(28)
	identityProdFooJoe := identity.NumericIdentity(29)

	qaBarLbls := labels.LabelArray{
		lblQA,
		lblBar,
	}

	prodBarLbls := labels.LabelArray{
		lblBar,
		lblProd,
	}

	qaFooLbls := labels.LabelArray{
		lblFoo,
		lblQA,
	}

	prodFooLbls := labels.LabelArray{
		lblProd,
		lblFoo,
	}

	prodFooJoeLbls := labels.LabelArray{
		lblFoo,
		lblProd,
		lblJoe,
	}

	identityCache[identityQABar] = qaBarLbls
	identityCache[identityProdBar] = prodBarLbls
	identityCache[identityQAFoo] = qaFooLbls
	identityCache[identityProdFoo] = prodFooLbls
	identityCache[identityProdFooJoe] = prodFooJoeLbls

	rules := api.Rules{
		{
			EndpointSelector: api.NewESFromLabels(lblBar),
			Ingress: []api.IngressRule{
				{
					FromEndpoints: []api.EndpointSelector{
						api.NewESFromLabels(lblJoe),
						api.NewESFromLabels(lblPete),
						api.NewESFromLabels(lblFoo),
					},
				},
				{
					FromEndpoints: []api.EndpointSelector{
						api.NewESFromLabels(lblFoo),
					},
					ToPorts: []api.PortRule{
						{
							Ports: []api.PortProtocol{
								{Port: "80", Protocol: api.ProtoTCP},
							},
							Rules: &api.L7Rules{
								HTTP: []api.PortRuleHTTP{
									{
										Path:   "/bar",
										Method: "GET",
									},
								},
							},
						},
					},
				},
			},
		},
		{
			EndpointSelector: api.NewESFromLabels(lblQA),
			Ingress: []api.IngressRule{
				{
					FromRequires: []api.EndpointSelector{
						api.NewESFromLabels(lblQA),
					},
				},
			},
		},
		{
			EndpointSelector: api.NewESFromLabels(lblProd),
			Ingress: []api.IngressRule{
				{
					FromRequires: []api.EndpointSelector{
						api.NewESFromLabels(lblProd),
					},
				},
			},
		},
	}

	identityPolicyQaBar := ResolveIdentityPolicy(rules, identityCache, identityQABar)
	err := OptimizeNetworkPolicy(identityPolicyQaBar)
	c.Assert(err, IsNil)
	//fmt.Printf("identityPolicyQaBar: %s\n", identityPolicyQaBar)

	expectedRemotePolicies := []uint64{
		uint64(identityQAFoo),
		// The prodFoo* identities are allowed by FromEndpoints but rejected by
		// FromRequires, so they are not included in the remote policies:
		// uint64(prodFooSecLblsCtx.ID),
		// uint64(prodFooJoeSecLblsCtx.ID),
	}
	sortkeys.Uint64s(expectedRemotePolicies)
	expectedNetworkPolicy := &cilium.NetworkPolicy{
		Name:   "",
		Policy: uint64(identityQABar),
		IngressPerPortPolicies: []*cilium.PortNetworkPolicy{
			{
				Rules: []*cilium.PortNetworkPolicyRule{
					{
						RemotePolicies: expectedRemotePolicies,
					},
				},
				ProtocolWildcard: true,
			},
			{
				Port:     80,
				Protocol: core.SocketAddress_TCP,
				Rules: []*cilium.PortNetworkPolicyRule{
					{
						RemotePolicies: expectedRemotePolicies,
						L7Rules: &cilium.PortNetworkPolicyRule_HttpRules{
							HttpRules: &cilium.HttpNetworkPolicyRules{
								HttpRules: []*cilium.HttpNetworkPolicyRule{
									{},
								},
							},
						},
					},
				},
			},
		},
	}
	c.Assert(identityPolicyQaBar, comparator.DeepEquals, expectedNetworkPolicy)
}
