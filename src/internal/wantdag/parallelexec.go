package wantdag

import (
	"sync/atomic"

	"github.com/blobcache/glfs"
	"go.brendoncarroll.net/state/cadata"
	"golang.org/x/sync/errgroup"

	"wantbuild.io/want/src/internal/glfstasks"
	"wantbuild.io/want/src/internal/stores"
	"wantbuild.io/want/src/wantjob"
)

func ParallelExecLast(jc wantjob.Ctx, s cadata.Getter, x DAG) (wantjob.Result, error) {
	// TODO: lazy execution
	nrs, err := ParallelExecAll(jc, s, x)
	if err != nil {
		return wantjob.Result{}, err
	}
	return nrs[len(nrs)-1], nil
}

func ParallelExecAll(jc wantjob.Ctx, src cadata.Getter, x DAG) ([]wantjob.Result, error) {
	results := make([]wantjob.Result, len(x))
	unblocks := make([][]NodeID, len(x))
	needCount := make([]int32, len(x))
	resolve := func(i NodeID) wantjob.Result {
		return results[i]
	}
	for i, n := range x {
		for _, in := range n.Inputs {
			// the input in.Node unblocks the output i
			unblocks[in.Node] = append(unblocks[in.Node], NodeID(i))
		}
		needCount[i] += int32(len(n.Inputs))
	}

	eg := errgroup.Group{}
	var worker func(NodeID) error
	worker = func(id NodeID) error {
		node := x[id]
		var outRef *glfs.Ref
		union := stores.Union{src, jc.Dst}
		switch {
		case node.IsFact():
			outRef = node.Value
			results[id] = *glfstasks.Success(*node.Value)
		case node.IsDerived():
			scratch := stores.NewMem()
			union = append(union, scratch)
			inputRef, err := PrepareInput(jc.Context, scratch, union, node.Inputs, resolve)
			if err != nil {
				return err
			}
			res, outSrc, err := wantjob.Do(jc.Context, jc.System, union, wantjob.Task{
				Op:    node.Op,
				Input: glfstasks.MarshalGLFSRef(*inputRef),
			})
			if err != nil {
				return err
			}
			if err := res.Err(); err != nil {
				jc.Errorf("error in node %v %v %v", id, node.Op, err)
			}
			if ref, err := glfstasks.ParseGLFSRef(res.Data); err == nil {
				outRef = ref
			}
			union = append(union, outSrc)
			results[id] = *res
		}
		if outRef != nil {
			if err := glfstasks.FastSync(jc.Context, jc.Dst, union, *outRef); err != nil {
				return err
			}
		}
		for _, ubn := range unblocks[id] {
			ubn := ubn
			if ct := atomic.AddInt32(&needCount[ubn], -1); ct == 0 {
				eg.Go(func() error { return worker(ubn) })
			}
		}
		return nil
	}
	for id, ct := range needCount {
		id := NodeID(id)
		if ct == 0 {
			eg.Go(func() error { return worker(id) })
		}
	}
	if err := eg.Wait(); err != nil {
		return nil, err
	}
	return results, nil
}
