package huma

import (
	"net"
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// A path that arrives at a collection must step into its elements. Anything
// else means the path was recorded wrong, e.g. a kind was added to
// `findInType` without marking its elements.
func TestCollectionPath(t *testing.T) {
	assert.Equal(t, []int{2}, collectionPath([]int{collectionElem, 2}))
	assert.Panics(t, func() { collectionPath([]int{2}) })
	assert.Panics(t, func() { collectionPath(nil) })
}

type textDuration struct {
	time.Duration
}

func (d *textDuration) UnmarshalText(data []byte) error {
	v, err := time.ParseDuration(string(data))
	if err != nil {
		return err
	}
	d.Duration = v
	return nil
}

func TestConvertTypeTextUnmarshaler(t *testing.T) {
	assert.Equal(t, textDuration{10 * time.Second}, convertType("D", reflect.TypeFor[textDuration](), "10s"))
	assert.Equal(t, &textDuration{10 * time.Second}, convertType("D", reflect.TypeFor[*textDuration](), "10s"))
	assert.Panics(t, func() { convertType("D", reflect.TypeFor[textDuration](), "bogus") })

	// net.IP is a []byte, so a plain conversion would store the ASCII bytes.
	assert.Equal(t, net.ParseIP("127.0.0.1"), convertType("IP", reflect.TypeFor[net.IP](), "127.0.0.1"))
	assert.Equal(t, []net.IP{net.ParseIP("1.1.1.1"), net.ParseIP("8.8.8.8")}, convertType("IPs", reflect.TypeFor[[]net.IP](), []string{"1.1.1.1", "8.8.8.8"}))
}

func TestTextUnmarshalerDefault(t *testing.T) {
	type Input struct {
		D  textDuration   `json:"d" default:"10s"`
		Ds []textDuration `json:"ds" default:"1s,2s"`
	}
	r := NewMapRegistry("#/components/schemas/", DefaultSchemaNamer)
	s := SchemaFromType(r, reflect.TypeFor[Input]())
	assert.Equal(t, "10s", s.Properties["d"].Default)
	assert.Equal(t, []string{"1s", "2s"}, s.Properties["ds"].Default)

	var in Input
	setDefaults(reflect.ValueOf(&in).Elem(), findDefaults(r, reflect.TypeFor[Input]()))
	assert.Equal(t, 10*time.Second, in.D.Duration)
	assert.Equal(t, []textDuration{{time.Second}, {2 * time.Second}}, in.Ds)

	type Invalid struct {
		D textDuration `json:"d" default:"bogus"`
	}
	assert.Panics(t, func() { SchemaFromType(r, reflect.TypeFor[Invalid]()) })
}

// Each level points at the next twice, so there are 2^depth paths to the leaf.
func TestFindInTypeSkipsUnmatchedDAG(t *testing.T) {
	typ := reflect.TypeFor[struct{ X int }]()
	for range 20 {
		typ = reflect.StructOf([]reflect.StructField{
			{Name: "A", Type: reflect.PointerTo(typ)},
			{Name: "B", Type: reflect.PointerTo(typ)},
		})
	}

	calls := 0
	r := findInType(typ, nil, func(reflect.StructField) bool {
		calls++
		return false
	}, true)
	assert.Empty(t, r.Paths)
	assert.Equal(t, 41, calls)
}

type findCycleRoot struct {
	A *findCycleA
	B *findCycleB
}

type findCycleA struct {
	B *findCycleB
	X int `default:"1"`
}

type findCycleB struct {
	A *findCycleA
}

// A default reached through a cycle must be found along both routes to it.
func TestFindInTypeCycleNotSkipped(t *testing.T) {
	r := NewMapRegistry("#/components/schemas/", DefaultSchemaNamer)
	found := findDefaults(r, reflect.TypeFor[findCycleRoot]()).Paths
	paths := make([][]int, 0, len(found))
	for _, p := range found {
		paths = append(paths, p.Path)
	}
	assert.Equal(t, [][]int{{0, 1}, {1, 0, 1}}, paths)
}

type findSCC1 struct {
	B *findSCC2
	C *findSCC3
}

type findSCC2 struct {
	A *findSCC1
	C *findSCC3
}

type findSCC3 struct {
	A *findSCC1
	B *findSCC2
}

// Types that all reference each other have a factorial number of paths.
func TestFindInTypeSkipsUnmatchedCycles(t *testing.T) {
	calls := 0
	r := findInType(reflect.TypeFor[findSCC1](), nil, func(reflect.StructField) bool {
		calls++
		return false
	}, true)
	assert.Empty(t, r.Paths)
	assert.Equal(t, 6, calls)
}

type findTreeItem struct {
	Children []findTreeItem
}

func (*findTreeItem) Resolve(Context) []error { return nil }

// The cycle passes through `[]findTreeItem`, and the resolver on the nested
// items must still be found.
func TestFindInTypeResolverThroughRecursiveSlice(t *testing.T) {
	found := findResolvers(resolverType, reflect.TypeFor[struct{ Items []findTreeItem }]()).Paths
	paths := make([][]int, 0, len(found))
	for _, p := range found {
		paths = append(paths, p.Path)
	}
	assert.Equal(t, [][]int{{0, collectionElem}, {0, collectionElem, 0, collectionElem}}, paths)
}
