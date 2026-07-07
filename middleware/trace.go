package middleware

import "strconv"

const (
	traceparentVersionSize = 2
	traceIDSize            = 32
	parentIDSize           = 16
	traceFlagsSize         = 2
	traceparentSize        = traceparentVersionSize + 1 + traceIDSize + 1 + parentIDSize + 1 + traceFlagsSize
	traceparentMaxSize     = 512
)

// TraceContext contains parsed W3C Trace Context data.
type TraceContext struct {
	// TraceID is the 16-byte trace ID as lowercase hex.
	TraceID string

	// ParentID is the traceparent parent-id as lowercase hex.
	ParentID string

	// Flags is the parsed trace-flags byte.
	Flags byte

	// Sampled reports whether the sampled flag is set.
	Sampled bool

	// Traceparent is the raw traceparent header value.
	Traceparent string

	// Tracestate is the raw tracestate header value. It is only populated when
	// Traceparent is valid.
	Tracestate string

	// Valid reports whether Traceparent parsed successfully.
	Valid bool
}

// ParseTraceparent parses a W3C Trace Context traceparent header value. It
// returns a zero TraceContext when the header is invalid.
func ParseTraceparent(header string) TraceContext {
	if len(header) < traceparentSize || len(header) > traceparentMaxSize {
		return TraceContext{}
	}
	if header[2] != '-' || header[35] != '-' || header[52] != '-' {
		return TraceContext{}
	}

	version := header[:2]
	if !isLowerHex(version) || version == "ff" {
		return TraceContext{}
	}
	if version == "00" && len(header) != traceparentSize {
		return TraceContext{}
	}
	if version != "00" && len(header) > traceparentSize && header[traceparentSize] != '-' {
		return TraceContext{}
	}

	traceID := header[3:35]
	parentID := header[36:52]
	flagsText := header[53:55]
	if !isLowerHex(traceID) || allZero(traceID) {
		return TraceContext{}
	}
	if !isLowerHex(parentID) || allZero(parentID) {
		return TraceContext{}
	}
	if !isLowerHex(flagsText) {
		return TraceContext{}
	}

	flags64, err := strconv.ParseUint(flagsText, 16, 8)
	if err != nil {
		return TraceContext{}
	}
	flags := byte(flags64)

	return TraceContext{
		TraceID:     traceID,
		ParentID:    parentID,
		Flags:       flags,
		Sampled:     flags&0x01 == 0x01,
		Traceparent: header,
		Valid:       true,
	}
}

func isLowerHex(value string) bool {
	for i := range len(value) {
		c := value[i]
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

func allZero(value string) bool {
	for i := range len(value) {
		if value[i] != '0' {
			return false
		}
	}
	return true
}
