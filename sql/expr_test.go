package sql_test

import (
	stdsql "database/sql"
	"errors"
	"fmt"
	"testing"

	_ "modernc.org/sqlite"

	p "github.com/jrhy/sandbox/parse"
	"github.com/jrhy/sandbox/sql"
	"github.com/jrhy/sandbox/sql/colval"
	"github.com/jrhy/sandbox/sql/types"
	"github.com/stretchr/testify/require"
)

var db *stdsql.DB

func init() {
	var err error
	db, err = stdsql.Open("sqlite", ":memory:")
	if err != nil {
		panic(fmt.Errorf("test sqlite: %v", err))
	}
}

func parseExpr(s string) (*types.Evaluator, error) {
	var evaluator *types.Evaluator
	if !(&p.Parser{Remaining: s}).Match(sql.Expression(&evaluator)) {
		return nil, errors.New("parse failed")
	}
	return evaluator, nil
}

func checkExprEquivSQLite(t *testing.T, expected, sql string) {
	t.Run(sql, func(t *testing.T) {
		t.Parallel()
		e, err := parseExpr(sql)
		require.NoError(t, err)
		cv := e.Func(nil)

		sqliteQuery := `select ` + sql
		sqliteResult, err := db.Query(sqliteQuery)
		require.NoError(t, err, "sqlite "+sqliteQuery)
		var i interface{}
		require.True(t, sqliteResult.Next())
		err = sqliteResult.Scan(&i)
		require.NoError(t, err, "sqlite scan")
		switch x := cv.(type) {
		case colval.Int:
			if _, sameType := i.(int64); !sameType {
				t.Fatalf(`sqlite query "%s" produced %T, but sandbox produced %T`, sqliteQuery, i, x)
			}
			require.Equal(t, int64(cv.(colval.Int)), i.(int64))
		case colval.Null:
			if i != nil {
				t.Fatalf(`sqlite query "%s" produced %T, but sandbox produced %T`, sqliteQuery, i, x)
			}
		case colval.Real:
			if _, sameType := i.(float64); !sameType {
				t.Fatalf(`sqlite query "%s" produced %T, but sandbox produced %T`, sqliteQuery, i, x)
			}
			require.Equal(t, float64(cv.(colval.Real)), i.(float64))
		default:
			t.Fatalf("unhandled type %T", x)
		}

		require.Equal(t, expected, cv.String())
	})
}

func TestExpr_Value(t *testing.T) {
	t.Parallel()
	checkExprEquivSQLite(t, `5`, `5`)
}

func TestExpr_Equality(t *testing.T) {
	t.Parallel()
	checkExprEquivSQLite(t, `1`, `5=5`)
}

func TestExpr_3VL(t *testing.T) {
	t.Parallel()
	checkExprEquivSQLite(t, `NULL`, `null or 0`)
}

func TestExpr_Or(t *testing.T) {
	t.Parallel()
	checkExprEquivSQLite(t, `NULL`, `null or 0`)
	checkExprEquivSQLite(t, `1`, `null or 1`)
	checkExprEquivSQLite(t, `NULL`, `0 or null`)
	checkExprEquivSQLite(t, `NULL`, `null or null`)
	checkExprEquivSQLite(t, `1`, `1 or 0`)
	checkExprEquivSQLite(t, `1`, `0 or 1`)
	checkExprEquivSQLite(t, `1`, `1 or 1`)
	checkExprEquivSQLite(t, `0`, `0 or 0`)
}

func TestExpr_And(t *testing.T) {
	t.Parallel()
	checkExprEquivSQLite(t, `NULL`, `null and null`)
	checkExprEquivSQLite(t, `0`, `null and 0`)
	checkExprEquivSQLite(t, `NULL`, `null and 1`)
	checkExprEquivSQLite(t, `0`, `0 and null`)
	checkExprEquivSQLite(t, `NULL`, `1 and null`)
	checkExprEquivSQLite(t, `0`, `0 and 0`)
	checkExprEquivSQLite(t, `0`, `1 and 0`)
	checkExprEquivSQLite(t, `0`, `0 and 1`)
	checkExprEquivSQLite(t, `1`, `1 and 1`)
}

func TestExpr_AndOrprecedence(t *testing.T) {
	t.Parallel()
	checkExprEquivSQLite(t, `0`, `0 and 1 or 1 and 0`)
}

func TestExpr_Precedence(t *testing.T) {
	t.Parallel()
	checkExprEquivSQLite(t, `24`, `3+4*5+1`)
}

func TestExpr_ColumnReference(t *testing.T) {
	t.Parallel()
	e, err := parseExpr(`3+foo.bar`)
	require.NoError(t, err)
	require.Equal(t, e.Inputs, map[string]struct{}{
		"foo.bar": {},
	})
	rows := e.Func(map[string]colval.ColumnValue{
		"foo.bar": colval.Int(5),
	})
	require.Equal(t, "8", mustJSON(rows))
}

func TestExpr_ArithmeticNull(t *testing.T) {
	t.Parallel()
	checkExprEquivSQLite(t, `NULL`, `3+null`)
}

func TestExpr_ArithmeticString(t *testing.T) {
	t.Parallel()
	checkExprEquivSQLite(t, `18.1`, `3+'4'+"5"+6.1`)
}

func TestExpr_Subexpression(t *testing.T) {
	t.Parallel()
	checkExprEquivSQLite(t, `-163`, `(-((2+3)*4+1)*3)+-+ - -100`)
}

func TestExpr_MaxInt(t *testing.T) {
	t.Parallel()
	checkExprEquivSQLite(t, `9223372036854775807`, `9223372036854775807`)
}

func TestExpr_Overflow(t *testing.T) {
	t.Parallel()
	checkExprEquivSQLite(t, `9.223372036854776e+18`, `9223372036854775807+1`)
}

func TestExpr_Mod(t *testing.T) {
	t.Parallel()
	checkExprEquivSQLite(t, `6`, `1000%7`)
	checkExprEquivSQLite(t, `6.0`, `1000.0%7`)
}

func TestExpr_DivFloat(t *testing.T) {
	t.Parallel()
	checkExprEquivSQLite(t, `142.85714285714286`, `1000.0/7`)
}

func TestExpr_NotEqual(t *testing.T) {
	t.Parallel()
	checkExprEquivSQLite(t, `1`, `5.0!=6.0`)
	checkExprEquivSQLite(t, `0`, `5.0!=5.0`)
	checkExprEquivSQLite(t, `1`, `5.0!=6`)
	checkExprEquivSQLite(t, `0`, `5.0!=5`)
	checkExprEquivSQLite(t, `1`, `5!=6.0`)
	checkExprEquivSQLite(t, `0`, `5!=5.0`)
	checkExprEquivSQLite(t, `1`, `5!=6`)
	checkExprEquivSQLite(t, `0`, `5!=5`)

	checkExprEquivSQLite(t, `1`, `5.0<>6.0`)
	checkExprEquivSQLite(t, `0`, `5.0<>5.0`)
	checkExprEquivSQLite(t, `1`, `5.0<>6`)
	checkExprEquivSQLite(t, `0`, `5.0<>5`)
	checkExprEquivSQLite(t, `1`, `5<>6.0`)
	checkExprEquivSQLite(t, `0`, `5<>5.0`)
	checkExprEquivSQLite(t, `1`, `5<>6`)
	checkExprEquivSQLite(t, `0`, `5<>5`)
}

func TestExpr_LessThan(t *testing.T) {
	t.Parallel()
	checkExprEquivSQLite(t, `1`, `5.0<6.0`)
	checkExprEquivSQLite(t, `0`, `5.0<5.0`)
	checkExprEquivSQLite(t, `1`, `5.0<6`)
	checkExprEquivSQLite(t, `0`, `5.0<5`)
	checkExprEquivSQLite(t, `1`, `5<6.0`)
	checkExprEquivSQLite(t, `0`, `5<5.0`)
	checkExprEquivSQLite(t, `1`, `5<6`)
	checkExprEquivSQLite(t, `0`, `5<5`)
}

func TestExpr_LessThanOrEqual(t *testing.T) {
	t.Parallel()
	checkExprEquivSQLite(t, `1`, `5.0<=6.0`)
	checkExprEquivSQLite(t, `1`, `5.0<=5.0`)
	checkExprEquivSQLite(t, `1`, `5.0<=6`)
	checkExprEquivSQLite(t, `1`, `5.0<=5`)
	checkExprEquivSQLite(t, `1`, `5<=6.0`)
	checkExprEquivSQLite(t, `1`, `5<=5.0`)
	checkExprEquivSQLite(t, `1`, `5<=6`)
	checkExprEquivSQLite(t, `1`, `5<=5`)
	checkExprEquivSQLite(t, `0`, `5<=4`)
}

func TestExpr_Greater(t *testing.T) {
	t.Parallel()
	checkExprEquivSQLite(t, `1`, `6.0>5.0`)
	checkExprEquivSQLite(t, `0`, `5.0>5.0`)
	checkExprEquivSQLite(t, `1`, `6.0>5`)
	checkExprEquivSQLite(t, `0`, `5.0>5`)
	checkExprEquivSQLite(t, `1`, `6>5.0`)
	checkExprEquivSQLite(t, `0`, `5>5.0`)
	checkExprEquivSQLite(t, `1`, `6>5`)
	checkExprEquivSQLite(t, `0`, `5>5`)
}

func TestExpr_GreaterThanOrEqual(t *testing.T) {
	t.Parallel()
	checkExprEquivSQLite(t, `1`, `6.0>=5.0`)
	checkExprEquivSQLite(t, `1`, `5.0>=5.0`)
	checkExprEquivSQLite(t, `1`, `6.0>=5`)
	checkExprEquivSQLite(t, `1`, `5.0>=5`)
	checkExprEquivSQLite(t, `1`, `6>=5.0`)
	checkExprEquivSQLite(t, `1`, `5>=5.0`)
	checkExprEquivSQLite(t, `1`, `6>=5`)
	checkExprEquivSQLite(t, `1`, `5>=5`)
	checkExprEquivSQLite(t, `0`, `4>=5`)
}
