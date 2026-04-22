package util

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestToFloat64(t *testing.T) {
	assert.Equal(t, 3.14, ToFloat64(3.14))
	assert.Equal(t, 42.0, ToFloat64(42))
	assert.Equal(t, 100.0, ToFloat64(int64(100)))
	assert.Equal(t, 7.5, ToFloat64(json.Number("7.5")))
	assert.Equal(t, 0.0, ToFloat64("not a number"))
	assert.Equal(t, 0.0, ToFloat64(nil))
	assert.Equal(t, 0.0, ToFloat64([]int{}))
}
