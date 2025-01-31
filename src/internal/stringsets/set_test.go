package stringsets

import (
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestComplement(t *testing.T) {
	xs := []Set{
		Prefix("abc"),
		Suffix("abc"),
		Unit("key"),
		Or{Unit("key1"), Unit("key2")},
		And{Not{Unit("key")}, Not{Unit("key2")}},
	}
	for i, x := range xs {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			requireEqual(t, x, Not{Not{x}}, "Not(Not(x)) should be x %v", x)
			require.True(t, Intersects(x, x), "%v should intersect itself", x)
			require.True(t, Equals(x, x))
			require.False(t, Intersects(x, Not{x}), "%v should not intersect %v", x, Not{x})
		})
	}
}

func TestIntersects(t *testing.T) {
	type testCase struct {
		A, B       Set
		Intersects bool
	}

	tcs := []testCase{
		{Unit("key1"), Empty{}, false},
		{Prefix("key1"), Empty{}, false},
		{Suffix("key1"), Empty{}, false},
		{Not{Unit("key1")}, Empty{}, false},

		{Unit("key1"), Unit("key1"), true},
		{Unit("key1"), Unit("key2"), false},

		{Prefix("a"), Unit("abc"), true},
		{Prefix("aa"), Unit("b"), false},

		{Suffix("z"), Unit("xyz"), true},
		{Suffix("zz"), Unit("b"), false},

		{Not{Unit("key1")}, Unit("key1"), false},
		{Not{Unit("key1")}, Not{Unit("key2")}, true},
		{Not{Unit("key1")}, Empty{}, false},
		{Not{Unit("key1")}, Top{}, true},
		{
			And{Prefix("mydir/"), Not{Prefix("mydir/not-this")}},
			Prefix("mydir/not-this"),
			false,
		},
		{
			And{Prefix("mydir/"), Not{Prefix("mydir/not-this")}},
			Prefix("mydir/definitely-this"),
			true,
		},
		{
			Or{Prefix(".git/"), Unit(".git")},
			Unit(".git/objects/f2"),
			true,
		},
	}

	for i, tc := range tcs {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			if tc.Intersects {
				assert.True(t, Intersects(tc.A, tc.B), "%v should intersect %v", tc.A, tc.B)
			} else {
				assert.False(t, Intersects(tc.A, tc.B), "%v should not intersect %v", tc.A, tc.B)
			}
		})
	}
}

func TestEquals(t *testing.T) {
	type testCase struct {
		A, B Set
		Eq   bool
	}

	tcs := []testCase{
		{Unit("key1"), Unit("key1"), true},
		{Unit("key1"), Unit("key2"), false},

		{Not{Unit("key1")}, Not{Unit("key1")}, true},
		{Not{Unit("key1")}, Not{Unit("key2")}, false},
		{Not{Unit("key1")}, Empty{}, false},
		{Not{Unit("key1")}, Top{}, false},

		{And{Unit("key1"), And{Not{Unit("key1")}, Not{Unit("key2")}}}, Empty{}, true},
		{And{Not{Unit("key2")}, Or{Unit("key1"), Unit("key2")}}, Unit("key1"), true},
		{And{Suffix("zzz"), Not{Suffix("zz")}}, Empty{}, true},

		{Not{Or{Prefix("abc"), Suffix("xyz")}}, And{Not{Prefix("abc")}, Not{Suffix("xyz")}}, true},
	}

	for i, tc := range tcs {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			if tc.Eq {
				assert.True(t, Equals(tc.A, tc.B), "%v should equal %v", tc.A, tc.B)
			} else {
				assert.False(t, Equals(tc.A, tc.B), "%v should not equal %v", tc.A, tc.B)
			}
		})
	}
}

func TestSuperset(t *testing.T) {
	type testCase struct {
		Super, Sub Set
	}
	tcs := []testCase{
		{
			And{Prefix("aa"), Suffix("zz")},
			And{Prefix("aaa"), Suffix("zzz")},
		},
		{Unit("key1"), Empty{}},
		{Prefix("aa"), Prefix("aaa")},
		{Suffix("zz"), Suffix("zzz")},
	}
	for i, tc := range tcs {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			require.True(t, Superset(tc.Super, tc.Sub), "%v should be a superset of %v", tc.Super, tc.Sub)
			require.True(t, Subset(tc.Sub, tc.Super), "%v should be a subset of %v", tc.Sub, tc.Super)
		})
	}
}

func TestLongestCommon(t *testing.T) {
	type testCase struct {
		xs  []string
		lcp string
	}
	tcs := []testCase{
		{
			[]string{"abcdef", "ab102", "abhgh"},
			"ab",
		},
		{
			[]string{"", "ab", "a"},
			"",
		},
	}
	for _, tc := range tcs {
		lcp := longestCommonPrefix(tc.xs...)
		assert.Equal(t, tc.lcp, lcp, "lcp of %v should be %v", tc.xs, tc.lcp)
	}
}

// TestSimplify tests local simplification.
func TestSimplify(t *testing.T) {
	type testCase struct {
		In, Out Set
	}
	tcs := []testCase{
		{
			In:  And{Prefix("a"), Prefix("aa")},
			Out: Prefix("aa"),
		},
		{
			In:  Or{Prefix("a"), Prefix("aa")},
			Out: Prefix("a"),
		},
		{
			In:  Not{Not{Unit("a")}},
			Out: Unit("a"),
		},
		{
			In:  And{Unit("key1"), Unit("key2")},
			Out: Empty{},
		},
		{
			In:  And{Not{Unit("key1")}, Unit("key2")},
			Out: Unit("key2"),
		},
	}
	for i, tc := range tcs {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			expected := tc.Out
			actual := tc.In.simplify()
			require.Equal(t, expected, actual)
		})
	}
}

func requireEqual(t testing.TB, expected, actual Set, args ...any) {
	require.Equal(t, Simplify(expected), Simplify(actual), args...)
}
