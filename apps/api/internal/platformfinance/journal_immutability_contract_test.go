package platformfinance

import (
	"reflect"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func methodNames(t reflect.Type) []string {
	names := make([]string, 0, t.NumMethod())
	for i := 0; i < t.NumMethod(); i++ {
		names = append(names, t.Method(i).Name)
	}
	return names
}

func TestPhase3AImmutabilityRuntimeSurfaceIsReadOrAppendOnly(t *testing.T) {
	cases := []struct {
		name     string
		typ      reflect.Type
		expected []string
	}{
		{name: "journal service", typ: reflect.TypeOf((*JournalService)(nil)).Elem(), expected: []string{"PostJournal", "ReverseJournal"}},
		{name: "journal repository", typ: reflect.TypeOf((*JournalRepository)(nil)).Elem(), expected: []string{
			"GetAccountDefinitions", "TryInsertJournal", "InsertEntries", "GetJournalByEventKey", "GetJournalByIDForReversal", "GetReversalBySourceID",
		}},
		{name: "journal read service", typ: reflect.TypeOf((*JournalReadService)(nil)).Elem(), expected: []string{"ListJournals", "GetSummary"}},
		{name: "journal read repository", typ: reflect.TypeOf((*JournalReadRepository)(nil)).Elem(), expected: []string{"ListJournals", "GetSummary"}},
		{name: "audited journal service", typ: reflect.TypeOf((*AuditedJournalService)(nil)), expected: []string{"ReverseJournal"}},
		{name: "live write guard", typ: reflect.TypeOf((*LiveWriteGuard)(nil)), expected: []string{"RejectPrematureLiveWrite"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			actual := methodNames(tc.typ)
			assert.ElementsMatch(t, tc.expected, actual)
			for _, name := range actual {
				assert.False(t, strings.HasPrefix(name, "Update"), "immutable ledger surface must not expose %s", name)
				assert.False(t, strings.HasPrefix(name, "Delete"), "immutable ledger surface must not expose %s", name)
			}
		})
	}
}

func TestPhase3AImmutabilityHTTPRouteSurfaceHasNoMutationPath(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	noopMiddleware := func(c *gin.Context) { c.Next() }
	require.NotPanics(t, func() {
		RegisterRoutes(router, noopMiddleware, noopMiddleware, func(...string) gin.HandlerFunc { return noopMiddleware }, nil)
	})

	routes := router.Routes()
	assert.Len(t, routes, 2)
	assert.ElementsMatch(t, []string{
		"GET /admin/finance/summary",
		"GET /admin/finance/breakdown",
	}, routeSignatures(routes))
	for _, route := range routes {
		if strings.HasPrefix(route.Path, "/admin/finance/") {
			assert.NotContains(t, []string{"POST", "PUT", "PATCH", "DELETE"}, route.Method,
				"platform finance must not expose a journal mutation route")
		}
	}
}

func routeSignatures(routes gin.RoutesInfo) []string {
	signatures := make([]string, 0, len(routes))
	for _, route := range routes {
		signatures = append(signatures, route.Method+" "+route.Path)
	}
	return signatures
}
