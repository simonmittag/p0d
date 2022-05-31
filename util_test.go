package p0d

import (
	"fmt"
	"testing"
)

func TestBraille(t *testing.T) {
	b := NewBrailleAnim()

	for i := 0; i < 24; i++ {
		b.Next()
		fmt.Printf("%d ", b.index)
	}
}
