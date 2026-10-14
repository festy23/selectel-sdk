package dns

import (
	"github.com/festy23/selectel-sdk/types"
	"math/big"
	"strings"
	"unicode/utf8"
)

func (s service) scopeValid() error {
	if s.scope.ProjectID == "" || s.scope.Region != "" || s.scope.Pool != "" {
		return types.NewError(types.ScopeMismatch)
	}
	return nil
}
func fqdn(name string) bool {
	return utf8.ValidString(name) && len(name) > 0 && strings.HasSuffix(name, ".")
}
func commentValid(comment *types.Nullable[string]) bool {
	if comment == nil {
		return true
	}
	v, valid := comment.Value()
	return !valid || utf8.ValidString(v) && utf8.RuneCountInString(v) <= maxCommentLength
}
func validateRR(ttl types.Integer, rows []Records, comment *types.Nullable[string]) error {
	n, ok := new(big.Int).SetString(ttl.String(), 10)
	if !ok || n.Cmp(big.NewInt(minTTL)) < 0 || n.Cmp(big.NewInt(maxTTL)) > 0 || len(rows) < minRecords || len(rows) > maxRecords || !commentValid(comment) {
		return types.NewError(types.InvalidArgument)
	}
	for _, row := range rows {
		if !utf8.Valid(row.Content.Reveal()) {
			return types.NewError(types.InvalidArgument)
		}
	}
	return nil
}
