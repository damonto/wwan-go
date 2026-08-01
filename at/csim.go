package at

import (
	"encoding/hex"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
)

type CSIMCommand []byte

func (c CSIMCommand) String() string {
	text, err := c.MarshalText()
	if err != nil {
		return ""
	}
	return string(text)
}

func (c CSIMCommand) MarshalBinary() ([]byte, error) {
	if c == nil {
		return nil, errors.New("encoding CSIM command: request is nil")
	}
	return slices.Clone(c), nil
}

func (c *CSIMCommand) UnmarshalBinary(data []byte) error {
	if data == nil {
		return errors.New("decoding CSIM command: request is nil")
	}
	*c = slices.Clone(data)
	return nil
}

func (c CSIMCommand) MarshalText() ([]byte, error) {
	if c == nil {
		return nil, errors.New("encoding CSIM command: request is nil")
	}
	body, err := csimData(c).MarshalText()
	if err != nil {
		return nil, err
	}
	return append([]byte("AT+CSIM="), body...), nil
}

func (c *CSIMCommand) UnmarshalText(text []byte) error {
	body, ok := strings.CutPrefix(strings.TrimSpace(string(text)), "AT+CSIM=")
	if !ok {
		return fmt.Errorf("invalid CSIM command: %q", text)
	}
	if !strings.Contains(body, ",") {
		return errors.New("invalid CSIM command: length separator is missing")
	}
	var data csimData
	if err := data.UnmarshalText([]byte(body)); err != nil {
		return fmt.Errorf("invalid CSIM command: %w", err)
	}
	*c = CSIMCommand(data)
	return nil
}

type CSIMResponse []byte

func (r CSIMResponse) String() string {
	return csimData(r).String()
}

func (r CSIMResponse) MarshalBinary() ([]byte, error) {
	return slices.Clone(r), nil
}

func (r *CSIMResponse) UnmarshalBinary(data []byte) error {
	*r = slices.Clone(data)
	return nil
}

func (r CSIMResponse) MarshalText() ([]byte, error) {
	return csimData(r).MarshalText()
}

func (r *CSIMResponse) UnmarshalText(text []byte) error {
	raw := string(text)
	for line := range strings.SplitSeq(raw, "\n") {
		line = strings.TrimSpace(line)
		if body, ok := strings.CutPrefix(line, "+CSIM:"); ok {
			return r.unmarshalBody(strings.TrimSpace(body), false)
		}
	}

	var err error
	for line := range strings.SplitSeq(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "+") {
			continue
		}
		if unmarshalErr := r.unmarshalBody(line, true); unmarshalErr == nil {
			return nil
		} else {
			err = errors.Join(err, fmt.Errorf("%q: %w", line, unmarshalErr))
		}
	}
	if err != nil {
		return fmt.Errorf("invalid CSIM response: %w", err)
	}

	return fmt.Errorf("invalid CSIM response: %q", raw)
}

func (r *CSIMResponse) unmarshalBody(body string, requireKnownStatusWord bool) error {
	var response csimData
	if err := response.UnmarshalText([]byte(body)); err != nil {
		return err
	}
	if requireKnownStatusWord {
		if len(response) < 2 {
			return errors.New("missing response status word")
		}
		if !hasKnownStatusWord(response) {
			return fmt.Errorf("unrecognized APDU status word: %X", response[len(response)-2:])
		}
	}
	*r = CSIMResponse(response)
	return nil
}

type csimData []byte

func (data csimData) String() string {
	text, _ := data.MarshalText()
	return string(text)
}

func (data csimData) MarshalText() ([]byte, error) {
	hexData := strings.ToUpper(hex.EncodeToString(data))
	return fmt.Appendf(nil, "%d,%q", len(hexData), hexData), nil
}

func (data *csimData) UnmarshalText(text []byte) error {
	body := string(text)
	hexData := body
	wantLen := -1
	lengthField, dataField, ok := strings.Cut(body, ",")
	if ok {
		lengthField = strings.TrimSpace(lengthField)
		n, err := strconv.Atoi(lengthField)
		if err != nil || n < 0 {
			return fmt.Errorf("invalid CSIM length %q", lengthField)
		}
		wantLen = n
		hexData = dataField
	}

	hexData = strings.TrimSpace(hexData)
	if strings.HasPrefix(hexData, `"`) {
		var err error
		hexData, err = strconv.Unquote(hexData)
		if err != nil {
			return fmt.Errorf("invalid CSIM data: %w", err)
		}
	}
	if wantLen >= 0 {
		if len(hexData) != wantLen {
			return fmt.Errorf("CSIM length mismatch: got %d, want %d", len(hexData), wantLen)
		}
	}

	decoded, err := hex.DecodeString(hexData)
	if err != nil {
		return fmt.Errorf("invalid CSIM data: %w", err)
	}
	*data = csimData(decoded)
	return nil
}

func hasKnownStatusWord(response []byte) bool {
	if len(response) < 2 {
		return false
	}
	sw1 := response[len(response)-2]
	return (sw1 >= 0x61 && sw1 <= 0x6F) || (sw1 >= 0x90 && sw1 <= 0x9F)
}
