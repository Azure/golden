package golden

import (
	"fmt"

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
	if parallel, ok := c.(Parallelism); ok {
		return d.runDagOnParallel(c, parallel.Parallelism(), onReady)
	}
	return d.runDagSerial(c, onReady)
}

func (d *Dag) runDagSerial(c Config, onReady func(Block) error) error {
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

type parallelRunState interface {
	beginParallelRun()
	endParallelRun()
	markParallelRunning(address string)
	markParallelCompleted(address string)
}

type parallelBlockStatus uint8

const (
	parallelBlockQueued parallelBlockStatus = iota + 1
	parallelBlockRunning
	parallelBlockSucceeded
	parallelBlockBlocked
)

type parallelBlockResult struct {
	block Block
	err   error
}

func (d *Dag) runDagOnParallel(c Config, parallelism int, onReady func(Block) error) error {
	if parallelism <= 0 {
		return fmt.Errorf("parallelism must be greater than zero, got %d", parallelism)
	}

	state, tracksParallelRun := c.(parallelRunState)
	if tracksParallelRun {
		state.beginParallelRun()
		defer state.endParallelRun()
	}

	statuses := make(map[string]parallelBlockStatus)
	var pending []Block
	enqueue := func(b Block) {
		address := b.Address()
		if _, exists := statuses[address]; exists {
			return
		}
		statuses[address] = parallelBlockQueued
		pending = append(pending, b)
	}

	var prePlanBlocks, otherBlocks []Block
	for _, vertex := range d.GetRoots() {
		b := vertex.(Block)
		if _, ok := b.(PrePlanBlock); ok {
			prePlanBlocks = append(prePlanBlocks, b)
			continue
		}
		otherBlocks = append(otherBlocks, b)
	}
	for _, b := range prePlanBlocks {
		enqueue(b)
	}
	for _, b := range otherBlocks {
		enqueue(b)
	}

	results := make(chan parallelBlockResult, parallelism)
	running := 0
	var runErr error
	var fatalErr error

	removePending := func(index int) Block {
		b := pending[index]
		pending = append(pending[:index], pending[index+1:]...)
		return b
	}

	enqueueChildren := func(address string) error {
		if !d.exist(address) {
			return nil
		}
		children, err := d.GetChildren(address)
		if err != nil {
			return err
		}
		for _, child := range children {
			enqueue(child.(Block))
		}
		return nil
	}

	dependenciesReady := func(address string) (ready bool, blocked bool, err error) {
		parents, err := d.GetParents(address)
		if err != nil {
			return false, false, err
		}
		ready = true
		for parentAddress := range parents {
			switch statuses[parentAddress] {
			case parallelBlockSucceeded:
				continue
			case parallelBlockBlocked:
				return false, true, nil
			default:
				ready = false
			}
		}
		return ready, false, nil
	}

	handleResult := func(result parallelBlockResult) error {
		running--
		address := result.block.Address()
		if tracksParallelRun {
			state.markParallelCompleted(address)
		}
		if result.err != nil {
			runErr = multierror.Append(runErr, result.err)
			statuses[address] = parallelBlockBlocked
		} else if result.block.isReadyForRead() {
			statuses[address] = parallelBlockSucceeded
		} else {
			statuses[address] = parallelBlockBlocked
		}
		return enqueueChildren(address)
	}
	drainRunning := func() {
		for running > 0 {
			if err := handleResult(<-results); err != nil {
				fatalErr = multierror.Append(fatalErr, err)
			}
		}
	}

	for len(pending) > 0 || running > 0 {
		madeProgress := false

		for index := 0; index < len(pending) && running < parallelism; {
			b := pending[index]
			address := b.Address()
			if !d.exist(address) {
				removePending(index)
				statuses[address] = parallelBlockBlocked
				madeProgress = true
				continue
			}

			ready, blocked, err := dependenciesReady(address)
			if err != nil {
				fatalErr = multierror.Append(fatalErr, err)
				break
			}
			if blocked {
				removePending(index)
				statuses[address] = parallelBlockBlocked
				if err := enqueueChildren(address); err != nil {
					fatalErr = multierror.Append(fatalErr, err)
					break
				}
				madeProgress = true
				continue
			}
			if !ready {
				index++
				continue
			}

			if b.expandable() {
				// Expansion mutates the graph, so wait until callbacks stop reading it.
				if running > 0 {
					break
				}
				removePending(index)
				children, err := d.GetChildren(address)
				if err != nil {
					fatalErr = multierror.Append(fatalErr, err)
					break
				}
				expandedBlocks, err := c.expandBlock(b)
				if err != nil {
					fatalErr = multierror.Append(fatalErr, err)
					break
				}
				statuses[address] = parallelBlockSucceeded
				for _, expandedBlock := range expandedBlocks {
					enqueue(expandedBlock)
				}
				for _, child := range children {
					enqueue(child.(Block))
				}
				madeProgress = true
				continue
			}

			removePending(index)
			statuses[address] = parallelBlockRunning
			if tracksParallelRun {
				state.markParallelRunning(address)
			}
			running++
			madeProgress = true
			go func(block Block) {
				results <- parallelBlockResult{block: block, err: onReady(block)}
			}(b)
		}

		if fatalErr != nil {
			drainRunning()
			return multierror.Append(runErr, fatalErr)
		}

		if running > 0 {
			if err := handleResult(<-results); err != nil {
				fatalErr = multierror.Append(fatalErr, err)
				drainRunning()
				return multierror.Append(runErr, fatalErr)
			}
			continue
		}

		if len(pending) > 0 && !madeProgress {
			return multierror.Append(runErr, fmt.Errorf("parallel DAG execution stalled with %d pending blocks", len(pending)))
		}
	}

	return runErr
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
