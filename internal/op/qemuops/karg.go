package qemuops

import (
	"strconv"
	"strings"

	"golang.org/x/exp/slices"
)

type kernelArgs struct {
	Console string

	Panic          int
	Reboot         string
	IgnoreLogLevel bool

	RootFSType string
	Root       string

	LPJ            int
	ClockSource    string
	PCI            string
	RandomTrustCpu string

	Init     string
	InitArgs []string
}

func (ka *kernelArgs) VirtioFSRoot(tag string) {
	ka.RootFSType = "virtiofs"
	ka.Root = tag + " rw"
}

func (ka kernelArgs) String() string {
	var args []string

	for k, v := range map[string]string{
		"reboot":  ka.Reboot,
		"console": ka.Console,

		"root":       ka.Root,
		"rootfstype": ka.RootFSType,

		"clocksource":      ka.ClockSource,
		"random.trust_cpu": ka.RandomTrustCpu,
		"init":             ka.Init,
	} {
		if v != "" {
			args = append(args, k+"="+v)
		}
	}

	for k, n := range map[string]int{
		"panic": ka.Panic,
		"lpj":   ka.LPJ,
	} {
		if n != 0 {
			args = append(args, k+"="+strconv.Itoa(n))
		}
	}

	for k, yes := range map[string]bool{
		"ignore_loglevel": ka.IgnoreLogLevel,
	} {
		if yes {
			args = append(args, k)
		}
	}

	slices.Sort(args)
	ret := strings.Join(args, " ")
	for i, ia := range ka.InitArgs {
		if i == 0 {
			ret += " "
		}
		if strings.Contains(ia, " ") {
			ia = "'" + ia + "'"
		}
		ret += " " + ia
	}
	return ret
}
