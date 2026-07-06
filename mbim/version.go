package mbim

import (
	"context"
	"fmt"
)

func (r *Reader) negotiateVersion(ctx context.Context) error {
	r.mbimExVersion = mbimExVersion10

	services := DeviceServicesRequest{TransactionID: r.nextTransactionID()}
	if err := r.transmit(ctx, services.Request()); err != nil {
		return fmt.Errorf("negotiating MBIM version: reading device services: %w", err)
	}
	if !services.Response.SupportsCID(ServiceMsBasicConnectExtensions, CIDVersion) {
		return nil
	}

	version := VersionRequest{
		TransactionID: r.nextTransactionID(),
		MBIMVersion:   mbimVersion10,
		MBIMExVersion: hostMBIMExVersion,
	}
	if err := r.transmit(ctx, version.Request()); err != nil {
		return fmt.Errorf("negotiating MBIM version: %w", err)
	}
	r.mbimExVersion = min(version.Response.MBIMExVersion, hostMBIMExVersion)
	return nil
}

func (r *Reader) usesUiccSlotID() bool {
	return r.mbimExVersion >= mbimExVersion40
}

func (r *Reader) subscriberReadySlotID() uint32 {
	if r.usesUiccSlotID() {
		return r.slot
	}
	return activeSubscriberSlot
}
