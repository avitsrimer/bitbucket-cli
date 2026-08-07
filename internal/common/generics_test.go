package common

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSort(t *testing.T) {
	t.Run("orders items by the less comparator", func(t *testing.T) {
		items := []int{3, 1, 2}

		Sort(items, func(a, b int) bool { return a < b })

		assert.Equal(t, []int{1, 2, 3}, items)
	})

	t.Run("preserves the original order of equal elements", func(t *testing.T) {
		type pair struct {
			key int
			seq int
		}
		items := []pair{
			{key: 1, seq: 0},
			{key: 1, seq: 1},
			{key: 0, seq: 2},
			{key: 1, seq: 3},
		}

		Sort(items, func(a, b pair) bool { return a.key < b.key })

		assert.Equal(t, []pair{
			{key: 0, seq: 2},
			{key: 1, seq: 0},
			{key: 1, seq: 1},
			{key: 1, seq: 3},
		}, items)
	})
}

func TestMap(t *testing.T) {
	t.Run("maps every item through mapper", func(t *testing.T) {
		result := Map([]int{1, 2, 3}, func(n int) string { return string(rune('a' + n - 1)) })

		assert.Equal(t, []string{"a", "b", "c"}, result)
	})

	t.Run("returns nil for a nil input", func(t *testing.T) {
		var input []int

		result := Map(input, func(n int) int { return n })

		assert.Nil(t, result)
	})

	t.Run("returns nil for an empty input", func(t *testing.T) {
		result := Map([]int{}, func(n int) int { return n })

		assert.Nil(t, result)
	})
}

func TestFilter(t *testing.T) {
	t.Run("keeps only items matching keep", func(t *testing.T) {
		result := Filter([]int{1, 2, 3, 4}, func(n int) bool { return n%2 == 0 })

		assert.Equal(t, []int{2, 4}, result)
	})

	t.Run("returns nil for a nil input", func(t *testing.T) {
		var input []int

		result := Filter(input, func(n int) bool { return true })

		assert.Nil(t, result)
	})

	t.Run("returns nil for an empty input", func(t *testing.T) {
		result := Filter([]int{}, func(n int) bool { return true })

		assert.Nil(t, result)
	})

	t.Run("returns nil when nothing matches", func(t *testing.T) {
		result := Filter([]int{1, 3, 5}, func(n int) bool { return n%2 == 0 })

		assert.Nil(t, result)
	})
}
