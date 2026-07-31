package golden

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type parallelTestConfig struct {
	*BaseConfig
	parallelism int
}

func (c *parallelTestConfig) Parallelism() int {
	return c.parallelism
}

func newParallelTestConfig(d *Dag, parallelism int) *parallelTestConfig {
	c := &parallelTestConfig{
		BaseConfig:  NewBasicConfig("", "test", "test", nil, nil, nil),
		parallelism: parallelism,
	}
	c.d = d
	c.setConfigOwner(c)
	return c
}

func newParallelTestBlock(address string, ready bool) *DummyResource {
	b := &DummyResource{
		BaseBlock: &BaseBlock{
			blockAddress: address,
			hb: &HclBlock{Block: &hclsyntax.Block{
				Body: &hclsyntax.Body{Attributes: hclsyntax.Attributes{}},
			}},
		},
	}
	if ready {
		b.markReady()
	}
	return b
}

func addParallelTestBlock(t *testing.T, d *Dag, address string, ready bool) *DummyResource {
	t.Helper()
	b := newParallelTestBlock(address, ready)
	require.NoError(t, d.AddVertexByID(address, b))
	return b
}

func waitForParallelCallbacks(t *testing.T, started <-chan string, count int) {
	t.Helper()
	for range count {
		select {
		case <-started:
		case <-time.After(2 * time.Second):
			t.Fatalf("timed out waiting for %d parallel callbacks", count)
		}
	}
}

func TestRunDagOnParallelHonorsParallelism(t *testing.T) {
	d := newDag()
	const blockCount = 6
	const parallelism = 2
	for i := range blockCount {
		addParallelTestBlock(t, d, fmt.Sprintf("block.%d", i), true)
	}

	c := newParallelTestConfig(d, parallelism)
	started := make(chan string, blockCount)
	release := make(chan struct{})
	done := make(chan error, 1)
	var current atomic.Int32
	var maximum atomic.Int32

	go func() {
		done <- c.runDag(func(b Block) error {
			running := current.Add(1)
			for {
				observed := maximum.Load()
				if running <= observed || maximum.CompareAndSwap(observed, running) {
					break
				}
			}
			started <- b.Address()
			<-release
			current.Add(-1)
			return nil
		})
	}()

	waitForParallelCallbacks(t, started, parallelism)
	assert.Equal(t, int32(parallelism), maximum.Load())
	select {
	case address := <-started:
		t.Fatalf("callback %s exceeded parallelism limit", address)
	case <-time.After(100 * time.Millisecond):
	}

	close(release)
	require.NoError(t, <-done)
	assert.Equal(t, int32(parallelism), maximum.Load())
	assert.Len(t, started, blockCount-parallelism)
}

func TestRunDagOnParallelWaitsForDependenciesFromCurrentRun(t *testing.T) {
	d := newDag()
	for _, address := range []string{"A", "B", "C", "D"} {
		// Apply starts with blocks already marked ready by Plan.
		addParallelTestBlock(t, d, address, true)
	}
	require.NoError(t, d.AddEdge("A", "C"))
	require.NoError(t, d.AddEdge("B", "C"))
	require.NoError(t, d.AddEdge("C", "D"))

	c := newParallelTestConfig(d, 2)
	rootStarted := make(chan string, 2)
	releaseRoots := make(chan struct{})
	done := make(chan error, 1)
	var mu sync.Mutex
	completed := make(map[string]bool)

	go func() {
		done <- c.runDag(func(b Block) error {
			address := b.Address()
			if address == "A" || address == "B" {
				rootStarted <- address
				<-releaseRoots
			}

			mu.Lock()
			defer mu.Unlock()
			switch address {
			case "C":
				if !completed["A"] || !completed["B"] {
					return errors.New("C started before both parents completed")
				}
			case "D":
				if !completed["C"] {
					return errors.New("D started before C completed")
				}
			}
			completed[address] = true
			return nil
		})
	}()

	waitForParallelCallbacks(t, rootStarted, 2)
	mu.Lock()
	assert.Empty(t, completed)
	mu.Unlock()
	close(releaseRoots)
	require.NoError(t, <-done)
	assert.Equal(t, map[string]bool{"A": true, "B": true, "C": true, "D": true}, completed)
}

func TestRunDagOnParallelBlocksFailedDescendantsAndContinuesIndependentBranches(t *testing.T) {
	d := newDag()
	for _, address := range []string{"failed", "blocked", "independent", "independent-child"} {
		addParallelTestBlock(t, d, address, true)
	}
	require.NoError(t, d.AddEdge("failed", "blocked"))
	require.NoError(t, d.AddEdge("independent", "independent-child"))

	c := newParallelTestConfig(d, 2)
	var called sync.Map
	err := c.runDag(func(b Block) error {
		called.Store(b.Address(), true)
		if b.Address() == "failed" {
			return errors.New("apply failed")
		}
		return nil
	})

	require.ErrorContains(t, err, "apply failed")
	_, blockedCalled := called.Load("blocked")
	assert.False(t, blockedCalled)
	_, independentCalled := called.Load("independent")
	assert.True(t, independentCalled)
	_, independentChildCalled := called.Load("independent-child")
	assert.True(t, independentChildCalled)
}

func TestRunDagOnParallelRejectsNonPositiveParallelism(t *testing.T) {
	d := newDag()
	addParallelTestBlock(t, d, "block", true)
	c := newParallelTestConfig(d, 0)

	err := c.runDag(func(Block) error {
		t.Fatal("callback should not run")
		return nil
	})

	require.EqualError(t, err, "parallelism must be greater than zero, got 0")
}

func TestParallelConfigRunPlanSupportsForEachDependencies(t *testing.T) {
	testBase := newTestBase()
	defer testBase.teardown()
	testBase.dummyFsWithFiles(map[string]string{
		"test.hcl": `
data "dummy" source {
  data = {
    one = "first"
    two = "second"
  }
}

resource "dummy" expanded {
  for_each = data.dummy.source.data
  tags = {
    value = each.value
  }
}

resource "dummy" downstream {
  tags = resource.dummy.expanded["one"].tags
}
`,
	})

	hclBlocks, err := loadHclBlocks(false, "")
	require.NoError(t, err)
	c := &parallelTestConfig{
		BaseConfig:  NewBasicConfig("", "faketerraform", "ft", nil, nil, nil),
		parallelism: 4,
	}
	require.NoError(t, InitConfig(c, hclBlocks))
	require.Same(t, c, c.configOwner)
	require.NoError(t, c.RunPlan())

	resources := Blocks[TestResource](c)
	require.Len(t, resources, 3)
	var downstream *DummyResource
	for _, resource := range resources {
		if resource.Name() == "downstream" {
			downstream = resource.(*DummyResource)
			break
		}
	}
	require.NotNil(t, downstream)
	assert.Equal(t, map[string]string{"value": "first"}, downstream.Tags)
}
