package carbon_test

import (
	"fmt"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/karpenter-core/pkg/apis/v1alpha5"
	"github.com/aws/karpenter-core/pkg/test"
	"github.com/aws/karpenter/pkg/apis/settings"
	"github.com/aws/karpenter/pkg/apis/v1alpha1"
	awstest "github.com/aws/karpenter/pkg/test"
	"github.com/aws/karpenter/test/pkg/debug"
	"github.com/samber/lo"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/labels"
	"knative.dev/pkg/ptr"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const WaitForConsolidation int = -1

var _ = Describe("Consolidation", Label(debug.NoWatch), Label(debug.NoEvents), func() {
	var timenow string = time.Now().Format("2006-01-02-15-04")
	var experimentDirectory string
	var provider *v1alpha1.AWSNodeTemplate
	var provisioner *v1alpha5.Provisioner
	var carbonAwareEnabled bool

	BeforeEach(func() {
		experimentDirectory = filepath.Join("experiments", timenow, "Consolidation")

		provider = awstest.AWSNodeTemplate(v1alpha1.AWSNodeTemplateSpec{AWS: v1alpha1.AWS{
			SecurityGroupSelector: map[string]string{"karpenter.sh/discovery": settings.FromContext(env.Context).ClusterName},
			SubnetSelector:        map[string]string{"karpenter.sh/discovery": settings.FromContext(env.Context).ClusterName},
		}})
		provisioner = test.Provisioner(test.ProvisionerOptions{
			Requirements: env.GetProvisionerRequirements(),
			ProviderRef:  &v1alpha5.MachineTemplateRef{Name: provider.Name},
			Kubelet: &v1alpha5.KubeletConfiguration{
				PodsPerCore: ptr.Int32(30),
			},
			Consolidation: &v1alpha5.Consolidation{
				Enabled: aws.Bool(true),
			},
			TTLSecondsAfterEmpty: nil,
		})
	})

	PDescribeTable("should consolidate nodes (replace)", func(carbonAwareEnabled bool, fileName string) {
		var pods []*v1.Pod
		experimentDirectory = filepath.Join(
			experimentDirectory,
			"consolidate-nodes",
			fmt.Sprintf("carbonAware-%t", carbonAwareEnabled),
		)

		pods, selector := env.ImportPodTopologyTestInput(path.Join("experiments", "testInput"), fileName+".json")

		By(fmt.Sprintf("setting carbonAwareEnabled to %s", strconv.FormatBool(carbonAwareEnabled)))
		env.ExpectSettingsOverridden(map[string]string{
			"featureGates.carbonAwareEnabled": strconv.FormatBool(carbonAwareEnabled),
		})

		By("waiting for pods to be deployed")
		for _, pod := range pods {
			env.ExpectCreated(pod)
		}
		env.EventuallyExpectPendingPodCount(selector, len(pods)) // TODO @JacobValdemar: Probably an one-off error here with len

		By("kicking off provisioning by applying the provisioner and nodeTemplate")
		env.ExpectCreated(provisioner, provider)
		env.EventuallyExpectHealthyPodCount(selector, len(pods))
		env.SaveTopology(experimentDirectory, "nodesBeforeConsolidation.json")

		// for _, pod := range pods {
		// 	env.ExpectDeleted(pod)
		// }
	},
		EntryDescription("CarbonAwareEnabled=%t, podTopologyInputFile=%s.json"),
		//Entry(nil, true, "observed-pod-topology2"),
		//Entry(nil, false, "observed-pod-topology2"),
	)

	Context("should consolidate homogeneous nodes", func() {
		var replicas int32
		var deployment *appsv1.Deployment
		var selector labels.Selector

		BeforeEach(func() {
			carbonAwareEnabled = true

			experimentDirectory = filepath.Join(
				experimentDirectory,
				"consolidate-homogeneous-nodes",
			)

			replicas = int32(2)
			deployment = test.Deployment(test.DeploymentOptions{
				PodOptions: test.PodOptions{
					ResourceRequirements: v1.ResourceRequirements{
						Requests: v1.ResourceList{
							v1.ResourceCPU:    resource.MustParse("1"),
							v1.ResourceMemory: resource.MustParse("100Mi"),
						},
					},
					TerminationGracePeriodSeconds: lo.ToPtr[int64](0),
				},
				Replicas: replicas,
			})
			selector = labels.SelectorFromSet(deployment.Spec.Selector.MatchLabels)
		})

		PIt("scaling deployment 2->5->7", func() {
			By(fmt.Sprintf("setting carbonAwareEnabled to %s", strconv.FormatBool(carbonAwareEnabled)))
			env.ExpectSettingsOverridden(map[string]string{
				"featureGates.carbonAwareEnabled": strconv.FormatBool(carbonAwareEnabled),
			})

			By("kicking off provisioning by applying the provisioner and nodeTemplate")
			env.ExpectCreated(provisioner, provider)

			experimentDirectory = filepath.Join(
				experimentDirectory,
				"2-5-7",
			)

			By("waiting for pods to be deployed")
			env.ExpectCreated(deployment)
			env.EventuallyExpectHealthyPodCount(selector, int(replicas))
			env.SaveTopology(experimentDirectory, fmt.Sprintf("nodesAt%dReplicas.json", replicas))

			replicas = 5
			By(fmt.Sprintf("scaling deployment to %d replicas", replicas))
			deployment.Spec.Replicas = ptr.Int32(replicas)
			env.ExpectUpdated(deployment)
			env.EventuallyExpectHealthyPodCount(selector, int(replicas))
			env.SaveTopology(experimentDirectory, fmt.Sprintf("nodesAt%dReplicas.json", replicas))

			replicas = 7
			By(fmt.Sprintf("scaling deployment to %d replicas", replicas))
			deployment.Spec.Replicas = ptr.Int32(replicas)
			env.ExpectUpdated(deployment)
			env.EventuallyExpectHealthyPodCount(selector, int(replicas))
			env.SaveTopology(experimentDirectory, fmt.Sprintf("nodesAt%dReplicas.json", replicas))
		})

		DescribeTable("scaling deployment", func(carbonAwareEnabled bool, testList []int) {
			By(fmt.Sprintf("setting carbonAwareEnabled to %s", strconv.FormatBool(carbonAwareEnabled)))
			env.ExpectSettingsOverridden(map[string]string{
				"featureGates.carbonAwareEnabled": strconv.FormatBool(carbonAwareEnabled),
			})

			By("kicking off provisioning by applying the provisioner and nodeTemplate")
			env.ExpectCreated(provisioner, provider)

			experimentDirectory = filepath.Join(
				experimentDirectory,
				fmt.Sprintf("carbonAware-%t", carbonAwareEnabled),
				strings.Trim(strings.Join(strings.Fields(fmt.Sprint(testList)), "-"), "[]"),
			)
			deployment.Spec.Replicas = ptr.Int32(0)
			env.ExpectCreated(deployment)

			// TODO: Jeg skal bare give den tid til at consolidere efter hver gang. Så den har mulighed hvis den vil.
			// TODO: Jeg skal også gøre så jeg kan køre hhv. enabled og disabled carbon aware tests og se dem begge.
			for step, newReplicaCount := range testList {
				if newReplicaCount == WaitForConsolidation {
					By("waiting for consolidation")
					// nodesAtLast := env.Monitor.CreatedNodes()
					time.Sleep(3 * time.Minute)
					// Eventually(func(g Gomega) {
					// 	currentNodes := env.Monitor.CreatedNodes()
					// 	g.Expect(len(currentNodes)).To(BeNumerically("<", len(nodesAtLast)))
					// }).WithTimeout(3 * time.Minute).WithOffset(1).Should(Succeed())
					env.EventuallyExpectHealthyPodCount(selector, int(replicas))
					env.SaveTopology(experimentDirectory, fmt.Sprintf("%d-consolidated.json", step))
					continue
				}

				replicas = int32(newReplicaCount)
				By(fmt.Sprintf("scaling deployment to %d replicas", replicas))
				deployment.Spec.Replicas = ptr.Int32(replicas)
				env.ExpectUpdated(deployment)
				env.EventuallyExpectHealthyPodCount(selector, int(replicas))
				env.SaveTopology(experimentDirectory, fmt.Sprintf("%d-at-%d-replicas.json", step, replicas))
			}

		},
			// Entry(nil, []int{3, 4, 10, WaitForConsolidation}),
			// Entry(nil, []int{3, 4, 5, 7, WaitForConsolidation, 10, WaitForConsolidation}),
			Entry(nil, true, []int{10, WaitForConsolidation}),
			Entry(nil, false, []int{10, WaitForConsolidation}),
		)

		PIt("setting replica count to 7", func() {
			By(fmt.Sprintf("setting carbonAwareEnabled to %s", strconv.FormatBool(carbonAwareEnabled)))
			env.ExpectSettingsOverridden(map[string]string{
				"featureGates.carbonAwareEnabled": strconv.FormatBool(carbonAwareEnabled),
			})

			By("kicking off provisioning by applying the provisioner and nodeTemplate")
			env.ExpectCreated(provisioner, provider)

			experimentDirectory = filepath.Join(
				experimentDirectory,
				"7",
			)

			replicas = 7
			By(fmt.Sprintf("scaling deployment to %d replicas", replicas))
			deployment.Spec.Replicas = ptr.Int32(replicas)

			By("waiting for pods to be deployed")
			env.ExpectCreated(deployment)
			env.EventuallyExpectHealthyPodCount(selector, int(replicas))
			env.SaveTopology(experimentDirectory, fmt.Sprintf("nodesAt%dReplicas.json", replicas))
		})

		PIt("scaling deployment 2->5->7->wait", func() {
			By(fmt.Sprintf("setting carbonAwareEnabled to %s", strconv.FormatBool(carbonAwareEnabled)))
			env.ExpectSettingsOverridden(map[string]string{
				"featureGates.carbonAwareEnabled": strconv.FormatBool(carbonAwareEnabled),
			})

			By("kicking off provisioning by applying the provisioner and nodeTemplate")
			env.ExpectCreated(provisioner, provider)

			experimentDirectory = filepath.Join(
				experimentDirectory,
				"2-5-7",
			)

			By("waiting for pods to be deployed")
			env.ExpectCreated(deployment)
			env.EventuallyExpectHealthyPodCount(selector, int(replicas))
			env.SaveTopology(experimentDirectory, fmt.Sprintf("nodesAt%dReplicas.json", replicas))

			replicas = 5
			By(fmt.Sprintf("scaling deployment to %d replicas", replicas))
			deployment.Spec.Replicas = ptr.Int32(replicas)
			env.ExpectUpdated(deployment)
			env.EventuallyExpectHealthyPodCount(selector, int(replicas))
			env.SaveTopology(experimentDirectory, fmt.Sprintf("nodesAt%dReplicas.json", replicas))

			replicas = 7
			By(fmt.Sprintf("scaling deployment to %d replicas", replicas))
			deployment.Spec.Replicas = ptr.Int32(replicas)
			env.ExpectUpdated(deployment)
			env.EventuallyExpectHealthyPodCount(selector, int(replicas))
			env.SaveTopology(experimentDirectory, fmt.Sprintf("nodesAt%dReplicas.json", replicas))

			By("waiting for consolidation")
			nodesAtSeven := env.Monitor.CreatedNodes()
			Eventually(func(g Gomega) {
				currentNodes := env.Monitor.CreatedNodes()
				g.Expect(len(currentNodes)).To(BeNumerically("<", len(nodesAtSeven)))
			}).WithTimeout(10 * time.Minute).WithOffset(1).Should(Succeed())
			env.SaveTopology(experimentDirectory, "nodesWhenConsolidated.json")
		})
	})

})
