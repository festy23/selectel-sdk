package privatedns

import (
	"github.com/festy23/selectel-sdk/types"
	"math/big"
	"unicode/utf8"
)

func negative(i *types.Integer, min int64) bool {
	if i == nil {
		return false
	}
	v, ok := new(big.Int).SetString(i.String(), 10)
	return !ok || v.Cmp(big.NewInt(min)) < 0
}

func validText(s string, min, max int) bool {
	return utf8.ValidString(s) && utf8.RuneCountInString(s) >= min && utf8.RuneCountInString(s) <= max
}
