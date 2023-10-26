package common

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/aws/karpenter-core/pkg/apis/v1alpha5"
	"github.com/aws/karpenter-core/pkg/test"
	"github.com/aws/karpenter/pkg/apis/v1alpha1"
	. "github.com/onsi/ginkgo/v2" //nolint:revive,stylecheck
	. "github.com/onsi/gomega"    //nolint:revive,stylecheck
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

func (env *Environment) GetProvisionerRequirements() []v1.NodeSelectorRequirement {
	return []v1.NodeSelectorRequirement{
		{
			Key:      v1alpha1.LabelInstanceCategory,
			Operator: v1.NodeSelectorOpIn,
			Values:   []string{"m", "t", "c", "r", "a"},
		},
		{
			Key:      v1alpha5.LabelCapacityType,
			Operator: v1.NodeSelectorOpIn,
			Values:   []string{v1alpha1.CapacityTypeOnDemand},
		},
		{
			Key:      v1.LabelOSStable,
			Operator: v1.NodeSelectorOpIn,
			Values:   []string{string(v1.Linux)},
		}, {
			Key:      v1.LabelArchStable,
			Operator: v1.NodeSelectorOpIn,
			Values:   []string{v1alpha5.ArchitectureAmd64, v1alpha5.ArchitectureArm64},
		},
	}
}

func (env *Environment) SaveTopology(dir string, fileName string) {
	GinkgoHelper()

	By("saving topology")

	// createdNodes := env.Monitor.CreatedNodes()

	// var instances []string
	// for _, node := range createdNodes {
	// 	instances = append(instances, node.Labels[v1.LabelInstanceType])
	// }

	nodesUtilization := env.Monitor.GetNodeUtilizations(v1.ResourceCPU)

	b, err := json.MarshalIndent(nodesUtilization, "", "    ")
	Expect(err).NotTo(HaveOccurred())

	err = os.MkdirAll(dir, os.ModePerm)
	Expect(err).NotTo(HaveOccurred())

	path := filepath.Join(dir, fileName)
	f, err := os.Create(path)
	Expect(err).NotTo(HaveOccurred())

	defer f.Close()
	_, err = f.Write(b)
	Expect(err).NotTo(HaveOccurred())

	f.Sync()

	// g.Expect(len(createdNodes)).To(BeNumerically(comparator, count),
	// 	fmt.Sprintf("expected %d created nodes, had %d (%v)", count, len(createdNodes), NodeNames(createdNodes)))
}

func (env *Environment) ImportPodTopologyTestInput(dir string, fileName string) ([]*v1.Pod, labels.Selector) {
	By(fmt.Sprintf("loading pod topology from %s", fileName))

	path := filepath.Join(dir, fileName)
	jsonFile, err := os.Open(path)
	Expect(err).NotTo(HaveOccurred())
	defer jsonFile.Close()

	byteValue, _ := io.ReadAll(jsonFile)

	var myPods []MyPod
	err = json.Unmarshal(byteValue, &myPods)
	Expect(err).NotTo(HaveOccurred())

	var pods []*v1.Pod
	label := map[string]string{"testing/pod-app": "loaded"}
	selector := labels.SelectorFromSet(label)
	for _, inputPod := range myPods {
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

	return pods, selector
}
