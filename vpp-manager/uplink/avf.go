// Copyright (C) 2020 Cisco Systems Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//    http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or
// implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package uplink

import (
	"fmt"
	"github.com/pkg/errors"
	"github.com/projectcalico/vpp-dataplane/vpp-manager/config"
	"github.com/projectcalico/vpp-dataplane/vpp-manager/utils"
	"github.com/projectcalico/vpp-dataplane/vpplink"
	"github.com/projectcalico/vpp-dataplane/vpplink/types"
	log "github.com/sirupsen/logrus"
	"github.com/vishvananda/netlink"
)

type AVFDriver struct {
	UplinkDriverData
	pciId string
}

func (d *AVFDriver) IsSupported(warn bool) (supported bool) {
	var ret bool
	supported = true
	ret = d.params.LoadedDrivers[config.DRIVER_VFIO_PCI]
	if !ret && warn {
		log.Warnf("did not find vfio-pci or uio_pci_generic driver")
		log.Warnf("VPP may fail to grab its interface")
	}
	supported = supported && ret

	ret = d.conf.Driver == config.DRIVER_I40E
	if !ret && warn {
		log.Warnf("Interface driver is <%s>, not %s", d.conf.Driver, config.DRIVER_I40E)
	}
	supported = supported && ret

	return supported
}

func (d *AVFDriver) PreconfigureLinux() (err error) {
	pciId, err := utils.CreateInterfaceVF(d.params.MainInterface)
	if err != nil {
		return errors.Wrapf(err, "Couldnt create Interface VF")
	}
	d.pciId = pciId

	link, err := netlink.LinkByName(d.params.MainInterface)
	if err != nil {
		return errors.Wrapf(err, "Couldnt find Interface %s", d.params.MainInterface)
	}
	netlink.LinkSetVfHardwareAddr(link, 0 /* vf */, d.conf.HardwareAddr)

	if d.conf.IsUp {
		// Set interface down if it is up, bind it to a VPP-friendly driver
		err := utils.SafeSetInterfaceDownByName(d.params.MainInterface)
		if err != nil {
			return err
		}
	}
	return nil
}

func (d *AVFDriver) RestoreLinux() {
	if !d.conf.IsUp {
		return
	}
	// This assumes the link has kept the same name after the rebind.
	// It should be always true on systemd based distros
	link, err := utils.SafeSetInterfaceUpByName(d.params.MainInterface)
	if err != nil {
		log.Warnf("Error setting %s up: %v", d.params.MainInterface, err)
		return
	}

	// Re-add all adresses and routes
	d.restoreLinuxIfConf(link)
}

func (d *AVFDriver) CreateMainVppInterface(vpp *vpplink.VppLink) error {
	swIfIndex, err := vpp.CreateAVF(&types.AVFInterface{
		NumRxQueues: d.params.NumRxQueues,
		TxQueueSize: d.params.TxQueueSize,
		RxQueueSize: d.params.RxQueueSize,
		PciId:       d.pciId,
	})
	if err != nil {
		return errors.Wrapf(err, "Error creating AVF interface")
	}
	log.Infof("Created AVF interface %d", swIfIndex)

	if swIfIndex != config.DataInterfaceSwIfIndex {
		return fmt.Errorf("Created AVF interface has wrong swIfIndex %d!", swIfIndex)
	}
	return nil
}

func NewAVFDriver(params *config.VppManagerParams, conf *config.InterfaceConfig) *AVFDriver {
	d := &AVFDriver{}
	d.name = NATIVE_DRIVER_AVF
	d.conf = conf
	d.params = params
	return d
}
