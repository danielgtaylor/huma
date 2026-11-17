// Package cbor provides a CBOR formatter for Huma with default configuration.
// Importing this package adds CBOR support to `huma.DefaultFormats`.
package cbor

import (
	"io"

	"github.com/danielgtaylor/huma/v2"
	"github.com/fxamacker/cbor/v2"
)

// cborDecMode decodes the way validation reads the body: exact-case keys and
// no duplicates, so a decoded value can never skip validation.
var cborDecMode, _ = cbor.DecOptions{
	DupMapKey:         cbor.DupMapKeyEnforcedAPF,
	FieldNameMatching: cbor.FieldNameMatchingCaseSensitive,
}.DecMode()

var cborEncMode, _ = cbor.EncOptions{
	// Canonical enc opts
	Sort:          cbor.SortCanonical,
	ShortestFloat: cbor.ShortestFloat16,
	NaNConvert:    cbor.NaNConvert7e00,
	InfConvert:    cbor.InfConvertFloat16,
	IndefLength:   cbor.IndefLengthForbidden,
	// Time handling
	Time:    cbor.TimeUnixDynamic,
	TimeTag: cbor.EncTagRequired,
}.EncMode()

// DefaultCBORFormat is the default CBOR formatter that can be set in the API's
// `Config.Formats` map. This is usually not needed as importing this package
// automatically adds the CBOR format to the default formats.
//
//	config := huma.Config{}
//	config.Formats = map[string]huma.Format{
//		"application/cbor": huma.DefaultCBORFormat,
//		"cbor":             huma.DefaultCBORFormat,
//	}
var DefaultCBORFormat = huma.Format{
	Marshal: func(w io.Writer, v any) error {
		return cborEncMode.NewEncoder(w).Encode(v)
	},
	Unmarshal: cborDecMode.Unmarshal,
}

func init() {
	huma.DefaultFormats["application/cbor"] = DefaultCBORFormat
	huma.DefaultFormats["cbor"] = DefaultCBORFormat
}
