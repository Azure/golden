package golden

import (
	"github.com/emirpasic/gods/queues/linkedlistqueue"
	"github.com/emirpasic/gods/sets/hashset"
	"github.com/hashicorp/go-multierror"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/heimdalr/dag"
)

type Dag struct {
	*dag.DAG
}

func newDag() *Dag {
	return &Dag{
		DAG: dag.NewDAG(),
	}
}

func (d *Dag) buildDag(blocks []Block) error {
	var walkErr error
	for _, b := range blocks {
		err := d.AddVertexByID(b.Address(), b)
		if err != nil {
			walkErr = multierror.Append(walkErr, err)
		}
	}
	for _, b := range blocks {
		diag := hclsyntax.Walk(b.HclBlock().Body, newDagWalker(d, b.Address()))
		if diag.HasErrors() {
			walkErr = multierror.Append(walkErr, diag.Errs()...)
		}
	}
	return walkErr
}

func (d *Dag) addEdge(from, to string) error {
	err := d.AddEdge(from, to)
	if err != nil {
		return err
	}
	return nil
}

func (d *Dag) runDag(c Config, onReady func(Block) error) error {
	var err error
	pending := linkedlistqueue.New()
	var prePlanBlocks, otherBlocks []Block
	for _, v := range d.GetRoots() {
		b := v.(Block)
		if _, ok := b.(PrePlanBlock); ok {
			prePlanBlocks = append(prePlanBlocks, b)
			continue
		}
		otherBlocks = append(otherBlocks, b)
	}
	for _, b := range prePlanBlocks {
		pending.Enqueue(b)
	}
	for _, b := range otherBlocks {
		pending.Enqueue(b)
	}
	for !pending.Empty() {
		next, _ := pending.Dequeue()
		b := next.(Block)
		// the node has already been expandable and deleted from dag
		address := b.Address()
		exist := d.exist(address)
		if !exist {
			continue
		}
		ancestors, dagErr := d.GetParents(address)
		if dagErr != nil {
			return dagErr
		}
		ready := true
		for upstreamAddress := range ancestors {
			v, dagErr := d.GetVertex(upstreamAddress)
			if dagErr != nil {
				return dagErr
			}
			if !v.(Block).isReadyForRead() {
				ready = false
				break
			}
		}
		if !ready {
			continue
		}
		if b.expandable() {
			children, dagErr := d.GetChildren(address)
			if dagErr != nil {
				return dagErr
			}
			expandedBlocks, err := c.expandBlock(b)
			if err != nil {
				return err
			}
			newPending := linkedlistqueue.New()
			for _, eb := range expandedBlocks {
				newPending.Enqueue(eb)
			}
			for _, b := range pending.Values() {
				newPending.Enqueue(b)
			}
			for _, n := range children {
				newPending.Enqueue(n)
			}
			pending = newPending
			continue
		}
		if callbackErr := onReady(b); callbackErr != nil {
			err = multierror.Append(err, callbackErr)
		}
		// this address might be expandable during onReady and no more exist.
		exist = d.exist(address)
		if !exist {
			continue
		}
		children, dagErr := d.GetChildren(address)
		if dagErr != nil {
			return dagErr
		}
		for _, n := range children {
			pending.Enqueue(n)
		}
	}
	return err
}

func traverse[T Block](d *Dag, f func(b T) error) error {
	var err error
	pending := linkedlistqueue.New()
	visited := hashset.New()
	for _, i := range d.GetRoots() {
		pending.Enqueue(i)
	}
	for !pending.Empty() {
		next, _ := pending.Dequeue()
		if visited.Contains(next) {
			continue
		}
		nb := next.(Block)
		address := nb.Address()
		parents, parentErr := d.GetParents(address)
		if parentErr != nil {
			return parentErr
		}
		ready := true
		for _, p := range parents {
			if !visited.Contains(p) {
				ready = false
				break
			}
		}
		if !ready {
			pending.Enqueue(next)
			continue
		}

		visited.Add(next)
		if b, ok := nb.(T); ok {
			if subError := f(b); subError != nil {
				err = multierror.Append(err, subError)
			}
		}
		children, getChildrenErr := d.GetChildren(address)
		if getChildrenErr != nil {
			return getChildrenErr
		}
		for _, c := range children {
			pending.Enqueue(c)
		}
	}
	return err
}

func (d *Dag) exist(address string) bool {
	n, existErr := d.GetVertex(address)
	notExist := n == nil || existErr != nil
	return !notExist
}
