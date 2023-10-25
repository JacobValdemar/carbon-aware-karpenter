package common

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2" //nolint:revive,stylecheck
)

func (env *Environment) SaveTopology() {
	GinkgoHelper()
	createdNodes := env.Monitor.CreatedNodes()

	b, err := json.Marshal(createdNodes)
	if err != nil {
		GinkgoWriter.Printf("Error json.Marshal: %s", err)
		return
	}

	timenow := time.Now().Format("2006-01-02-15-04")
	dir := filepath.Join("tmp", timenow)
	path := filepath.Join(dir, "nodes.json")

	err = os.MkdirAll(dir, os.ModePerm)
	if err != nil {
		GinkgoWriter.Printf("Error os.MkdirAll: %s", err)
		return
	}

	f, err := os.Create(path)
	if err != nil {
		GinkgoWriter.Printf("Error os.Create: %s", err)
		return
	}
	defer f.Close()
	_, err = f.Write(b)
	if err != nil {
		GinkgoWriter.Printf("Error f.Write: %s", err)
		return
	}

	f.Sync()

	// g.Expect(len(createdNodes)).To(BeNumerically(comparator, count),
	// 	fmt.Sprintf("expected %d created nodes, had %d (%v)", count, len(createdNodes), NodeNames(createdNodes)))
}
