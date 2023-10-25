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

	. "github.com/onsi/ginkgo/v2"
	"github.com/samber/lo"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"

	"github.com/aws/karpenter-core/pkg/apis/v1alpha5"
	"github.com/aws/karpenter-core/pkg/test"
	"github.com/aws/karpenter/pkg/apis/settings"
	"github.com/aws/karpenter/pkg/apis/v1alpha1"
	awstest "github.com/aws/karpenter/pkg/test"
	"github.com/aws/karpenter/test/pkg/debug"
)

const testGroup = "carbon"

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
			Requirements: []v1.NodeSelectorRequirement{
				{
					Key:      v1alpha5.LabelCapacityType,
					Operator: v1.NodeSelectorOpIn,
					Values:   []string{v1alpha1.CapacityTypeOnDemand},
				},
				{
					Key:      v1.LabelOSStable,
					Operator: v1.NodeSelectorOpIn,
					Values:   []string{string(v1.Linux)},
				},
				// {
				// 	Key:      "karpenter.k8s.aws/instance-hypervisor",
				// 	Operator: v1.NodeSelectorOpIn,
				// 	Values:   []string{"nitro"},
				// },
			},
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
		// Get the DS pod count and use it to calculate the DS pod overhead
		//dsCount = env.GetDaemonSetCount(provisioner)

		experimentDirectory = filepath.Join("experiments", timenow, "Provisioning")

	})

	DescribeTable("homogeneous pods",
		func(enabled bool, replicaCount int, cpuRequest string, memoryRequest string) {
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

			By(fmt.Sprintf("setting carbonAwareEnabled to %s", strconv.FormatBool(enabled)))
			env.ExpectSettingsOverridden(map[string]string{
				"featureGates.carbonAwareEnabled": strconv.FormatBool(enabled),
			})

			By("waiting for the deployment to deploy all of its pods")
			env.ExpectCreated(deployment)
			env.EventuallyExpectPendingPodCount(selector, replicas)

			By("kicking off provisioning by applying the provisioner and nodeTemplate")
			env.ExpectCreated(provisioner, nodeTemplate)
			env.EventuallyExpectHealthyPodCount(selector, replicas)

			By("saving topology")
			var carbonAwareStatus string
			if enabled {
				carbonAwareStatus = "enabled"
			} else {
				carbonAwareStatus = "disabled"
			}
			experimentDirectory = filepath.Join(
				experimentDirectory,
				"homogeneous-pods",
				fmt.Sprintf("%d-replicas", replicas),
				fmt.Sprintf("cpu-%s", cpuRequest),
				fmt.Sprintf("memory-%s", memoryRequest),
				fmt.Sprintf("carbonAware-%s", carbonAwareStatus),
			)
			env.SaveTopology(experimentDirectory, "nodes.json")
		},
		EntryDescription("CarbonAwareEnabled=%t, replicas=%d, CPU=%s, memory=%s"),
		//Entry(nil, true, 2, "10m", "50Mi"),
		//Entry(nil, false, 2, "10m", "50Mi"),
		//Entry(nil, true, 7, "150m", "50Mi"),
		//Entry(nil, false, 7, "150m", "50Mi"),
		// Entry(nil, true, 3, "350m", "50Mi"),
		// Entry(nil, false, 3, "350m", "50Mi"),
		// Entry(nil, true, 3, "675m", "100Mi"),
		// Entry(nil, false, 3, "675m", "100Mi"),
	)

	DescribeTable("hetrogeneous pods",
		func(enabled bool, fileName string) {
			var pods []*v1.Pod

			By(fmt.Sprintf("loading pod topology from %s.json", fileName))
			inputPods := env.ImportPodTopologyTestInput(path.Join("experiments", "testInput"), fileName+".json")
			for _, inputPod := range inputPods {
				var requests v1.ResourceList
				if inputPod.MemoryRequest == "" && inputPod.CPURequest == "" {
					requests = v1.ResourceList{}
				} else if inputPod.MemoryRequest != "" && inputPod.CPURequest == "" {
					requests = v1.ResourceList{
						v1.ResourceMemory: resource.MustParse(inputPod.MemoryRequest),
					}
				} else if inputPod.MemoryRequest == "" && inputPod.CPURequest != "" {
					requests = v1.ResourceList{
						v1.ResourceCPU: resource.MustParse(inputPod.CPURequest),
					}
				} else {
					requests = v1.ResourceList{
						v1.ResourceCPU:    resource.MustParse(inputPod.CPURequest),
						v1.ResourceMemory: resource.MustParse(inputPod.MemoryRequest),
					}
				}

				label := map[string]string{"testing/pod-app": "loaded"}
				selector = labels.SelectorFromSet(label)
				pods = append(pods, test.Pod(test.PodOptions{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							v1alpha5.DoNotEvictPodAnnotationKey: "true",
						},
						Labels: label,
					},
					ResourceRequirements: v1.ResourceRequirements{
						Requests: requests,
					},
				}))
			}

			By(fmt.Sprintf("setting carbonAwareEnabled to %s", strconv.FormatBool(enabled)))
			env.ExpectSettingsOverridden(map[string]string{
				"featureGates.carbonAwareEnabled": strconv.FormatBool(enabled),
			})

			By("waiting for pods to be deployed")
			for _, pod := range pods {
				env.ExpectCreated(pod)
			}
			env.EventuallyExpectPendingPodCount(selector, len(pods)) // TODO @JacobValdemar: Probably an one-off error here with len

			By("kicking off provisioning by applying the provisioner and nodeTemplate")
			env.ExpectCreated(provisioner, nodeTemplate)
			env.EventuallyExpectHealthyPodCount(selector, len(pods)) // TODO @JacobValdemar: Probably an one-off error here with len

			By("saving topology")
			var carbonAwareStatus string
			if enabled {
				carbonAwareStatus = "enabled"
			} else {
				carbonAwareStatus = "disabled"
			}
			experimentDirectory = filepath.Join(
				experimentDirectory,
				"hetrogeneous-pods",
				fmt.Sprintf("file-%s", fileName),
				fmt.Sprintf("carbonAware-%s", carbonAwareStatus),
			)
			env.SaveTopology(experimentDirectory, "nodes.json")

		},
		EntryDescription("CarbonAwareEnabled=%t, podTopologyInputFile=%s.json"),
		// Entry(nil, true, "observed-pod-topology1"),
		// Entry(nil, false, "observed-pod-topology1"),
		// Entry(nil, true, "observed-pod-topology2"),
		// Entry(nil, false, "observed-pod-topology2"),
	)

	// TODO @JacobValdemar: Problem is that every time it is always only ONE node which is provisioned. Never two small or a combination hereof
	// TODO @JacobValdemar: Consolidation
	// TODO @JacobValdemar: Create script that exports real cluster's pod+node topology, which can be used as input to test to see potential improvements
	//				          original cluster -> Karpenter -> Carbon Aware Karpenter
})
