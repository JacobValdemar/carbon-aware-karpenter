package carbon_test

import (
	"fmt"
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
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Consolidation", Label(debug.NoWatch), Label(debug.NoEvents), func() {
	var timenow string = time.Now().Format("2006-01-02-15-04")
	var experimentDirectory string

	BeforeEach(func() {
		experimentDirectory = filepath.Join("experiments", timenow, "Consolidation")
	})

	PDescribeTable("should consolidate nodes (replace)", func(carbonAwareEnabled bool) {
		experimentDirectory = filepath.Join(
			experimentDirectory,
			"consolidate-nodes",
			fmt.Sprintf("carbonAware-%t", carbonAwareEnabled),
		)
		provider := awstest.AWSNodeTemplate(v1alpha1.AWSNodeTemplateSpec{AWS: v1alpha1.AWS{
			SecurityGroupSelector: map[string]string{"karpenter.sh/discovery": settings.FromContext(env.Context).ClusterName},
			SubnetSelector:        map[string]string{"karpenter.sh/discovery": settings.FromContext(env.Context).ClusterName},
		}})
		provisioner := test.Provisioner(test.ProvisionerOptions{
			Requirements: []v1.NodeSelectorRequirement{
				{
					Key:      v1alpha5.LabelCapacityType,
					Operator: v1.NodeSelectorOpIn,
					Values:   []string{"on-demand"},
				},
				{
					Key:      v1alpha1.LabelInstanceSize,
					Operator: v1.NodeSelectorOpIn,
					Values:   []string{"large", "2xlarge"},
				},
				env.GetAllowedInstanceCategories(),
			},
			ProviderRef: &v1alpha5.MachineTemplateRef{Name: provider.Name},
		})

		var numPods int32 = 3
		largeDep := test.Deployment(test.DeploymentOptions{
			Replicas: numPods,
			PodOptions: test.PodOptions{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app": "large-app"},
				},
				TopologySpreadConstraints: []v1.TopologySpreadConstraint{
					{
						MaxSkew:           1,
						TopologyKey:       v1.LabelHostname,
						WhenUnsatisfiable: v1.DoNotSchedule,
						LabelSelector: &metav1.LabelSelector{
							MatchLabels: map[string]string{
								"app": "large-app",
							},
						},
					},
				},
				ResourceRequirements: v1.ResourceRequirements{
					Requests: v1.ResourceList{v1.ResourceCPU: resource.MustParse("4")},
				},
			},
		})
		smallDep := test.Deployment(test.DeploymentOptions{
			Replicas: numPods,
			PodOptions: test.PodOptions{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app": "small-app"},
				},
				TopologySpreadConstraints: []v1.TopologySpreadConstraint{
					{
						MaxSkew:           1,
						TopologyKey:       v1.LabelHostname,
						WhenUnsatisfiable: v1.DoNotSchedule,
						LabelSelector: &metav1.LabelSelector{
							MatchLabels: map[string]string{
								"app": "small-app",
							},
						},
					},
				},
				ResourceRequirements: v1.ResourceRequirements{
					Requests: v1.ResourceList{v1.ResourceCPU: resource.MustParse("1.5")},
				},
			},
		})
		selector := labels.SelectorFromSet(largeDep.Spec.Selector.MatchLabels)

		By(fmt.Sprintf("setting carbonAwareEnabled to %s", strconv.FormatBool(carbonAwareEnabled)))
		env.ExpectSettingsOverridden(map[string]string{
			"featureGates.carbonAwareEnabled": strconv.FormatBool(carbonAwareEnabled),
		})

		env.ExpectCreated(provisioner, provider, largeDep, smallDep)

		env.EventuallyExpectHealthyPodCount(selector, int(numPods))

		// 3 nodes due to the anti-affinity rules
		env.ExpectCreatedNodeCount("==", 3)
		env.SaveTopology(experimentDirectory, "nodesBeforeConsolidation.json")

		// scaling down the large deployment leaves only small pods on each node
		largeDep.Spec.Replicas = aws.Int32(0)
		env.ExpectUpdated(largeDep)
		env.EventuallyExpectAvgUtilization(v1.ResourceCPU, "<", 0.5)

		provisioner.Spec.TTLSecondsAfterEmpty = nil
		provisioner.Spec.Consolidation = &v1alpha5.Consolidation{
			Enabled: aws.Bool(true),
		}
		env.ExpectUpdated(provisioner)

		// With consolidation enabled, we now must replace each node in turn to consolidate due to the anti-affinity
		// rules on the smaller deployment.  The 2xl nodes should go to a large
		env.EventuallyExpectAvgUtilization(v1.ResourceCPU, ">", 0.8)

		var nodes v1.NodeList
		Expect(env.Client.List(env.Context, &nodes)).To(Succeed())
		numLargeNodes := 0
		numOtherNodes := 0
		for _, n := range nodes.Items {
			// only count the nodes created by the provisoiner
			if n.Labels[v1alpha5.ProvisionerNameLabelKey] != provisioner.Name {
				continue
			}
			if strings.HasSuffix(n.Labels[v1.LabelInstanceTypeStable], ".large") {
				numLargeNodes++
			} else {
				numOtherNodes++
			}
		}

		// all of the 2xlarge nodes should have been replaced with large instance types
		Expect(numLargeNodes).To(Equal(3))
		// and we should have no other nodes
		Expect(numOtherNodes).To(Equal(0))

		env.SaveTopology(experimentDirectory, "nodesAfterConsolidation.json")

		env.ExpectDeleted(largeDep, smallDep)
	},
		EntryDescription("CarbonAwareEnabled=%t"),
	// Entry(nil, true),
	// Entry(nil, false),
	)
})
