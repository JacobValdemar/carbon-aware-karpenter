package common

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/aws/karpenter-core/pkg/apis/v1alpha5"
	"github.com/aws/karpenter-core/pkg/test"
	"github.com/aws/karpenter/pkg/apis/v1alpha1"
	. "github.com/onsi/ginkgo/v2" //nolint:revive,stylecheck
	. "github.com/onsi/gomega"    //nolint:revive,stylecheck
	appsv1 "k8s.io/api/apps/v1"
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

	var instances []string
	for _, node := range nodesUtilization {
		instances = append(instances, fmt.Sprintf("%s: %f", node.InstanceType, node.Utilization))
	}

	sort.Slice(instances, func(i, j int) bool {
		return i < j
	})

	impact := analyzer(nodesUtilization)

	save := struct {
		Impact  float64
		Summary []string
		Verbose []NodeUtil
	}{
		Impact:  impact,
		Summary: instances,
		Verbose: nodesUtilization,
	}

	b, err := json.MarshalIndent(save, "", "    ")
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

type InputContainer struct {
	CPURequest    string `json:"cpu_request,omitempty"`
	MemoryRequest string `json:"memory_request,omitempty"`
}

func (env *Environment) Sleep(sleeptime time.Duration) {
	By(fmt.Sprintf("waiting for consolidation (%s)", sleeptime.String()))
	time.Sleep(sleeptime)
}

func (env *Environment) ImportPodTopologyTestInput(dir string, fileName string) ([]*appsv1.Deployment, labels.Selector) {
	By(fmt.Sprintf("loading pod topology from %s", fileName))

	path := filepath.Join(dir, fileName)
	jsonFile, err := os.Open(path)
	Expect(err).NotTo(HaveOccurred())
	defer jsonFile.Close()

	byteValue, _ := io.ReadAll(jsonFile)

	var inputPods [][]InputContainer
	err = json.Unmarshal(byteValue, &inputPods)
	Expect(err).NotTo(HaveOccurred())

	var deployments []*appsv1.Deployment
	label := map[string]string{"testing/pod-app": "loaded"}
	selector := labels.SelectorFromSet(label)
	for _, inputPod := range inputPods {

		var cpu resource.Quantity
		var memory resource.Quantity

		for _, container := range inputPod {
			CPURequest := container.CPURequest
			MemoryRequest := container.MemoryRequest

			if CPURequest == "" {
				CPURequest = "0"
			}

			if MemoryRequest == "" {
				MemoryRequest = "0"
			}

			cpu.Add(resource.MustParse(CPURequest))
			memory.Add(resource.MustParse(MemoryRequest))
		}

		requests := v1.ResourceList{
			v1.ResourceCPU:    cpu,
			v1.ResourceMemory: memory,
		}

		//fmt.Printf("cpu: %s, memory: %s\n", requests.Cpu().String(), requests.Memory().String())
		if cpu.IsZero() && memory.IsZero() {
			continue
		}

		deployments = append(deployments, test.Deployment(test.DeploymentOptions{
			PodOptions: test.PodOptions{
				ObjectMeta: metav1.ObjectMeta{
					Labels: label,
				},
				ResourceRequirements: v1.ResourceRequirements{
					Requests: requests,
				},
			},
			Replicas: 1,
		}))
	}

	return deployments, selector
}
