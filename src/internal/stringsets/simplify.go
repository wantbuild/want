package stringsets

import (
	"reflect"
)

func Simplify(x Set) Set {
	// TODO: Prevent oscillations between equivalent states from and.dist()
	for i := 0; i < 3; i++ {
		x1 := simplifyOnce(x)
		if reflect.DeepEqual(x1, x) {
			break
		}
		x = x1
	}
	return x
}

func simplifyOnce(x Set) Set {
	x = x.simplify()
	x = sumOfMinterms(x)
	return x
}

func sumOfMinterms(x Set) Set {
	switch x := x.(type) {
	case Or:
		return Or{
			L: sumOfMinterms(x.L),
			R: sumOfMinterms(x.R),
		}
	case And:
		return x.sumOfMinterms()
	case Not:
		if And, ok := x.X.(And); ok {
			return sumOfMinterms(Or{
				L: Not{And.L},
				R: Not{And.R},
			})
		}
		if or, ok := x.X.(Or); ok {
			return And{
				L: Not{sumOfMinterms(or.L)},
				R: Not{sumOfMinterms(or.R)},
			}
		}
		return Not{sumOfMinterms(x.X)}
	default:
		return x
	}
}

func (a And) sumOfMinterms() Set {
	l, r := sumOfMinterms(a.L), sumOfMinterms(a.R)

	lor, lok := l.(Or)
	ror, rok := r.(Or)
	switch {
	case lok && !rok:
		return Or{
			L: And{lor.L, r},
			R: And{lor.R, r},
		}
	case rok && !lok:
		return Or{
			L: And{l, ror.L},
			R: And{l, ror.R},
		}
	case rok && lok:
		// First, Outside, Inside, Last
		// (a | b) & (c | d)
		// (a & c) | (a & d) | (b & c) | (b & d)
		return Or{
			L: Or{
				L: And{lor.L, ror.L},
				R: And{lor.L, ror.R},
			},
			R: Or{
				L: And{lor.R, ror.L},
				R: And{lor.R, ror.R},
			},
		}
	default:
		return And{l, r}
	}
}

func Equals(a, b Set) bool {
	a, b = Simplify(a), Simplify(b)
	eq := reflect.DeepEqual(a, b)
	return eq
}
