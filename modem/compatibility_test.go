package modem

import (
	"fmt"
	"testing"
)

func TestApplyStaticCompatibility(t *testing.T) {
	type disposition struct {
		portType PortType
		role     PortRole
	}
	tests := []struct {
		name     string
		identity USBIdentity
		ports    []Port
		want     map[string]disposition
	}{
		{
			name:     "Quectel layout",
			identity: USBIdentity{VendorID: 0x2c7c, ProductID: 0x0306},
			ports: []Port{
				{Name: "diag", Type: PortAT, Subsystem: "tty", USB: USBInterface{Valid: true, Number: 0}},
				{Name: "gps", Type: PortAT, Subsystem: "tty", USB: USBInterface{Valid: true, Number: 1}},
				{Name: "primary", Type: PortAT, Subsystem: "tty", USB: USBInterface{Valid: true, Number: 2}},
				{Name: "secondary", Type: PortAT, Subsystem: "tty", USB: USBInterface{Valid: true, Number: 3}},
			},
			want: map[string]disposition{
				"diag":      {portType: PortQCDM},
				"gps":       {portType: PortGPS},
				"primary":   {portType: PortAT, role: PortRolePrimary},
				"secondary": {portType: PortAT, role: PortRoleSecondary},
			},
		},
		{
			name:     "Fibocom debug port",
			identity: USBIdentity{VendorID: 0x2cb7, ProductID: 0x01a0},
			ports: []Port{
				{Name: "debug", Type: PortAT, Subsystem: "tty", USB: USBInterface{Valid: true, Number: 4}},
			},
			want: map[string]disposition{"debug": {portType: PortDebug}},
		},
		{
			name:     "explicitly ignored port is not treated as AT",
			identity: USBIdentity{VendorID: 0x2cb7, ProductID: 0x01a2},
			ports: []Port{
				{Name: "ignored", Type: PortAT, Subsystem: "tty", USB: USBInterface{Valid: true, Number: 3}},
			},
			want: map[string]disposition{"ignored": {portType: PortUnknown}},
		},
		{
			name:     "unknown product keeps generic AT",
			identity: USBIdentity{VendorID: 0x2c7c, ProductID: 0xffff},
			ports: []Port{
				{Name: "unknown", Type: PortAT, Subsystem: "tty", USB: USBInterface{Valid: true, Number: 2}},
			},
			want: map[string]disposition{"unknown": {portType: PortAT}},
		},
		{
			name:     "kernel types are authoritative",
			identity: USBIdentity{VendorID: 0x2c7c, ProductID: 0x0306},
			ports: []Port{
				{Name: "qmi", Type: PortQMI, Subsystem: "tty", USB: USBInterface{Valid: true, Number: 1}},
				{Name: "mbim", Type: PortMBIM, Subsystem: "tty", USB: USBInterface{Valid: true, Number: 2}},
				{Name: "network", Type: PortNetwork, Subsystem: "tty", USB: USBInterface{Valid: true, Number: 3}},
			},
			want: map[string]disposition{
				"qmi":     {portType: PortQMI},
				"mbim":    {portType: PortMBIM},
				"network": {portType: PortNetwork},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			device := applyStaticCompatibility(Device{Bus: BusUSB, USB: tt.identity, Ports: tt.ports})
			if len(device.Ports) != len(tt.want) {
				t.Fatalf("len(Ports) = %d, want %d", len(device.Ports), len(tt.want))
			}
			for _, port := range device.Ports {
				want, ok := tt.want[port.Name]
				if !ok {
					t.Fatalf("unexpected port %q", port.Name)
				}
				if port.Type != want.portType || port.Role != want.role {
					t.Errorf("port %q disposition = %d/%d, want %d/%d", port.Name, port.Type, port.Role, want.portType, want.role)
				}
			}
		})
	}
}

func TestUSBPortLayoutsHaveUniqueInterfaces(t *testing.T) {
	products := make(map[USBIdentity]struct{})
	for _, layout := range usbPortLayouts {
		for _, productID := range layout.productIDs {
			name := fmt.Sprintf("%04x:%04x", layout.vendorID, productID)
			t.Run(name, func(t *testing.T) {
				identity := USBIdentity{VendorID: layout.vendorID, ProductID: productID}
				if _, ok := products[identity]; ok {
					t.Fatalf("product has multiple layouts")
				}
				products[identity] = struct{}{}

				seen := make(map[uint8]struct{}, len(layout.ports))
				for _, port := range layout.ports {
					if _, ok := seen[port.interfaceNumber]; ok {
						t.Fatalf("interface %d has multiple rules", port.interfaceNumber)
					}
					if port.portType == PortQMI || port.portType == PortMBIM || port.portType == PortNetwork {
						t.Fatalf("interface %d assigns kernel-owned port type %d", port.interfaceNumber, port.portType)
					}
					seen[port.interfaceNumber] = struct{}{}
				}
			})
		}
	}
}
