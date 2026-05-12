package output

import (
	"encoding/json"
	"fmt"
	"io"
)

func WriteJSON(w io.Writer, value any, mask bool) error {
	b, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	text := string(b)
	if mask {
		text = MaskPII(text)
	}
	_, err = fmt.Fprintln(w, text)
	return err
}

func WriteRawJSON(w io.Writer, raw []byte, mask bool) error {
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		text := string(raw)
		if mask {
			text = MaskPII(text)
		}
		_, err := fmt.Fprintln(w, text)
		return err
	}
	return WriteJSON(w, value, mask)
}
