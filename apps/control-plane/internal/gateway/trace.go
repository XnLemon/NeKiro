package gateway

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"sync/atomic"

	"github.com/Nene7ko/NeKiro/contracts"
)

type TraceGenerator struct {
	prefix  string
	counter atomic.Uint64
}

func NewTraceGenerator() (*TraceGenerator, error) {
	return newTraceGenerator(rand.Reader)
}

func newTraceGenerator(source io.Reader) (*TraceGenerator, error) {
	seed := make([]byte, 16)
	if _, err := io.ReadFull(source, seed); err != nil {
		return nil, fmt.Errorf("initialize trace generator: %w", err)
	}
	return &TraceGenerator{prefix: "trc_" + hex.EncodeToString(seed)}, nil
}

func (generator *TraceGenerator) Next() contracts.TraceID {
	return contracts.TraceID(fmt.Sprintf("%s_%x", generator.prefix, generator.counter.Add(1)))
}
