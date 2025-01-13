package wantjob

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewOpSet(t *testing.T) {
	actual := NewOpSet("b", "c", "b", "a")
	expected := OpSet{"a", "b", "c"}
	require.Equal(t, expected, actual)
}
