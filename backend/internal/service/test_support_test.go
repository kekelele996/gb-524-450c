package service

import (
	"fmt"
	"sync/atomic"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"spectrum-interference-triangulation/backend/internal/model"
)

var testDBSeq atomic.Uint64

// newTestDB 使用独立的 SQLite 内存数据库建立功能测试所需的全部表。
func newTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:review-basis-%d?mode=memory&cache=shared", testDBSeq.Add(1))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite test database: %v", err)
	}
	if err := db.AutoMigrate(
		&model.User{},
		&model.ReceiverStation{},
		&model.InterferenceCase{},
		&model.BearingObservation{},
		&model.LocalizationEstimate{},
		&model.AuditEvent{},
	); err != nil {
		t.Fatalf("migrate sqlite test database: %v", err)
	}
	return db
}
