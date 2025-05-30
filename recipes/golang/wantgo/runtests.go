package wantgo

import (
	"bytes"
	"context"
	"encoding/json"
	"path"
	"strings"

	"blobcache.io/glfs"
	"go.brendoncarroll.net/state/cadata"

	"wantbuild.io/want/src/wantcfg"
	"wantbuild.io/want/src/wantjob"
	"wantbuild.io/want/src/wantqemu"
)

const MaxConfigSize = 1e6

// RunTestTask will run build and run all the tests in the module
type RunTestsTask struct {
	Module   glfs.Ref
	ModCache *glfs.Ref
	RunTestsConfig

	VMSpec
}

type VMSpec struct {
	BaseFS glfs.Ref
	Kernel glfs.Ref
}

type RunTestsConfig struct {
	GOOS   string
	GOARCH string
	Ignore []string
}

func PostRunTestsTask(ctx context.Context, s cadata.PostExister, x RunTestsTask) (*glfs.Ref, error) {
	cfgJson, err := json.Marshal(x.RunTestsConfig)
	if err != nil {
		return nil, err
	}
	cfgRef, err := glfs.PostBlob(ctx, s, bytes.NewReader(cfgJson))
	if err != nil {
		return nil, err
	}
	m := map[string]glfs.Ref{
		"module":         x.Module,
		"run_tests.json": *cfgRef,

		"basefs": x.BaseFS,
		"kernel": x.Kernel,
	}
	if x.ModCache != nil {
		m["modcache"] = *x.ModCache
	}
	return glfs.PostTreeMap(ctx, s, m)
}

func GetRunTestsTask(ctx context.Context, s cadata.Getter, x glfs.Ref) (*RunTestsTask, error) {
	moduleRef, err := glfs.GetAtPath(ctx, s, x, "module")
	if err != nil {
		return nil, err
	}
	modcacheRef, err := glfs.GetAtPath(ctx, s, x, "modcache")
	if err != nil && !glfs.IsErrNoEnt(err) {
		return nil, err
	}
	configRef, err := glfs.GetAtPath(ctx, s, x, "run_tests.json")
	if err != nil {
		return nil, err
	}
	configData, err := glfs.GetBlobBytes(ctx, s, *configRef, MaxConfigSize)
	if err != nil {
		return nil, err
	}
	var config RunTestsConfig
	if err := json.Unmarshal(configData, &config); err != nil {
		return nil, err
	}

	baseFS, err := glfs.GetAtPath(ctx, s, x, "basefs")
	if err != nil {
		return nil, err
	}
	kernel, err := glfs.GetAtPath(ctx, s, x, "kernel")
	if err != nil {
		return nil, err
	}

	return &RunTestsTask{
		Module:   *moduleRef,
		ModCache: modcacheRef,
		VMSpec: VMSpec{
			Kernel: *kernel,
			BaseFS: *baseFS,
		},
		RunTestsConfig: config,
	}, nil
}

// RunTests creates a graph with nodes for building and running a test.
func RunTests(jc wantjob.Ctx, src cadata.Getter, x RunTestsTask) (*glfs.Ref, error) {
	testExecs, err := makeAllTestExecs(jc, src, x)
	if err != nil {
		return nil, err
	}
	emptyTree, err := glfs.PostTreeSlice(jc.Context, jc.Dst, nil)
	if err != nil {
		return nil, err
	}

	ret := make(map[string]glfs.Ref, len(testExecs))
	for p, testExec := range testExecs {
		ioLayers, err := glfs.PostTreeSlice(jc.Context, jc.Dst, []glfs.TreeEntry{
			{Name: "output", FileMode: 0o777, Ref: *emptyTree},
			{Name: "input/testexec", FileMode: 0o777, Ref: testExec},
		})
		if err != nil {
			return nil, err
		}
		rootfs, err := glfs.Merge(jc.Context, jc.Dst, src, x.BaseFS, *ioLayers)
		if err != nil {
			return nil, err
		}
		args := []string{"-test.v", "-test.coverprofile", "/output"}
		taskRef, err := wantqemu.PostMicroVMTask(jc.Context, jc.Dst, wantqemu.MicroVMTask{
			Cores:  1,
			Memory: 1e9,
			Kernel: x.Kernel,
			VirtioFS: map[string]wantqemu.VirtioFSSpec{
				"rootfs": {Root: *rootfs, Writeable: true},
			},
			KernelArgs: "panic=-1 reboot=t init=/input/testexec " + strings.Join(args, " "),
			Output:     wantqemu.GrabVirtioFS("rootfs", wantcfg.Prefix("output")),
		})
		if err != nil {
			return nil, err
		}
		taskJson, err := json.Marshal(taskRef)
		if err != nil {
			return nil, err
		}
		res, outStore, err := wantjob.Do(jc.Context, jc.System, jc.Dst, wantjob.Task{
			Op:    "qemu.amd64_microvm",
			Input: taskJson,
		})
		if err != nil {
			return nil, err
		}
		if err := res.Err(); err != nil {
			return nil, err
		}
		var outRef glfs.Ref
		if err := json.Unmarshal(res.Root, &outRef); err != nil {
			return nil, err
		}
		if err := glfs.Sync(jc.Context, jc.Dst, outStore, outRef); err != nil {
			return nil, err
		}
		ret[p+".test"] = outRef
	}
	return glfs.PostTreeMap(jc.Context, jc.Dst, ret)
}

// makeAllTestExecs builds test executables for each package.
// The map key will be the package path within the module.
func makeAllTestExecs(jc wantjob.Ctx, src cadata.Getter, x RunTestsTask) (map[string]glfs.Ref, error) {
	tasks, err := makeTestExecTasks(jc.Context, src, x.Module, x.ModCache, x.RunTestsConfig)
	if err != nil {
		return nil, err
	}
	jobIdxs := make([]wantjob.Idx, len(tasks))
	for i, v := range tasks {
		ref, err := PostMakeTestExecTask(jc.Context, jc.Dst, v)
		if err != nil {
			return nil, err
		}
		refData, err := json.Marshal(*ref)
		if err != nil {
			return nil, err
		}
		idx, err := jc.Spawn(jc.Context, src, wantjob.Task{
			Op:    OpMakeTestExec,
			Input: refData,
		})
		if err != nil {
			return nil, err
		}
		jobIdxs[i] = idx
	}
	testExecs := make(map[string]glfs.Ref)
	for i, idx := range jobIdxs {
		if err := jc.Await(jc.Context, idx); err != nil {
			return nil, err
		}
		res, outStore, err := jc.ViewResult(jc.Context, idx)
		if err != nil {
			return nil, err
		}
		if err := res.Err(); err != nil {
			return nil, err
		}
		var outRef glfs.Ref
		if err := json.Unmarshal(res.Root, &outRef); err != nil {
			return nil, err
		}
		if err := glfs.Sync(jc.Context, jc.Dst, outStore, outRef); err != nil {
			return nil, err
		}
		p := tasks[i].Path
		testExecs[p] = outRef
	}
	return testExecs, nil
}

// makeTestExecTasks generates tasks to create a test executable for each package
func makeTestExecTasks(ctx context.Context, src cadata.Getter, modRef glfs.Ref, modCache *glfs.Ref, rtcfg RunTestsConfig) ([]MakeTestExecTask, error) {
	var ret []MakeTestExecTask
	if err := glfs.WalkTree(ctx, src, modRef, func(prefix string, ent glfs.TreeEntry) error {
		if yes, err := IsGoPackage(ctx, src, ent.Ref); err != nil {
			return err
		} else if yes {
			p := path.Join(prefix, ent.Name)
			ret = append(ret, MakeTestExecTask{
				Module:   modRef,
				ModCache: modCache,
				MakeTestExecConfig: MakeTestExecConfig{
					GOARCH: rtcfg.GOARCH,
					GOOS:   rtcfg.GOOS,
					Path:   p,
				},
			})
		}
		return nil
	}); err != nil {
		return nil, err
	}
	return ret, nil
}
