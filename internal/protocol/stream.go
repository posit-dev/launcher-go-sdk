package protocol

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
)

// DefaultMaxMsgSize is the default maximum message size in bytes (5MB).
const DefaultMaxMsgSize = 5242880

var (
	// ErrMsgTooLarge is returned when a message exceeds the maximum message size.
	ErrMsgTooLarge = fmt.Errorf("message exceeds maximum message size")
	// ErrMsgInvalid is returned when a message is invalid.
	ErrMsgInvalid = fmt.Errorf("invalid message")
)

// Encoder is a streaming encoder for Launcher's JSON-based wire protocol.
type Encoder struct {
	w       io.Writer
	maxSize int
}

// NewEncoder creates a new streaming encoder for Launcher's JSON-based wire
// protocol.
func NewEncoder(w io.Writer, maxSize int) *Encoder {
	return &Encoder{w: w, maxSize: maxSize}
}

// Encode the given message into the stream.
func (e *Encoder) Encode(msg interface{}) error {
	buf, err := json.Marshal(msg)
	if err != nil {
		return ErrMsgInvalid
	}
	return e.write(buf)
}

// EncodeRaw encodes a raw JSON message into the stream.
func (e *Encoder) EncodeRaw(msg json.RawMessage) error {
	if !json.Valid(msg) {
		return ErrMsgInvalid
	}
	return e.write(msg)
}

func (e *Encoder) write(msg []byte) error {
	if len(msg) > e.maxSize {
		return ErrMsgTooLarge
	}
	if len(msg) > int(^uint32(0)) {
		return ErrMsgTooLarge
	}
	header := uint32(len(msg)) //nolint:gosec // bounds checked above
	if err := binary.Write(e.w, binary.BigEndian, header); err != nil {
		return err
	}
	if _, err := e.w.Write(msg); err != nil {
		return err
	}
	return nil
}

// Decoder is a streaming decoder for Launcher's JSON-based wire protocol.
type Decoder struct {
	r         io.Reader
	headerBuf [4]byte
	buf       []byte
	msgLen    uint32
	err       error
}

// NewDecoder creates a new streaming decoder for Launcher's JSON-based wire
// protocol.
func NewDecoder(r io.Reader, maxSize int) *Decoder {
	return &Decoder{r: r, buf: make([]byte, maxSize)}
}

// More consumes the next message in the stream or returns false if there is
// none.
func (d *Decoder) More() bool {
	if d.err != nil {
		return false
	}
	d.msgLen = 0
	if _, d.err = io.ReadFull(d.r, d.headerBuf[:]); d.err != nil {
		return false
	}
	d.msgLen = binary.BigEndian.Uint32(d.headerBuf[:])
	if int(d.msgLen) > len(d.buf) {
		d.err = ErrMsgTooLarge
		return false
	}
	if _, d.err = io.ReadFull(d.r, d.buf[:d.msgLen]); d.err != nil {
		return false
	}
	return true
}

// Request reads the current request or returns nil in the case of an error.
func (d *Decoder) Request() Request {
	if d.err != nil {
		return nil
	}
	req, err := RequestFromJSON(d.buf[:d.msgLen])
	if err != nil {
		d.err = err
	}
	return req
}

// Raw returns the current message or returns nil in the case of an error.
func (d *Decoder) Raw() *json.RawMessage {
	if d.err != nil {
		return nil
	}
	msg := &json.RawMessage{}
	// Note: RawMessage.UnmarshalJSON() cannot fail unless msg is nil and
	// performs no validation itself, so we have to validate afterwards.
	_ = json.Unmarshal(d.buf[:d.msgLen], msg) //nolint:errcheck // RawMessage.UnmarshalJSON cannot fail unless msg is nil
	if !json.Valid(*msg) {
		d.err = ErrMsgInvalid
		return nil
	}
	return msg
}

// MsgSize returns the length of the current message, in bytes.
func (d *Decoder) MsgSize() uint32 {
	return d.msgLen
}

// Err returns the most recently encountered error, or nil if there is none.
func (d *Decoder) Err() error {
	if d.err == nil || d.err == io.EOF { //nolint:errorlint // exact match intentional; wrapped EOF should not be suppressed
		return nil
	}
	return fmt.Errorf("failed to read from stream: %w", d.err)
}
