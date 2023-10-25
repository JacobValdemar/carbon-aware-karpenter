package common

import (
	"encoding/json"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2" //nolint:revive,stylecheck
	. "github.com/onsi/gomega"    //nolint:revive,stylecheck
)

func (env *Environment) SaveTopology(dir string, fileName string) {
	GinkgoHelper()
	createdNodes := env.Monitor.CreatedNodes()

	b, err := json.Marshal(createdNodes)
	if err != nil {
		GinkgoWriter.Printf("Error json.Marshal: %s", err)
		return
	}

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
