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
		experimentDirectory = filepath.Join("experiments", timenow, "us-east-1", "Provisioning")
	})

	PDescribeTable("homogeneous pods",
		func(testString1 string, testString2 string, carbonEfficient bool, replicaCount int, cpuRequest string, memoryRequest string) {
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

			By(fmt.Sprintf("setting carbonEfficient to %s", strconv.FormatBool(carbonEfficient)))
			env.ExpectSettingsOverridden(map[string]string{
				"featureGates.carbonEfficient": strconv.FormatBool(carbonEfficient),
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
				testString1,
				testString2,
			)
			env.SaveTopology(experimentDirectory, "nodes.json")
		},
		//EntryDescription("CarbonEfficient=%t, replicas=%d, CPU=%s, memory=%s"),
		EntryDescription("%s, %s"),

		// Randomized
		// Entry(nil, "A1", "CarbonEfficient", true, 13, "292m", "121Mi"),
		// Entry(nil, "A1", "Original", false, 13, "292m", "121Mi"),
		// Entry(nil, "A2", "CarbonEfficient", true, 13, "220m", "17Mi"),
		// Entry(nil, "A2", "Original", false, 13, "220m", "17Mi"),
		// Entry(nil, "A3", "CarbonEfficient", true, 16, "179m", "182Mi"),
		// Entry(nil, "A3", "Original", false, 16, "179m", "182Mi"),
		// Entry(nil, "A4", "CarbonEfficient", true, 20, "258m", "235Mi"),
		// Entry(nil, "A4", "Original", false, 20, "258m", "235Mi"),
		// Entry(nil, "A5", "CarbonEfficient", true, 8, "37m", "71Mi"),
		// Entry(nil, "A5", "Original", false, 8, "37m", "71Mi"),
		// Entry(nil, "A6", "CarbonEfficient", true, 17, "69m", "107Mi"),
		// Entry(nil, "A6", "Original", false, 17, "69m", "107Mi"),
		// Entry(nil, "A7", "CarbonEfficient", true, 20, "75m", "190Mi"),
		// Entry(nil, "A7", "Original", false, 20, "75m", "190Mi"),
		// Entry(nil, "A8", "CarbonEfficient", true, 17, "142m", "216Mi"),
		// Entry(nil, "A8", "Original", false, 17, "142m", "216Mi"),
		// Entry(nil, "A9", "CarbonEfficient", true, 14, "156m", "95Mi"),
		// Entry(nil, "A9", "Original", false, 14, "156m", "95Mi"),
		// Entry(nil, "A10", "CarbonEfficient", true, 11, "150m", "64Mi"),
		// Entry(nil, "A10", "Original", false, 11, "150m", "64Mi"),

		PEntry(nil, "A1", "CarbonEfficient", true, 3, "485m", "1625Mi"),
		Entry(nil, "A1", "Original", false, 3, "485m", "1625Mi"),
		PEntry(nil, "A2", "CarbonEfficient", true, 10, "718m", "100Mi"),
		Entry(nil, "A2", "Original", false, 10, "718m", "100Mi"),
		PEntry(nil, "A3", "CarbonEfficient", true, 10, "2110m", "2092Mi"),
		Entry(nil, "A3", "Original", false, 10, "2110m", "2092Mi"),
		PEntry(nil, "A4", "CarbonEfficient", true, 12, "1133m", "1564Mi"),
		Entry(nil, "A4", "Original", false, 12, "1133m", "1564Mi"),
		PEntry(nil, "A5", "CarbonEfficient", true, 17, "2922m", "186Mi"),
		Entry(nil, "A5", "Original", false, 17, "2922m", "186Mi"),
		PEntry(nil, "A6", "CarbonEfficient", true, 15, "2918m", "2672Mi"),
		Entry(nil, "A6", "Original", false, 15, "2918m", "2672Mi"),
		PEntry(nil, "A7", "CarbonEfficient", true, 18, "1935m", "997Mi"),
		Entry(nil, "A7", "Original", false, 18, "1935m", "997Mi"),
		PEntry(nil, "A8", "CarbonEfficient", true, 2, "2582m", "697Mi"),
		Entry(nil, "A8", "Original", false, 2, "2582m", "697Mi"),
		PEntry(nil, "A9", "CarbonEfficient", true, 5, "1212m", "350Mi"),
		Entry(nil, "A9", "Original", false, 5, "1212m", "350Mi"),
		PEntry(nil, "A10", "CarbonEfficient", true, 10, "1900m", "939Mi"),
		Entry(nil, "A10", "Original", false, 10, "1900m", "939Mi"),
		PEntry(nil, "A11", "CarbonEfficient", true, 20, "2956m", "693Mi"),
		Entry(nil, "A11", "Original", false, 20, "2956m", "693Mi"),
		PEntry(nil, "A12", "CarbonEfficient", true, 16, "1683m", "1960Mi"),
		Entry(nil, "A12", "Original", false, 16, "1683m", "1960Mi"),
		PEntry(nil, "A13", "CarbonEfficient", true, 10, "2802m", "207Mi"),
		Entry(nil, "A13", "Original", false, 10, "2802m", "207Mi"),
		PEntry(nil, "A14", "CarbonEfficient", true, 7, "2164m", "833Mi"),
		Entry(nil, "A14", "Original", false, 7, "2164m", "833Mi"),
		PEntry(nil, "A15", "CarbonEfficient", true, 2, "1457m", "852Mi"),
		Entry(nil, "A15", "Original", false, 2, "1457m", "852Mi"),
		PEntry(nil, "A16", "CarbonEfficient", true, 15, "1921m", "2642Mi"),
		Entry(nil, "A16", "Original", false, 15, "1921m", "2642Mi"),
		PEntry(nil, "A17", "CarbonEfficient", true, 11, "2664m", "1338Mi"),
		Entry(nil, "A17", "Original", false, 11, "2664m", "1338Mi"),
		PEntry(nil, "A18", "CarbonEfficient", true, 4, "604m", "2270Mi"),
		Entry(nil, "A18", "Original", false, 4, "604m", "2270Mi"),
		PEntry(nil, "A19", "CarbonEfficient", true, 9, "1192m", "1814Mi"),
		Entry(nil, "A19", "Original", false, 9, "1192m", "1814Mi"),
		PEntry(nil, "A20", "CarbonEfficient", true, 4, "2977m", "2352Mi"),
		Entry(nil, "A20", "Original", false, 4, "2977m", "2352Mi"),
		PEntry(nil, "A21", "CarbonEfficient", true, 16, "1213m", "350Mi"),
		Entry(nil, "A21", "Original", false, 16, "1213m", "350Mi"),
		PEntry(nil, "A22", "CarbonEfficient", true, 8, "1980m", "2936Mi"),
		Entry(nil, "A22", "Original", false, 8, "1980m", "2936Mi"),
		PEntry(nil, "A23", "CarbonEfficient", true, 19, "2705m", "2548Mi"),
		Entry(nil, "A23", "Original", false, 19, "2705m", "2548Mi"),
		PEntry(nil, "A24", "CarbonEfficient", true, 1, "2987m", "161Mi"),
		Entry(nil, "A24", "Original", false, 1, "2987m", "161Mi"),
		PEntry(nil, "A25", "CarbonEfficient", true, 17, "1963m", "1404Mi"),
		Entry(nil, "A25", "Original", false, 17, "1963m", "1404Mi"),
		PEntry(nil, "A26", "CarbonEfficient", true, 13, "334m", "984Mi"),
		Entry(nil, "A26", "Original", false, 13, "334m", "984Mi"),
		PEntry(nil, "A27", "CarbonEfficient", true, 11, "118m", "1894Mi"),
		Entry(nil, "A27", "Original", false, 11, "118m", "1894Mi"),
		PEntry(nil, "A28", "CarbonEfficient", true, 14, "1858m", "698Mi"),
		Entry(nil, "A28", "Original", false, 14, "1858m", "698Mi"),
		PEntry(nil, "A29", "CarbonEfficient", true, 15, "1706m", "1744Mi"),
		Entry(nil, "A29", "Original", false, 15, "1706m", "1744Mi"),
		PEntry(nil, "A30", "CarbonEfficient", true, 2, "2887m", "1814Mi"),
		Entry(nil, "A30", "Original", false, 2, "2887m", "1814Mi"),
		PEntry(nil, "A31", "CarbonEfficient", true, 18, "2241m", "1804Mi"),
		Entry(nil, "A31", "Original", false, 18, "2241m", "1804Mi"),
		PEntry(nil, "A32", "CarbonEfficient", true, 1, "1991m", "1351Mi"),
		Entry(nil, "A32", "Original", false, 1, "1991m", "1351Mi"),
		PEntry(nil, "A33", "CarbonEfficient", true, 6, "1575m", "115Mi"),
		Entry(nil, "A33", "Original", false, 6, "1575m", "115Mi"),
		PEntry(nil, "A34", "CarbonEfficient", true, 4, "787m", "1546Mi"),
		Entry(nil, "A34", "Original", false, 4, "787m", "1546Mi"),
		PEntry(nil, "A35", "CarbonEfficient", true, 19, "2887m", "1229Mi"),
		Entry(nil, "A35", "Original", false, 19, "2887m", "1229Mi"),
	)

	// PDescribeTable("hetrogeneous pods",
	// 	func(carbonEfficient bool, fileName string) {
	// 		pods, selector := env.ImportPodTopologyTestInput(path.Join("experiments", "testInput"), fileName+".json")

	// 		By(fmt.Sprintf("setting carbonEfficient to %s", strconv.FormatBool(carbonEfficient)))
	// 		env.ExpectSettingsOverridden(map[string]string{
	// 			"featureGates.carbonEfficient": strconv.FormatBool(carbonEfficient),
	// 		})

	// 		By("waiting for pods to be deployed")
	// 		for _, pod := range pods {
	// 			env.ExpectCreated(pod)
	// 		}
	// 		env.EventuallyExpectPendingPodCount(selector, len(pods))

	// 		By("kicking off provisioning by applying the provisioner and nodeTemplate")
	// 		env.ExpectCreated(provisioner, nodeTemplate)
	// 		env.EventuallyExpectNodeCount(">", 0)
	// 		env.EventuallyExpectPendingPodCount(selector, 0)

	// 		experimentDirectory = filepath.Join(
	// 			experimentDirectory,
	// 			"hetrogeneous-pods",
	// 			fmt.Sprintf("file-%s", fileName),
	// 			fmt.Sprintf("carbonEfficient-%t", carbonEfficient),
	// 		)
	// 		env.SaveTopology(experimentDirectory, "nodes.json")

	// 	},
	// 	EntryDescription("carbonEfficient=%t, podTopologyInputFile=%s.json"),
	// 	// Entry(nil, true, "observed-pod-topology1"),
	// 	// Entry(nil, false, "observed-pod-topology1"),
	// 	Entry(nil, true, "observed-pod-topology2"),
	// 	// Entry(nil, false, "observed-pod-topology2"),
	// )

	// TODO @JacobValdemar: Problem is that every time it is always only ONE node which is provisioned. Never two small or a combination hereof
	// TODO @JacobValdemar: Consolidation
	// TODO @JacobValdemar: Create script that exports real cluster's pod+node topology, which can be used as input to test to see potential improvements
	//				          original cluster -> Karpenter -> Carbon Efficient Karpenter
})
