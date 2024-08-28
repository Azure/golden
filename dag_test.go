package golden

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestTraverse_TraverseDagShouldHonorDagOrder(t *testing.T) {
	for i := 0; i < 1000; i++ {
		dag := newDag()
		// Create dummy blocks
		blockA := &DummyResource{
			BaseBlock: &BaseBlock{blockAddress: "A"},
		}
		blockB := &DummyResource{
			BaseBlock: &BaseBlock{blockAddress: "B"},
		}
		blockC := &DummyResource{
			BaseBlock: &BaseBlock{blockAddress: "C"},
		}

		// Add blocks to the DAG
		require.NoError(t, dag.AddVertexByID(blockA.blockAddress, blockA))
		require.NoError(t, dag.AddVertexByID(blockB.blockAddress, blockB))
		require.NoError(t, dag.AddVertexByID(blockC.blockAddress, blockC))

		require.NoError(t, dag.AddEdge(blockA.blockAddress, blockB.blockAddress))
		require.NoError(t, dag.AddEdge(blockB.blockAddress, blockC.blockAddress))
		require.NoError(t, dag.AddEdge(blockA.blockAddress, blockC.blockAddress))

		// Track the number of times the callback is called for each block
		var visited []string
		err := traverse[*DummyResource](dag, func(b *DummyResource) error {
			visited = append(visited, b.blockAddress)
			return nil
		})

		// Ensure no error occurred during traversal
		require.NoError(t, err)

		require.Equal(t, []string{"A", "B", "C"}, visited)
	}
}
