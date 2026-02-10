package tracking

import (
	"testing"
	"time"

	"github.com/robofuse/robofuse/internal/logger"
)

// tracking_test.go guards expiry behavior across CreatedAt/LastChecked values.

func TestGetExpired_UsesLastCheckedFallbackCreatedAt(t *testing.T) {
	now := time.Now()
	olderThan := 6 * 24 * time.Hour

	svc := &Service{
		trackingFile: "",
		data:         make(map[string]*FileTracking),
		logger:       logger.New("test"),
	}

	// Old created, but recently checked: should NOT be expired.
	svc.data["recent-check"] = &FileTracking{
		RelativePath: "recent-check",
		CreatedAt:    now.Add(-10 * 24 * time.Hour),
		LastChecked:  now.Add(-1 * time.Hour),
	}

	// Old created and never checked: should be expired.
	svc.data["never-checked"] = &FileTracking{
		RelativePath: "never-checked",
		CreatedAt:    now.Add(-10 * 24 * time.Hour),
	}

	expired := svc.GetExpired(olderThan)

	found := map[string]bool{}
	for _, item := range expired {
		found[item.RelativePath] = true
	}

	if found["recent-check"] {
		t.Fatalf("expected recent-check to not be expired based on LastChecked")
	}
	if !found["never-checked"] {
		t.Fatalf("expected never-checked to be expired based on CreatedAt")
	}
}
