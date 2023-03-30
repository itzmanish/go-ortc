package rtc

import (
	"testing"

	"go.uber.org/atomic"

	"github.com/stretchr/testify/assert"
)

func TestSFU(t *testing.T) {
	sfu, err := NewSFU()
	assert.NoError(t, err)
	assert.NotNil(t, sfu)
}

func runSelectWithChannel(ch chan struct{}, count int) int {
	select {
	case <-ch:
		return count + 1
	default:
	}
	return 0
}

func runAtomicGet(value atomic.Bool, count int) int {
	value.Load()
	return count + 1
}

func BenchmarkSelectVsAtomicValue(b *testing.B) {
	count1 := 0
	count2 := 0
	b.Run("select with chan", func(b *testing.B) {
		ch := make(chan struct{}, b.N)
		for i := 0; i < b.N; i++ {
			count1 += runSelectWithChannel(ch, count1)
		}
		b.Log("total count", count1)
	})
	b.Run("atomic value read", func(b *testing.B) {
		v := *atomic.NewBool(true)
		for i := 0; i < b.N; i++ {
			count2 += runAtomicGet(v, count2)
		}
		b.Log("total count for atomic value", count2)
	})

	b.Log("count", count1, count2)
}
