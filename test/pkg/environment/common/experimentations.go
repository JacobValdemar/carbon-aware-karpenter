package common

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"

	"github.com/aws/karpenter/pkg/apis/v1alpha1"
	. "github.com/onsi/ginkgo/v2" //nolint:revive,stylecheck
	. "github.com/onsi/gomega"    //nolint:revive,stylecheck
	v1 "k8s.io/api/core/v1"
)

func (env *Environment) GetAllowedInstanceCategories() v1.NodeSelectorRequirement {
	return v1.NodeSelectorRequirement{
		Key:      v1alpha1.LabelInstanceCategory,
		Operator: v1.NodeSelectorOpIn,
		Values:   []string{"m", "t", "c", "r", "a"},
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

func (env *Environment) ImportPodTopologyTestInput(dir string, fileName string) []MyPod {
	path := filepath.Join(dir, fileName)
	jsonFile, err := os.Open(path)
	Expect(err).NotTo(HaveOccurred())
	defer jsonFile.Close()

	byteValue, _ := io.ReadAll(jsonFile)

	var pods []MyPod
	err = json.Unmarshal(byteValue, &pods)
	Expect(err).NotTo(HaveOccurred())

	return pods
}
