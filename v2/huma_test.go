package huma

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"

	"github.com/mitchellh/mapstructure"
)

func BenchmarkSecondDecode(b *testing.B) {
	type MediumSized struct {
		ID   int      `json:"id"`
		Name string   `json:"name"`
		Tags []string `json:"tags"`
		// Created time.Time `json:"created"`
		// Updated time.Time `json:"updated"`
		Rating float64 `json:"rating"`
		Owner  struct {
			ID    int    `json:"id"`
			Name  string `json:"name"`
			Email string `json:"email"`
		}
		Categories []struct {
			Name    string   `json:"name"`
			Order   int      `json:"order"`
			Visible bool     `json:"visible"`
			Aliases []string `json:"aliases"`
		}
	}

	data := []byte(`{
		"id": 123,
		"name": "Test",
		"tags": ["one", "two", "three"],
		"created": "2021-01-01T12:00:00Z",
		"updated": "2021-01-01T12:00:00Z",
		"rating": 5.0,
		"owner": {
			"id": 4,
			"name": "Alice",
			"email": "alice@example.com"
		},
		"categories": [
			{
				"name": "First",
				"order": 1,
				"visible": true
			},
			{
				"name": "Second",
				"order": 2,
				"visible": false,
				"aliases": ["foo", "bar"]
			}
		]
	}`)

	pb := NewPathBuffer([]byte{}, 0)
	res := &ValidateResult{}
	registry := NewMapRegistry("#/components/schemas/", DefaultSchemaNamer)
	fmt.Println("name", reflect.TypeOf(MediumSized{}).Name())
	schema := registry.Schema(reflect.TypeOf(MediumSized{}), false, "")

	b.Run("json.Unmarshal", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			var tmp any
			if err := json.Unmarshal(data, &tmp); err != nil {
				panic(err)
			}

			Validate(registry, schema, pb, ModeReadFromServer, tmp, res)

			var out MediumSized
			if err := json.Unmarshal(data, &out); err != nil {
				panic(err)
			}
		}
	})

	b.Run("mapstructure.Decode", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			var tmp any
			if err := json.Unmarshal(data, &tmp); err != nil {
				panic(err)
			}

			Validate(registry, schema, pb, ModeReadFromServer, tmp, res)

			var out MediumSized
			if err := mapstructure.Decode(tmp, &out); err != nil {
				panic(err)
			}
		}
	})
}

// var jsonData = []byte(`[
//   {
//     "desired_state": "ON",
//     "etag": "203f7a94",
//     "id": "bvt3",
//     "name": "BVT channel - CNN Plus 2",
//     "org": "t2dev",
//     "self": "https://api.istreamplanet.com/v2/t2dev/channels/bvt3",
// 		"created": "2021-01-01T12:00:00Z",
// 		"count": 18273,
// 		"rating": 5.0,
// 		"tags": ["one", "three"],
//     "source": {
//       "id": "stn-dd4j42ytxmajz6xz",
//       "self": "https://api.istreamplanet.com/v2/t2dev/sources/stn-dd4j42ytxmajz6xz"
//     }
//   },
//   {
//     "desired_state": "ON",
//     "etag": "WgY5zNTPn3ECf_TSPAgL9Y-E9doUaRxAdjukGsCt_sQ",
//     "id": "bvt2",
//     "name": "BVT channel - Hulu",
//     "org": "t2dev",
//     "self": "https://api.istreamplanet.com/v2/t2dev/channels/bvt2",
// 		"created": "2023-01-01T12:01:00Z",
// 		"count": 1,
// 		"rating": 4.5,
// 		"tags": ["two"],
//     "source": {
//       "id": "stn-yuqvm3hzowrv6rph",
//       "self": "https://api.istreamplanet.com/v2/t2dev/sources/stn-yuqvm3hzowrv6rph"
//     }
//   },
//   {
//     "desired_state": "ON",
//     "etag": "1GaleyULVhpmHJXCJPUGSeBM2YYAZGBYKVcR5sZu5U8",
//     "id": "bvt1",
//     "name": "BVT channel - Hulu",
//     "org": "t2dev",
//     "self": "https://api.istreamplanet.com/v2/t2dev/channels/bvt1",
// 		"created": "2023-01-01T12:00:00Z",
// 		"count": 57,
// 		"rating": 3.5,
// 		"tags": ["one", "two"],
//     "source": {
//       "id": "stn-fc6sqodptbz5keuy",
//       "self": "https://api.istreamplanet.com/v2/t2dev/sources/stn-fc6sqodptbz5keuy"
//     }
//   }
// ]`)

// type Summary struct {
// 	DesiredState string    `json:"desired_state"`
// 	ETag         string    `json:"etag"`
// 	ID           string    `json:"id"`
// 	Name         string    `json:"name"`
// 	Org          string    `json:"org"`
// 	Self         string    `json:"self"`
// 	Created      time.Time `json:"created"`
// 	Count        int       `json:"count"`
// 	Rating       float64   `json:"rating"`
// 	Tags         []string  `json:"tags"`
// 	Source       struct {
// 		ID   string `json:"id"`
// 		Self string `json:"self"`
// 	} `json:"source"`
// }

// func BenchmarkMarshalStructJSON(b *testing.B) {
// 	var summaries []Summary
// 	if err := stdjson.Unmarshal(jsonData, &summaries); err != nil {
// 		panic(err)
// 	}

// 	b.ResetTimer()
// 	b.ReportAllocs()
// 	for i := 0; i < b.N; i++ {
// 		b, _ := stdjson.Marshal(summaries)
// 		_ = b
// 	}
// }

// func BenchmarkMarshalAnyJSON(b *testing.B) {
// 	var summaries any
// 	stdjson.Unmarshal(jsonData, &summaries)

// 	b.ResetTimer()
// 	b.ReportAllocs()
// 	for i := 0; i < b.N; i++ {
// 		b, _ := stdjson.Marshal(summaries)
// 		_ = b
// 	}
// }

// func BenchmarkUnmarshalStructJSON(b *testing.B) {
// 	var summaries []Summary

// 	b.ResetTimer()
// 	b.ReportAllocs()
// 	for i := 0; i < b.N; i++ {
// 		summaries = nil
// 		stdjson.Unmarshal(jsonData, &summaries)
// 		_ = summaries
// 	}
// }

// func BenchmarkUnmarshalAnyJSON(b *testing.B) {
// 	var summaries any

// 	b.ResetTimer()
// 	b.ReportAllocs()
// 	for i := 0; i < b.N; i++ {
// 		summaries = nil
// 		stdjson.Unmarshal(jsonData, &summaries)
// 		_ = summaries
// 	}
// }

// func BenchmarkMarshalStructJSONiter(b *testing.B) {
// 	var summaries []Summary
// 	json.Unmarshal(jsonData, &summaries)

// 	b.ResetTimer()
// 	b.ReportAllocs()
// 	for i := 0; i < b.N; i++ {
// 		b, _ := json.Marshal(summaries)
// 		_ = b
// 	}
// }

// func BenchmarkMarshalAnyJSONiter(b *testing.B) {
// 	var summaries any
// 	json.Unmarshal(jsonData, &summaries)

// 	b.ResetTimer()
// 	b.ReportAllocs()
// 	for i := 0; i < b.N; i++ {
// 		b, _ := json.Marshal(summaries)
// 		_ = b
// 	}
// }

// func BenchmarkUnmarshalStructJSONiter(b *testing.B) {
// 	var summaries []Summary

// 	b.ResetTimer()
// 	b.ReportAllocs()
// 	for i := 0; i < b.N; i++ {
// 		summaries = nil
// 		json.Unmarshal(jsonData, &summaries)
// 		_ = summaries
// 	}
// }

// func BenchmarkUnmarshalAnyJSONiter(b *testing.B) {
// 	var summaries any

// 	b.ResetTimer()
// 	b.ReportAllocs()
// 	for i := 0; i < b.N; i++ {
// 		summaries = nil
// 		json.Unmarshal(jsonData, &summaries)
// 		_ = summaries
// 	}
// }
