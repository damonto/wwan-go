package qcom

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/damonto/wwan-go/qcom/tlv"
)

const (
	dmsTLVFirmwareList         = 0x01
	dmsTLVDownloadOverride     = 0x10
	dmsTLVModemStorageIndex    = 0x11
	dmsTLVMaximumBuildIDLength = 0x10
	dmsTLVBootVersion          = 0x10
	dmsTLVPRIVersion           = 0x11
	dmsTLVOEMLockID            = 0x12

	dmsFirmwareUniqueIDLength = 16
	dmsFirmwareBuildIDMax     = 255
	dmsFirmwareImageCountMax  = 255
)

// DMSFirmwareImageType identifies a modem or PRI firmware image.
type DMSFirmwareImageType uint8

const (
	DMSFirmwareImageModem DMSFirmwareImageType = iota
	DMSFirmwareImagePRI

	// Longer aliases mirror the names used by the QMI IDL documentation.
	DMSFirmwareImageTypeModem = DMSFirmwareImageModem
	DMSFirmwareImageTypePRI   = DMSFirmwareImagePRI
)

// DMSFirmwareImage identifies one firmware image preference.
type DMSFirmwareImage struct {
	Type     DMSFirmwareImageType
	UniqueID [dmsFirmwareUniqueIDLength]byte
	BuildID  string
}

// DMSFirmwarePreference contains the modem's selected firmware images.
type DMSFirmwarePreference struct {
	Images []DMSFirmwareImage
}

// DMSFirmwarePreferenceRequest contains a new firmware preference.
type DMSFirmwarePreferenceRequest struct {
	Images            []DMSFirmwareImage
	DownloadOverride  *bool
	ModemStorageIndex *uint8
}

// DMSSetFirmwarePreferenceRequest encodes DMS Set Firmware Preference.
type DMSSetFirmwarePreferenceRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
	Info          DMSFirmwarePreferenceRequest
}

// Request converts the request into a QMI DMS request.
func (r DMSSetFirmwarePreferenceRequest) Request() (Request, error) {
	list, err := encodeDMSFirmwareImageList(r.Info.Images)
	if err != nil {
		return Request{}, err
	}
	tlvs := tlv.TLVs{tlv.Bytes(dmsTLVFirmwareList, list)}
	if r.Info.DownloadOverride != nil {
		tlvs = append(tlvs, tlv.Uint(dmsTLVDownloadOverride, boolByte(*r.Info.DownloadOverride)))
	}
	if r.Info.ModemStorageIndex != nil {
		tlvs = append(tlvs, tlv.Uint(dmsTLVModemStorageIndex, *r.Info.ModemStorageIndex))
	}
	return Request{
		Service:       ServiceDMS,
		ClientID:      r.ClientID,
		TransactionID: r.TransactionID,
		MessageID:     MessageDMSSetFirmwarePreference,
		Timeout:       r.Timeout,
		TLVs:          tlvs,
	}, nil
}

// DMSGetFirmwarePreferenceRequest encodes DMS Get Firmware Preference.
type DMSGetFirmwarePreferenceRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
}

// Request converts the request into a QMI DMS request.
func (r DMSGetFirmwarePreferenceRequest) Request() Request {
	return dmsEmptyRequest(r.ClientID, r.TransactionID, r.Timeout, MessageDMSGetFirmwarePreference)
}

// DMSGetFirmwarePreferenceResponse is the parsed firmware preference.
type DMSGetFirmwarePreferenceResponse struct {
	Images []DMSFirmwareImage
}

// UnmarshalTLVs parses a QMI DMS Get Firmware Preference response.
func (r *DMSGetFirmwarePreferenceResponse) UnmarshalTLVs(tlvs tlv.TLVs) error {
	*r = DMSGetFirmwarePreferenceResponse{}
	value, ok := tlv.Value(tlvs, dmsTLVFirmwareList)
	if !ok {
		return errors.New("parsing QMI DMS firmware preference: image list TLV missing")
	}
	images, err := decodeDMSFirmwareImageList(value)
	if err != nil {
		return err
	}
	r.Images = images
	return nil
}

// DMSSetFirmwarePreferenceResponse contains image types selected for download.
type DMSSetFirmwarePreferenceResponse struct {
	ImageDownloadList      []DMSFirmwareImageType
	ImageDownloadListKnown bool
	MaximumBuildIDLength   uint8
	MaximumBuildIDKnown    bool
}

// UnmarshalTLVs parses a QMI DMS Set Firmware Preference response.
func (r *DMSSetFirmwarePreferenceResponse) UnmarshalTLVs(tlvs tlv.TLVs) error {
	*r = DMSSetFirmwarePreferenceResponse{}
	value, ok := tlv.Value(tlvs, dmsTLVFirmwareList)
	if !ok {
		return r.unmarshalMaximumBuildID(tlvs)
	}
	if len(value) < 1 {
		return errors.New("parsing QMI DMS firmware preference result: image count is missing")
	}
	count := int(value[0])
	if count > dmsFirmwareImageCountMax {
		return fmt.Errorf("parsing QMI DMS firmware preference result: image count %d exceeds %d", count, dmsFirmwareImageCountMax)
	}
	if len(value) != 1+count {
		return fmt.Errorf("parsing QMI DMS firmware preference result: TLV length %d, want %d", len(value), 1+count)
	}
	r.ImageDownloadList = make([]DMSFirmwareImageType, count)
	for i := range count {
		if err := validateDMSFirmwareImageType(value[1+i]); err != nil {
			return fmt.Errorf("parsing QMI DMS firmware preference result image %d: %w", i, err)
		}
		r.ImageDownloadList[i] = DMSFirmwareImageType(value[1+i])
	}
	r.ImageDownloadListKnown = true
	return r.unmarshalMaximumBuildID(tlvs)
}

func (r *DMSSetFirmwarePreferenceResponse) unmarshalMaximumBuildID(tlvs tlv.TLVs) error {
	value, ok := tlv.Value(tlvs, dmsTLVMaximumBuildIDLength)
	if !ok {
		return nil
	}
	if len(value) != 1 {
		return fmt.Errorf("parsing QMI DMS firmware preference result: maximum build ID TLV length %d, want 1", len(value))
	}
	r.MaximumBuildIDLength = value[0]
	r.MaximumBuildIDKnown = true
	return nil
}

// FirmwarePreference returns the modem's selected firmware images.
func (c *Client) FirmwarePreference(ctx context.Context) (DMSFirmwarePreference, error) {
	var result DMSGetFirmwarePreferenceResponse
	if err := c.dmsRead(ctx, MessageDMSGetFirmwarePreference, &result); err != nil {
		return DMSFirmwarePreference{}, fmt.Errorf("querying QMI DMS firmware preference: %w", err)
	}
	return DMSFirmwarePreference{Images: result.Images}, nil
}

// GetFirmwarePreference is an alias for FirmwarePreference.
func (c *Client) GetFirmwarePreference(ctx context.Context) (DMSFirmwarePreference, error) {
	return c.FirmwarePreference(ctx)
}

// SetFirmwarePreference selects the modem and PRI images used for download.
func (c *Client) SetFirmwarePreference(ctx context.Context, req DMSFirmwarePreferenceRequest) (DMSSetFirmwarePreferenceResponse, error) {
	request, err := (DMSSetFirmwarePreferenceRequest{
		Timeout: DefaultRequestTimeout,
		Info:    req,
	}).Request()
	if err != nil {
		return DMSSetFirmwarePreferenceResponse{}, fmt.Errorf("setting QMI DMS firmware preference: %w", err)
	}
	var result DMSSetFirmwarePreferenceResponse
	err = c.withServiceClient(ctx, ServiceDMS, func(clientID uint8) error {
		resp, err := c.requestServiceWithTimeout(ctx, ServiceDMS, clientID, request.MessageID, request.TLVs, request.Timeout)
		if err != nil {
			return err
		}
		if err := resultOK(resp); err != nil {
			return err
		}
		return result.UnmarshalTLVs(resp.TLVs)
	})
	if err != nil {
		return DMSSetFirmwarePreferenceResponse{}, fmt.Errorf("setting QMI DMS firmware preference: %w", err)
	}
	return result, nil
}

// DMSStoredImage identifies one image stored on the modem.
type DMSStoredImage struct {
	StorageIndex uint8
	FailureCount uint8
	UniqueID     [dmsFirmwareUniqueIDLength]byte
	BuildID      string
}

// DMSStoredImageSet groups stored images of one firmware type.
type DMSStoredImageSet struct {
	Type              DMSFirmwareImageType
	MaximumImages     uint8
	RunningImageIndex uint8
	Images            []DMSStoredImage
}

// DMSStoredImages contains all stored image groups.
type DMSStoredImages struct {
	Images []DMSStoredImageSet
}

// DMSListStoredImagesRequest encodes DMS List Stored Images.
type DMSListStoredImagesRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
}

// Request converts the request into a QMI DMS request.
func (r DMSListStoredImagesRequest) Request() Request {
	return dmsEmptyRequest(r.ClientID, r.TransactionID, r.Timeout, MessageDMSListStoredImages)
}

// DMSListStoredImagesResponse is the parsed stored image list.
type DMSListStoredImagesResponse struct {
	Images []DMSStoredImageSet
}

// UnmarshalTLVs parses a QMI DMS List Stored Images response.
func (r *DMSListStoredImagesResponse) UnmarshalTLVs(tlvs tlv.TLVs) error {
	*r = DMSListStoredImagesResponse{}
	value, ok := tlv.Value(tlvs, dmsTLVFirmwareList)
	if !ok {
		return errors.New("parsing QMI DMS stored images: image list TLV missing")
	}
	sets, err := decodeDMSStoredImageSets(value)
	if err != nil {
		return err
	}
	r.Images = sets
	return nil
}

// StoredImages returns all firmware images stored on the modem.
func (c *Client) StoredImages(ctx context.Context) (DMSStoredImages, error) {
	var result DMSListStoredImagesResponse
	if err := c.dmsRead(ctx, MessageDMSListStoredImages, &result); err != nil {
		return DMSStoredImages{}, fmt.Errorf("querying QMI DMS stored images: %w", err)
	}
	return DMSStoredImages{Images: result.Images}, nil
}

// ListStoredImages is an alias for StoredImages.
func (c *Client) ListStoredImages(ctx context.Context) (DMSStoredImages, error) {
	return c.StoredImages(ctx)
}

// DMSDeleteStoredImageRequest identifies a stored image to delete.
type DMSDeleteStoredImageRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
	Image         DMSFirmwareImage
}

// Request validates and converts the image identifier into a QMI request.
func (r DMSDeleteStoredImageRequest) Request() (Request, error) {
	value, err := r.Image.MarshalBinary()
	if err != nil {
		return Request{}, err
	}
	return Request{
		Service:       ServiceDMS,
		ClientID:      r.ClientID,
		TransactionID: r.TransactionID,
		MessageID:     MessageDMSDeleteStoredImage,
		Timeout:       r.Timeout,
		TLVs:          tlv.TLVs{tlv.Bytes(dmsTLVFirmwareList, value)},
	}, nil
}

// DeleteStoredImage removes one firmware image from modem storage.
func (c *Client) DeleteStoredImage(ctx context.Context, image DMSFirmwareImage) error {
	request, err := (DMSDeleteStoredImageRequest{
		Timeout: DefaultRequestTimeout,
		Image:   image,
	}).Request()
	if err != nil {
		return fmt.Errorf("deleting QMI DMS stored image: %w", err)
	}
	if err := c.dmsReadRequest(ctx, request, nil); err != nil {
		return fmt.Errorf("deleting QMI DMS stored image: %w", err)
	}
	return nil
}

// DMSStoredImageInfoRequest identifies an image for detailed information.
type DMSStoredImageInfoRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
	Image         DMSFirmwareImage
}

// Request converts the request into a QMI DMS request.
func (r DMSStoredImageInfoRequest) Request() (Request, error) {
	value, err := r.Image.MarshalBinary()
	if err != nil {
		return Request{}, err
	}
	return Request{
		Service:       ServiceDMS,
		ClientID:      r.ClientID,
		TransactionID: r.TransactionID,
		MessageID:     MessageDMSGetStoredImageInfo,
		Timeout:       r.Timeout,
		TLVs:          tlv.TLVs{tlv.Bytes(dmsTLVFirmwareList, value)},
	}, nil
}

// DMSStoredImageInfo contains optional details for one stored image.
type DMSStoredImageInfo struct {
	BootMajorVersion uint16
	BootMinorVersion uint16
	BootVersionKnown bool
	PRIVersion       uint32
	PRIInfo          string
	PRIVersionKnown  bool
	OEMLockID        uint32
	OEMLockIDKnown   bool
}

// UnmarshalTLVs parses a QMI DMS Get Stored Image Info response.
func (r *DMSStoredImageInfo) UnmarshalTLVs(tlvs tlv.TLVs) error {
	*r = DMSStoredImageInfo{}
	if value, ok := tlv.Value(tlvs, dmsTLVBootVersion); ok {
		if len(value) != 4 {
			return fmt.Errorf("parsing QMI DMS stored image info: boot version TLV length %d, want 4", len(value))
		}
		r.BootMajorVersion = binary.LittleEndian.Uint16(value)
		r.BootMinorVersion = binary.LittleEndian.Uint16(value[2:])
		r.BootVersionKnown = true
	}
	if value, ok := tlv.Value(tlvs, dmsTLVPRIVersion); ok {
		if len(value) != 36 {
			return fmt.Errorf("parsing QMI DMS stored image info: PRI version TLV length %d, want 36", len(value))
		}
		r.PRIVersion = binary.LittleEndian.Uint32(value)
		r.PRIInfo = strings.TrimRight(string(value[4:]), "\x00")
		r.PRIVersionKnown = true
	}
	if value, ok := tlv.Value(tlvs, dmsTLVOEMLockID); ok {
		if len(value) != 4 {
			return fmt.Errorf("parsing QMI DMS stored image info: OEM lock ID TLV length %d, want 4", len(value))
		}
		r.OEMLockID = binary.LittleEndian.Uint32(value)
		r.OEMLockIDKnown = true
	}
	return nil
}

// StoredImageInfo returns details for one stored firmware image.
func (c *Client) StoredImageInfo(ctx context.Context, image DMSFirmwareImage) (DMSStoredImageInfo, error) {
	request, err := (DMSStoredImageInfoRequest{
		Timeout: DefaultRequestTimeout,
		Image:   image,
	}).Request()
	if err != nil {
		return DMSStoredImageInfo{}, fmt.Errorf("querying QMI DMS stored image info: %w", err)
	}
	var result DMSStoredImageInfo
	err = c.withServiceClient(ctx, ServiceDMS, func(clientID uint8) error {
		resp, err := c.requestServiceWithTimeout(ctx, ServiceDMS, clientID, request.MessageID, request.TLVs, request.Timeout)
		if err != nil {
			return err
		}
		if err := resultOK(resp); err != nil {
			return err
		}
		return result.UnmarshalTLVs(resp.TLVs)
	})
	if err != nil {
		return DMSStoredImageInfo{}, fmt.Errorf("querying QMI DMS stored image info: %w", err)
	}
	return result, nil
}

// DMSBootImageDownloadMode selects the modem's next boot mode.
type DMSBootImageDownloadMode uint8

const (
	DMSBootImageDownloadNormal           DMSBootImageDownloadMode = 0
	DMSBootImageDownloadBootAndRecovery  DMSBootImageDownloadMode = 1
	DMSBootImageDownloadModeNormal                                = DMSBootImageDownloadNormal
	DMSBootImageDownloadModeBootRecovery                          = DMSBootImageDownloadBootAndRecovery
)

// DMSSetBootImageDownloadModeRequest encodes DMS Set Boot Image Download Mode.
type DMSSetBootImageDownloadModeRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
	Mode          DMSBootImageDownloadMode
}

// Request converts the request into a QMI DMS request.
func (r DMSSetBootImageDownloadModeRequest) Request() (Request, error) {
	if r.Mode > DMSBootImageDownloadBootAndRecovery {
		return Request{}, fmt.Errorf("invalid DMS boot image download mode %d", r.Mode)
	}
	return Request{
		Service:       ServiceDMS,
		ClientID:      r.ClientID,
		TransactionID: r.TransactionID,
		MessageID:     MessageDMSSetBootImageDownloadMode,
		Timeout:       r.Timeout,
		TLVs:          tlv.TLVs{tlv.Uint(0x01, uint8(r.Mode))},
	}, nil
}

// DMSGetBootImageDownloadModeRequest encodes DMS Get Boot Image Download Mode.
type DMSGetBootImageDownloadModeRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
}

// Request converts the request into a QMI DMS request.
func (r DMSGetBootImageDownloadModeRequest) Request() Request {
	return dmsEmptyRequest(r.ClientID, r.TransactionID, r.Timeout, MessageDMSGetBootImageDownloadMode)
}

// BootImageDownloadMode returns the modem's current next-boot mode.
func (c *Client) BootImageDownloadMode(ctx context.Context) (DMSBootImageDownloadMode, error) {
	var result DMSBootImageDownloadMode
	if err := c.dmsRead(ctx, MessageDMSGetBootImageDownloadMode, &result); err != nil {
		return 0, fmt.Errorf("querying QMI DMS boot image download mode: %w", err)
	}
	return result, nil
}

// UnmarshalTLVs parses the QMI DMS boot image download mode.
func (mode *DMSBootImageDownloadMode) UnmarshalTLVs(tlvs tlv.TLVs) error {
	*mode = DMSBootImageDownloadNormal
	value, ok := tlv.Value(tlvs, dmsTLVBootVersion)
	if !ok {
		return errors.New("parsing QMI DMS boot image download mode: mode TLV missing")
	}
	if len(value) != 1 {
		return fmt.Errorf("parsing QMI DMS boot image download mode: mode TLV length %d, want 1", len(value))
	}
	if value[0] > byte(DMSBootImageDownloadBootAndRecovery) {
		return fmt.Errorf("parsing QMI DMS boot image download mode: invalid mode %d", value[0])
	}
	*mode = DMSBootImageDownloadMode(value[0])
	return nil
}

// SetBootImageDownloadMode sets the modem's next-boot mode.
func (c *Client) SetBootImageDownloadMode(ctx context.Context, mode DMSBootImageDownloadMode) error {
	request, err := (DMSSetBootImageDownloadModeRequest{
		Timeout: DefaultRequestTimeout,
		Mode:    mode,
	}).Request()
	if err != nil {
		return fmt.Errorf("setting QMI DMS boot image download mode: %w", err)
	}
	if err := c.dmsReadRequest(ctx, request, nil); err != nil {
		return fmt.Errorf("setting QMI DMS boot image download mode: %w", err)
	}
	return nil
}

// DMSSetFirmwareIDRequest encodes DMS Set Firmware ID.
type DMSSetFirmwareIDRequest struct {
	ClientID      uint8
	TransactionID uint16
	Timeout       time.Duration
}

// Request converts the request into a QMI DMS request.
func (r DMSSetFirmwareIDRequest) Request() Request {
	return dmsEmptyRequest(r.ClientID, r.TransactionID, r.Timeout, MessageDMSSetFirmwareID)
}

// SetFirmwareID asks the modem to persist its current firmware ID.
func (c *Client) SetFirmwareID(ctx context.Context) error {
	if err := c.dmsReadRequest(ctx, DMSSetFirmwareIDRequest{Timeout: DefaultRequestTimeout}.Request(), nil); err != nil {
		return fmt.Errorf("setting QMI DMS firmware ID: %w", err)
	}
	return nil
}

func encodeDMSFirmwareImageList(images []DMSFirmwareImage) ([]byte, error) {
	if len(images) > dmsFirmwareImageCountMax {
		return nil, fmt.Errorf("firmware image count %d exceeds %d", len(images), dmsFirmwareImageCountMax)
	}
	value := []byte{byte(len(images))}
	for i, image := range images {
		encoded, err := image.MarshalBinary()
		if err != nil {
			return nil, fmt.Errorf("encoding firmware image %d: %w", i, err)
		}
		value = append(value, encoded...)
	}
	return value, nil
}

// MarshalBinary encodes one QMI DMS firmware image descriptor.
func (image DMSFirmwareImage) MarshalBinary() ([]byte, error) {
	if err := validateDMSFirmwareImageType(byte(image.Type)); err != nil {
		return nil, err
	}
	if err := validateDMSFirmwareString(image.BuildID); err != nil {
		return nil, fmt.Errorf("validating firmware build ID: %w", err)
	}
	value := make([]byte, 0, 1+dmsFirmwareUniqueIDLength+1+len(image.BuildID))
	value = append(value, byte(image.Type))
	value = append(value, image.UniqueID[:]...)
	value = append(value, byte(len(image.BuildID)))
	return append(value, image.BuildID...), nil
}

// UnmarshalBinary decodes one QMI DMS firmware image descriptor.
func (image *DMSFirmwareImage) UnmarshalBinary(data []byte) error {
	decoded, rest, err := decodeDMSFirmwareImage(data)
	if err != nil {
		return err
	}
	if len(rest) != 0 {
		return fmt.Errorf("parsing firmware image: %d trailing bytes", len(rest))
	}
	*image = decoded
	return nil
}

func decodeDMSFirmwareImageList(value []byte) ([]DMSFirmwareImage, error) {
	if len(value) < 1 {
		return nil, errors.New("parsing QMI DMS firmware image list: image count is missing")
	}
	count := int(value[0])
	if count > dmsFirmwareImageCountMax {
		return nil, fmt.Errorf("parsing QMI DMS firmware image list: image count %d exceeds %d", count, dmsFirmwareImageCountMax)
	}
	value = value[1:]
	images := make([]DMSFirmwareImage, 0, count)
	for i := range count {
		image, rest, err := decodeDMSFirmwareImage(value)
		if err != nil {
			return nil, fmt.Errorf("parsing QMI DMS firmware image %d: %w", i, err)
		}
		images = append(images, image)
		value = rest
	}
	if len(value) != 0 {
		return nil, fmt.Errorf("parsing QMI DMS firmware image list: %d trailing bytes", len(value))
	}
	return images, nil
}

func decodeDMSFirmwareImage(value []byte) (DMSFirmwareImage, []byte, error) {
	minimum := 1 + dmsFirmwareUniqueIDLength + 1
	if len(value) < minimum {
		return DMSFirmwareImage{}, nil, errors.New("image entry is truncated")
	}
	if err := validateDMSFirmwareImageType(value[0]); err != nil {
		return DMSFirmwareImage{}, nil, err
	}
	var image DMSFirmwareImage
	image.Type = DMSFirmwareImageType(value[0])
	copy(image.UniqueID[:], value[1:1+dmsFirmwareUniqueIDLength])
	buildLength := int(value[1+dmsFirmwareUniqueIDLength])
	start := minimum
	if len(value) < start+buildLength {
		return DMSFirmwareImage{}, nil, errors.New("build ID is truncated")
	}
	image.BuildID = string(value[start : start+buildLength])
	return image, value[start+buildLength:], nil
}

func decodeDMSStoredImageSets(value []byte) ([]DMSStoredImageSet, error) {
	if len(value) < 1 {
		return nil, errors.New("parsing QMI DMS stored images: image count is missing")
	}
	count := int(value[0])
	value = value[1:]
	sets := make([]DMSStoredImageSet, 0, count)
	for i := range count {
		if len(value) < 4 {
			return nil, fmt.Errorf("parsing QMI DMS stored images: image group %d header is truncated", i)
		}
		if err := validateDMSFirmwareImageType(value[0]); err != nil {
			return nil, fmt.Errorf("parsing QMI DMS stored images group %d: %w", i, err)
		}
		set := DMSStoredImageSet{
			Type:              DMSFirmwareImageType(value[0]),
			MaximumImages:     value[1],
			RunningImageIndex: value[2],
		}
		subCount := int(value[3])
		value = value[4:]
		set.Images = make([]DMSStoredImage, 0, subCount)
		for j := range subCount {
			if len(value) < 3+dmsFirmwareUniqueIDLength+1 {
				return nil, fmt.Errorf("parsing QMI DMS stored images group %d image %d is truncated", i, j)
			}
			image := DMSStoredImage{
				StorageIndex: value[0],
				FailureCount: value[1],
			}
			copy(image.UniqueID[:], value[2:2+dmsFirmwareUniqueIDLength])
			buildLength := int(value[2+dmsFirmwareUniqueIDLength])
			start := 3 + dmsFirmwareUniqueIDLength
			if len(value) < start+buildLength {
				return nil, fmt.Errorf("parsing QMI DMS stored images group %d image %d build ID is truncated", i, j)
			}
			image.BuildID = string(value[start : start+buildLength])
			set.Images = append(set.Images, image)
			value = value[start+buildLength:]
		}
		sets = append(sets, set)
	}
	if len(value) != 0 {
		return nil, fmt.Errorf("parsing QMI DMS stored images: %d trailing bytes", len(value))
	}
	return sets, nil
}

func validateDMSFirmwareImageType(value byte) error {
	if value > byte(DMSFirmwareImagePRI) {
		return fmt.Errorf("invalid firmware image type %d", value)
	}
	return nil
}

func validateDMSFirmwareString(value string) error {
	if len(value) > dmsFirmwareBuildIDMax {
		return fmt.Errorf("length %d exceeds %d", len(value), dmsFirmwareBuildIDMax)
	}
	for i := range len(value) {
		if value[i] > 0x7F {
			return errors.New("contains a non-ASCII character")
		}
	}
	return nil
}
