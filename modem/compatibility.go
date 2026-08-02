package modem

import "slices"

type usbPortRule struct {
	interfaceNumber uint8
	portType        PortType
	role            PortRole
}

type usbPortLayout struct {
	vendorID   uint16
	productIDs []uint16
	ports      []usbPortRule
}

// usbPortLayouts contains only exact USB layouts that can be classified
// without talking to a serial port. Unknown products keep generic behavior.
var usbPortLayouts = []usbPortLayout{
	{
		vendorID:   0x2c7c,
		productIDs: []uint16{0x0121, 0x0125, 0x0191, 0x0195, 0x0296, 0x0306, 0x0512, 0x0800, 0x0801},
		ports: []usbPortRule{
			{interfaceNumber: 0, portType: PortQCDM},
			{interfaceNumber: 1, portType: PortGPS},
			{interfaceNumber: 2, portType: PortAT, role: PortRolePrimary},
			{interfaceNumber: 3, portType: PortAT, role: PortRoleSecondary},
		},
	},
	{
		vendorID:   0x2c7c,
		productIDs: []uint16{0x0128},
		ports: []usbPortRule{
			{interfaceNumber: 3, portType: PortQCDM},
			{interfaceNumber: 4, portType: PortGPS},
			{interfaceNumber: 5, portType: PortAT, role: PortRolePrimary},
		},
	},
	{
		vendorID:   0x2cb7,
		productIDs: []uint16{0x0007},
		ports: []usbPortRule{
			{interfaceNumber: 2, portType: PortAT, role: PortRolePrimary},
			{interfaceNumber: 4, portType: PortDebug},
			{interfaceNumber: 6, portType: PortAT, role: PortRoleSecondary},
		},
	},
	{
		vendorID:   0x2cb7,
		productIDs: []uint16{0x01a0},
		ports: []usbPortRule{
			{interfaceNumber: 2, portType: PortAT, role: PortRolePrimary},
			{interfaceNumber: 3, portType: PortAT, role: PortRoleSecondary},
			{interfaceNumber: 4, portType: PortDebug},
		},
	},
	{
		vendorID:   0x2cb7,
		productIDs: []uint16{0x0104},
		ports: []usbPortRule{
			{interfaceNumber: 0, portType: PortQCDM},
			{interfaceNumber: 1, portType: PortAT, role: PortRolePrimary},
			{interfaceNumber: 2, portType: PortAT, role: PortRoleSecondary},
			{interfaceNumber: 3, portType: PortUnknown},
		},
	},
	{
		vendorID:   0x2cb7,
		productIDs: []uint16{0x01a2},
		ports: []usbPortRule{
			{interfaceNumber: 2, portType: PortAT, role: PortRolePrimary},
			{interfaceNumber: 3, portType: PortUnknown},
			{interfaceNumber: 4, portType: PortDebug},
			{interfaceNumber: 5, portType: PortDebug},
		},
	},
	{
		vendorID:   0x1199,
		productIDs: []uint16{0x68c0, 0x9071, 0x9079, 0x9091},
		ports: []usbPortRule{
			{interfaceNumber: 0, portType: PortQCDM},
			{interfaceNumber: 2, portType: PortGPS},
			{interfaceNumber: 3, portType: PortAT, role: PortRolePrimary},
		},
	},
	{
		vendorID:   0x1199,
		productIDs: []uint16{0xc001},
		ports: []usbPortRule{
			{interfaceNumber: 0, portType: PortAT, role: PortRolePrimary},
			{interfaceNumber: 2, portType: PortAT, role: PortRolePPP},
			{interfaceNumber: 4, portType: PortGPS},
		},
	},
	{
		vendorID:   0x1bc7,
		productIDs: []uint16{0x1010},
		ports: []usbPortRule{
			{interfaceNumber: 1, portType: PortGPS},
			{interfaceNumber: 2, portType: PortAT, role: PortRoleSecondary},
			{interfaceNumber: 3, portType: PortAT, role: PortRolePrimary},
		},
	},
	{
		vendorID:   0x1bc7,
		productIDs: []uint16{0x1011},
		ports: []usbPortRule{
			{interfaceNumber: 1, portType: PortAT, role: PortRolePrimary},
		},
	},
	{
		vendorID:   0x1bc7,
		productIDs: []uint16{0x1031, 0x1033},
		ports: []usbPortRule{
			{interfaceNumber: 0, portType: PortQCDM},
			{interfaceNumber: 1, portType: PortAT, role: PortRolePrimary},
			{interfaceNumber: 2, portType: PortAT, role: PortRoleSecondary},
		},
	},
	{
		vendorID:   0x1bc7,
		productIDs: []uint16{0x1040},
		ports: []usbPortRule{
			{interfaceNumber: 0, portType: PortQCDM},
			{interfaceNumber: 3, portType: PortUnknown},
			{interfaceNumber: 4, portType: PortAT, role: PortRolePrimary},
			{interfaceNumber: 5, portType: PortAT, role: PortRoleSecondary},
			{interfaceNumber: 6, portType: PortUnknown},
		},
	},
	{
		vendorID:   0x1e0e,
		productIDs: []uint16{0xcefe},
		ports: []usbPortRule{
			{interfaceNumber: 0, portType: PortAT, role: PortRoleSecondary},
			{interfaceNumber: 1, portType: PortAT, role: PortRoleSecondary},
			{interfaceNumber: 2, portType: PortAT, role: PortRolePrimary},
		},
	},
	{
		vendorID:   0x1e0e,
		productIDs: []uint16{0x9100, 0x9200},
		ports: []usbPortRule{
			{interfaceNumber: 1, portType: PortAT, role: PortRoleSecondary},
			{interfaceNumber: 3, portType: PortAT, role: PortRolePrimary},
		},
	},
	{
		vendorID:   0x1e0e,
		productIDs: []uint16{0x9001},
		ports: []usbPortRule{
			{interfaceNumber: 0, portType: PortQCDM},
			{interfaceNumber: 1, portType: PortGPS},
			{interfaceNumber: 2, portType: PortAT, role: PortRolePrimary},
			{interfaceNumber: 3, portType: PortAT, role: PortRoleSecondary},
			{interfaceNumber: 4, portType: PortAudio},
		},
	},
	{
		vendorID:   0x1e0e,
		productIDs: []uint16{0x9205},
		ports: []usbPortRule{
			{interfaceNumber: 0, portType: PortQCDM},
			{interfaceNumber: 1, portType: PortGPS},
			{interfaceNumber: 2, portType: PortAT, role: PortRolePrimary},
			{interfaceNumber: 3, portType: PortAT, role: PortRolePPP},
		},
	},
	{
		vendorID:   0x1e0e,
		productIDs: []uint16{0x9206},
		ports: []usbPortRule{
			{interfaceNumber: 0, portType: PortQCDM},
			{interfaceNumber: 1, portType: PortGPS},
			{interfaceNumber: 2, portType: PortAT, role: PortRolePrimary},
			{interfaceNumber: 3, portType: PortUnknown},
			{interfaceNumber: 4, portType: PortUnknown},
			{interfaceNumber: 5, portType: PortAT, role: PortRolePPP},
		},
	},
	{
		vendorID:   0x1e0e,
		productIDs: []uint16{0x9011},
		ports: []usbPortRule{
			{interfaceNumber: 2, portType: PortUnknown},
			{interfaceNumber: 4, portType: PortAT, role: PortRolePrimary},
			{interfaceNumber: 5, portType: PortAT, role: PortRolePPP},
		},
	},
}

func applyStaticCompatibility(device Device) Device {
	if device.Bus != BusUSB {
		return device
	}
	for i := range device.Ports {
		port := &device.Ports[i]
		// Kernel-confirmed control protocols and network ports are authoritative.
		if (port.Type != PortUnknown && port.Type != PortAT) || port.Subsystem != "tty" || !port.USB.Valid {
			continue
		}
		rule, ok := matchUSBPortRule(device.USB, port.USB.Number)
		if !ok {
			continue
		}
		port.Type = rule.portType
		port.Role = rule.role
	}
	return normalizeDevice(device)
}

func matchUSBPortRule(identity USBIdentity, interfaceNumber uint8) (usbPortRule, bool) {
	for _, layout := range usbPortLayouts {
		if layout.vendorID != identity.VendorID || !slices.Contains(layout.productIDs, identity.ProductID) {
			continue
		}
		for _, rule := range layout.ports {
			if rule.interfaceNumber == interfaceNumber {
				return rule, true
			}
		}
		return usbPortRule{}, false
	}
	return usbPortRule{}, false
}
