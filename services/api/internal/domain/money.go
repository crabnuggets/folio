package domain

import (
	"math/big"
	"sort"
)

// summarySum uses arbitrary precision internally so aggregation cannot wrap int64.
type summarySum struct{ n big.Int }

// add accumulates minor units without limiting the intermediate total to int64.
func (a *summarySum) add(v Money) { a.n.Add(&a.n, big.NewInt(int64(v))) }

// sub subtracts minor units without limiting the intermediate total to int64.
func (a *summarySum) sub(v Money) { a.n.Sub(&a.n, big.NewInt(int64(v))) }

// value returns the minor-unit total or a validation error if it exceeds int64.
func (a *summarySum) value() (Money, error) {
	if !a.n.IsInt64() {
		return 0, Invalid("Financial total exceeds the supported amount range")
	}
	return Money(a.n.Int64()), nil
}

// summarySubtract returns a minus b in minor units, or a validation error on overflow.
func summarySubtract(a, b Money) (Money, error) {
	var sum summarySum
	sum.add(a)
	sum.sub(b)
	return sum.value()
}

func sortedKeys[T any](values map[string]T) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
