package platformfinance

import (
	"reflect"
	"testing"
)

func TestPhase3AAuditSurfaceHasNoDomainPostingMethods(t *testing.T) {
	auditedType := reflect.TypeOf((*AuditedJournalService)(nil))
	if auditedType.NumMethod() != 1 || auditedType.Method(0).Name != "ReverseJournal" {
		t.Fatalf("audited service must expose only ReverseJournal, methods=%d", auditedType.NumMethod())
	}
	guardType := reflect.TypeOf((*LiveWriteGuard)(nil))
	if guardType.NumMethod() != 1 || guardType.Method(0).Name != "RejectPrematureLiveWrite" {
		t.Fatalf("LIVE guard must expose only RejectPrematureLiveWrite, methods=%d", guardType.NumMethod())
	}
}
