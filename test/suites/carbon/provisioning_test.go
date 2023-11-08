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

package carbon_test

import (
	"fmt"
	"path"
	"path/filepath"
	"strconv"
	"time"

	"github.com/samber/lo"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/labels"
	"knative.dev/pkg/ptr"

	"github.com/aws/karpenter-core/pkg/apis/v1alpha5"
	"github.com/aws/karpenter-core/pkg/test"
	"github.com/aws/karpenter/pkg/apis/settings"
	"github.com/aws/karpenter/pkg/apis/v1alpha1"
	awstest "github.com/aws/karpenter/pkg/test"
	"github.com/aws/karpenter/test/pkg/debug"

	. "github.com/onsi/ginkgo/v2"
	//. "github.com/onsi/gomega"
)

var _ = Describe("Provisioning", Label(debug.NoWatch), Label(debug.NoEvents), func() {
	var provisioner *v1alpha5.Provisioner
	var nodeTemplate *v1alpha1.AWSNodeTemplate
	var deployment *appsv1.Deployment
	var selector labels.Selector
	var experimentDirectory string
	var timenow string = time.Now().Format("2006-01-02-15-04")
	//var dsCount int

	BeforeEach(func() {
		nodeTemplate = awstest.AWSNodeTemplate(v1alpha1.AWSNodeTemplateSpec{AWS: v1alpha1.AWS{
			SecurityGroupSelector: map[string]string{"karpenter.sh/discovery": settings.FromContext(env.Context).ClusterName},
			SubnetSelector:        map[string]string{"karpenter.sh/discovery": settings.FromContext(env.Context).ClusterName},
		}})
		provisioner = test.Provisioner(test.ProvisionerOptions{
			ProviderRef: &v1alpha5.MachineTemplateRef{
				Name: nodeTemplate.Name,
			},
			Kubelet: &v1alpha5.KubeletConfiguration{
				PodsPerCore: ptr.Int32(30),
			},
			Requirements: env.GetProvisionerRequirements(),
			// No limits!!!
			// https://tenor.com/view/chaos-gif-22919457
			Limits: v1.ResourceList{},
		})
		deployment = test.Deployment(test.DeploymentOptions{
			PodOptions: test.PodOptions{
				ResourceRequirements: v1.ResourceRequirements{
					Requests: v1.ResourceList{
						v1.ResourceCPU:    resource.MustParse("10m"),
						v1.ResourceMemory: resource.MustParse("50Mi"),
					},
				},
				TerminationGracePeriodSeconds: lo.ToPtr[int64](0),
			},
		})
		selector = labels.SelectorFromSet(deployment.Spec.Selector.MatchLabels)
		experimentDirectory = filepath.Join("experiments", timenow, "eu-north-1", "Provisioning")
	})

	DescribeTable("homogeneous pods",
		func(carbonAwareEnabled bool, replicaCount int, cpuRequest string, memoryRequest string) {
			replicas := replicaCount
			deployment = test.Deployment(test.DeploymentOptions{
				PodOptions: test.PodOptions{
					ResourceRequirements: v1.ResourceRequirements{
						Requests: v1.ResourceList{
							v1.ResourceCPU:    resource.MustParse(cpuRequest),
							v1.ResourceMemory: resource.MustParse(memoryRequest),
						},
					},
					TerminationGracePeriodSeconds: lo.ToPtr[int64](0),
				},
				Replicas: int32(replicas),
			})
			selector = labels.SelectorFromSet(deployment.Spec.Selector.MatchLabels)

			env.ExpectPrefixDelegationDisabled()

			By(fmt.Sprintf("setting carbonAwareEnabled to %s", strconv.FormatBool(carbonAwareEnabled)))
			env.ExpectSettingsOverridden(map[string]string{
				"featureGates.carbonAwareEnabled": strconv.FormatBool(carbonAwareEnabled),
			})

			By("waiting for the deployment to deploy all of its pods")
			env.ExpectCreated(deployment)
			env.EventuallyExpectPendingPodCount(selector, replicas)

			By("kicking off provisioning by applying the provisioner and nodeTemplate")
			env.ExpectCreated(provisioner, nodeTemplate)
			env.EventuallyExpectHealthyPodCount(selector, replicas)

			experimentDirectory = filepath.Join(
				experimentDirectory,
				"homogeneous-pods",
				fmt.Sprintf("%d-replicas", replicas),
				fmt.Sprintf("cpu-%s", cpuRequest),
				fmt.Sprintf("memory-%s", memoryRequest),
				fmt.Sprintf("carbonAware-%t", carbonAwareEnabled),
			)
			env.SaveTopology(experimentDirectory, "nodes.json")
		},
		EntryDescription("CarbonAwareEnabled=%t, replicas=%d, CPU=%s, memory=%s"),

		PEntry(nil, true, 2, "50m", "50Mi"),
		PEntry(nil, false, 2, "50m", "50Mi"),

		PEntry(nil, true, 7, "150m", "50Mi"),
		PEntry(nil, false, 7, "150m", "50Mi"),

		PEntry(nil, true, 3, "675m", "100Mi"),
		PEntry(nil, false, 3, "675m", "100Mi"),

		PEntry(nil, false, 12, "120m", "300Mi"),
		PEntry(nil, true, 12, "120m", "300Mi"),
	)

	PDescribeTable("hetrogeneous pods",
		func(carbonAwareEnabled bool, fileName string) {
			pods, selector := env.ImportPodTopologyTestInput(path.Join("experiments", "testInput"), fileName+".json")

			By(fmt.Sprintf("setting carbonAwareEnabled to %s", strconv.FormatBool(carbonAwareEnabled)))
			env.ExpectSettingsOverridden(map[string]string{
				"featureGates.carbonAwareEnabled": strconv.FormatBool(carbonAwareEnabled),
			})

			By("waiting for pods to be deployed")
			for _, pod := range pods {
				env.ExpectCreated(pod)
			}
			env.EventuallyExpectPendingPodCount(selector, len(pods))

			By("kicking off provisioning by applying the provisioner and nodeTemplate")
			env.ExpectCreated(provisioner, nodeTemplate)
			env.EventuallyExpectNodeCount(">", 0)
			env.EventuallyExpectPendingPodCount(selector, 0)

			experimentDirectory = filepath.Join(
				experimentDirectory,
				"hetrogeneous-pods",
				fmt.Sprintf("file-%s", fileName),
				fmt.Sprintf("carbonAware-%t", carbonAwareEnabled),
			)
			env.SaveTopology(experimentDirectory, "nodes.json")

		},
		EntryDescription("CarbonAwareEnabled=%t, podTopologyInputFile=%s.json"),
		// Entry(nil, true, "observed-pod-topology1"),
		// Entry(nil, false, "observed-pod-topology1"),
		Entry(nil, true, "observed-pod-topology2"),
		// Entry(nil, false, "observed-pod-topology2"),
	)

	// TODO @JacobValdemar: Problem is that every time it is always only ONE node which is provisioned. Never two small or a combination hereof
	// TODO @JacobValdemar: Consolidation
	// TODO @JacobValdemar: Create script that exports real cluster's pod+node topology, which can be used as input to test to see potential improvements
	//				          original cluster -> Karpenter -> Carbon Aware Karpenter
})
