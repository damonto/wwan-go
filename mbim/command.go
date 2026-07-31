package mbim

import (
	"fmt"
	"slices"
	"time"
)

const (
	mbimCIDResponseTimeout     = 8 * time.Second
	mbimCIDLongResponseTimeout = 58 * time.Second
)

func command(serviceID [16]byte, commandID uint32, commandType CommandType, data []byte) *Command {
	return &Command{
		FragmentTotal:   1,
		FragmentCurrent: 0,
		ServiceID:       serviceID,
		CommandID:       commandID,
		CommandType:     commandType,
		Data:            slices.Clone(data),
	}
}

func commandWithError(serviceID [16]byte, commandID uint32, commandType CommandType, data []byte, err error) *Command {
	command := command(serviceID, commandID, commandType, data)
	if err != nil {
		command.marshalErr = fmt.Errorf("encoding command payload: %w", err)
	}
	return command
}
