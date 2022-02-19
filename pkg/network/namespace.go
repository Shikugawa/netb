// Copyright 2021 Rei Shimizu

// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at

//     http://www.apache.org/licenses/LICENSE-2.0

// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package network

import (
	"fmt"
	"net"
	"strings"

	"github.com/Shikugawa/ayame/pkg/config"
	log "github.com/sirupsen/logrus"
	"go.uber.org/multierr"
)

type RegisteredDeviceConfig struct {
	config.NamespaceDeviceConfig `json:"device_config"`
	Configured                   bool `json:"configured"`
}

type Namespace struct {
	Name                   string                   `json:"name"`
	Active                 bool                     `json:"is_active"`
	RegisteredDeviceConfig []RegisteredDeviceConfig `json:"registered_device_config"`
}

func InitNamespace(config *config.NamespaceConfig, dryrun bool) (*Namespace, error) {
	var configs []RegisteredDeviceConfig
	for _, c := range config.Devices {
		tmp := RegisteredDeviceConfig{
			Configured: false,
		}
		tmp.NamespaceDeviceConfig = c
		configs = append(configs, tmp)
	}

	ns := &Namespace{
		Name:                   config.Name,
		Active:                 false,
		RegisteredDeviceConfig: configs,
	}

	if err := RunIpNetnsAdd(config.Name, dryrun); err != nil {
		return nil, err
	}

	log.Infof("succeeded to create ns %s\n", config.Name)
	ns.Active = true
	return ns, nil
}

func (n *Namespace) Destroy(dryrun bool) error {
	if !n.Active {
		return fmt.Errorf("%s is already inactive\n", n.Name)
	}

	if err := RunIpNetnsDelete(n.Name, dryrun); err != nil {
		return err
	}

	log.Infof("succeeded to delete ns %s\n", n.Name)
	return nil
}

func (n *Namespace) Attach(veth *Veth, dryrun bool) error {
	if veth.Attached {
		return fmt.Errorf("device %s is already attached", veth.Name)
	}

	for idx, config := range n.RegisteredDeviceConfig {
		if !strings.HasPrefix(veth.Name, config.Name) {
			continue
		}

		if config.Configured {
			return fmt.Errorf("device %s has been attached to namexpace %s", config.NamespaceDeviceConfig.Name, n.Name)
		}

		_, _, err := net.ParseCIDR(config.Cidr)
		if err != nil {
			return fmt.Errorf("failed to parse CIDR %s in namespace %s device %s: %s\n", config.Cidr, n.Name, config.Name, err)
		}

		if err := RunIpLinkSetNamespaces(veth.Name, n.Name, dryrun); err != nil {
			return fmt.Errorf("failed to set device %s in namespace %s: %s", config.Name, n.Name, err)
		}

		if err := RunAssignCidrToNamespaces(veth.Name, n.Name, config.Cidr, dryrun); err != nil {
			return fmt.Errorf("failed to assign CIDR %s to ns %s on %s", config.Cidr, n.Name, veth.Name)
		}

		log.Infof("succeeded to attach CIDR %s to dev %s on ns %s\n", config.Cidr, veth.Name, n.Name)

		n.RegisteredDeviceConfig[idx].Configured = true
		veth.Attached = true
		break
	}

	return nil
}

func InitNamespaces(conf []*config.NamespaceConfig, dryrun bool) ([]*Namespace, error) {
	var namespaces []*Namespace

	// Setup namespaces
	for _, c := range conf {
		ns, err := InitNamespace(c, dryrun)
		if err != nil {
			return nil, err
		}

		namespaces = append(namespaces, ns)
	}

	return namespaces, nil
}

func InitNamespacesLinks(namespaces []*Namespace, links []*DirectLink, dryrun bool) error {
	netLinks := make(map[string][]int)

	for i, ns := range namespaces {
		for _, devConf := range ns.RegisteredDeviceConfig {
			if _, ok := netLinks[devConf.Name]; !ok {
				netLinks[devConf.Name] = []int{}
			}
			netLinks[devConf.Name] = append(netLinks[devConf.Name], i)
		}
	}

	// Configure netlinks
	findValidLinkIndex := func(name string) int {
		for i, link := range links {
			if name == link.Name {
				return i
			}
		}
		return -1
	}

	for linkName, idxs := range netLinks {
		if len(idxs) == 1 {
			return fmt.Errorf("%s have only 1 link in %s\n", linkName, namespaces[idxs[0]].Name)
		}

		if len(idxs) > 2 {
			return fmt.Errorf("%s has over 3 links despite it is not supported", linkName)
		}

		linkIdx := findValidLinkIndex(linkName)
		if linkIdx == -1 {
			return fmt.Errorf("can't find device %s in configured links", linkName)
		}

		targetLink := links[linkIdx]
		if err := targetLink.CreateLink(namespaces[idxs[0]], namespaces[idxs[1]], dryrun); err != nil {
			return fmt.Errorf("failed to create links %s: %s", linkName, err.Error())
		}
	}

	return nil
}

func CleanupNamespaces(nss []*Namespace, dryrun bool) error {
	var allerr error
	for _, n := range nss {
		if err := n.Destroy(dryrun); err != nil {
			allerr = multierr.Append(allerr, err)
		}
	}
	return allerr
}
