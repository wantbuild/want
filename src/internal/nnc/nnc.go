// package nnc provides No Nonsense Containers.
package nnc

import (
	"fmt"
)

type MountSrc struct {
	// TmpFS mounts a tmpfs at the given path
	TmpFS  *struct{} `json:"tmpfs,omitempty"`
	ProcFS *struct{} `json:"procfs,omitempty"`
	SysFS  *struct{} `json:"sysfs,omitempty"`

	// Host mounts a host path into the container
	Host *string `json:"host,omitempty"`
}

func (m *MountSrc) Validate() error {
	var set []string
	if m.TmpFS != nil {
		set = append(set, "tmpfs")
	}
	if m.ProcFS != nil {
		set = append(set, "procfs")
	}
	if m.SysFS != nil {
		set = append(set, "sysfs")
	}
	if m.Host != nil {
		set = append(set, "host")
	}
	if len(set) != 1 {
		return fmt.Errorf("exactly one of tmpfs, procfs, or sysfs must be set")
	}
	return nil
}

type MountSpec struct {
	// Dst is the mountpoint, the front-end of the mount
	Dst string `json:"dst"`
	// Src is backend of the mount
	Src MountSrc `json:"src"`
}

type NetworkSpec struct {
}

type ContainerSpec struct {
	// Init is the path to the binary to run as PID 1
	Init string   `json:"entrypoint"`
	Args []string `json:"args"`
	Env  []string `json:"env"`

	Mounts  []MountSpec   `json:"mounts"`
	Network []NetworkSpec `json:"network"`
}

func (s *ContainerSpec) Validate() error {
	for _, m := range s.Mounts {
		if err := m.Src.Validate(); err != nil {
			return err
		}
	}
	return nil
}
