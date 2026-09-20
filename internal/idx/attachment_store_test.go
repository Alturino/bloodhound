package idx

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alturino/bloodhound/internal/db/.gen/bloodhound/public/model"
)

func TestInsertAttachment_Integration(t *testing.T) {
	testDB := setupTestDB(t)
	store := newTestAttachmentStoreWithDB(testDB.db)
	ctx := context.Background()

	// Insert a parent announcement first (foreign key)
	annStore := newTestAnnouncementStoreWithDB(testDB.db)
	parentAnn := newTestAnnouncement(func(a *model.Announcements) {
		a.IdxID = "att-parent-001"
	})
	require.NoError(t, annStore.InsertAnnouncement(ctx, nil, parentAnn))

	t.Run("empty slice returns nil", func(t *testing.T) {
		err := store.InsertAttachment(ctx, nil)
		assert.NoError(t, err)
	})

	t.Run("single attachment", func(t *testing.T) {
		att := newTestAttachment(func(a *model.Attachments) {
			a.IdxAnnouncementID = "att-parent-001"
		})
		err := store.InsertAttachment(ctx, nil, att)
		assert.NoError(t, err)

		// Verify it was inserted by querying directly
		var count int
		err = testDB.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM attachments").Scan(&count)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, count, 1)
	})

	t.Run("multiple attachments", func(t *testing.T) {
		att1 := newTestAttachment(func(a *model.Attachments) {
			a.ID = uuid.New()
			a.IdxAnnouncementID = "att-parent-001"
			a.OriginalFilename = "multi1.pdf"
		})
		att2 := newTestAttachment(func(a *model.Attachments) {
			a.ID = uuid.New()
			a.IdxAnnouncementID = "att-parent-001"
			a.OriginalFilename = "multi2.pdf"
		})
		err := store.InsertAttachment(ctx, nil, att1, att2)
		assert.NoError(t, err)
	})
}

func TestGetAttachmentByID_Integration(t *testing.T) {
	testDB := setupTestDB(t)
	store := newTestAttachmentStoreWithDB(testDB.db)
	ctx := context.Background()

	// Insert parent announcement
	annStore := newTestAnnouncementStoreWithDB(testDB.db)
	parentAnn := newTestAnnouncement(func(a *model.Announcements) {
		a.IdxID = "att-get-parent-001"
	})
	require.NoError(t, annStore.InsertAnnouncement(ctx, nil, parentAnn))

	t.Run("empty ids returns zero value", func(t *testing.T) {
		att, err := store.Attachment(ctx, nil)
		assert.NoError(t, err)
		assert.Equal(t, model.Attachments{}, att)
	})

	t.Run("not found", func(t *testing.T) {
		_, err := store.Attachment(ctx, nil, uuid.New())
		assert.Error(t, err)
	})
}

func TestGetUnprocessedAttachments_Integration(t *testing.T) {
	testDB := setupTestDB(t)
	store := newTestAttachmentStoreWithDB(testDB.db)
	ctx := context.Background()

	// Insert parent announcement
	annStore := newTestAnnouncementStoreWithDB(testDB.db)
	parentAnn := newTestAnnouncement(func(a *model.Announcements) {
		a.IdxID = "att-unproc-parent-001"
	})
	require.NoError(t, annStore.InsertAnnouncement(ctx, nil, parentAnn))

	t.Run("no unprocessed attachments", func(t *testing.T) {
		attachments, err := store.UnprocessedAttachments(ctx)
		assert.NoError(t, err)
		assert.Empty(t, attachments)
	})

	t.Run("returns unprocessed attachments", func(t *testing.T) {
		att := newTestAttachment(func(a *model.Attachments) {
			a.ID = uuid.New()
			a.IdxAnnouncementID = "att-unproc-parent-001"
			a.IsDownloaded = false
			a.IsProcessing = false
			a.StoragePath = ""
		})
		require.NoError(t, store.InsertAttachment(ctx, nil, att))

		attachments, err := store.UnprocessedAttachments(ctx)
		assert.NoError(t, err)
		assert.Len(t, attachments, 1)
		assert.Equal(t, "att-unproc-parent-001", attachments[0].IdxAnnouncementID)
		assert.Equal(t, "laporan.pdf", attachments[0].OriginalFilename)
	})

	t.Run("excludes downloaded", func(t *testing.T) {
		att := newTestAttachment(func(a *model.Attachments) {
			a.ID = uuid.New()
			a.IdxAnnouncementID = "att-unproc-parent-001"
			a.OriginalFilename = "downloaded.pdf"
			a.IsDownloaded = true
			a.StoragePath = "some/path.pdf"
		})
		require.NoError(t, store.InsertAttachment(ctx, nil, att))

		attachments, err := store.UnprocessedAttachments(ctx)
		assert.NoError(t, err)
		for _, a := range attachments {
			assert.NotEqual(t, "downloaded.pdf", a.OriginalFilename)
		}
	})

	// NOTE: IsProcessing is in DefaultColumns for INSERT, so it always defaults to false.
	// To test the processing exclusion, we need to first claim an attachment via ClaimAttachments.
	t.Run("excludes processing after claim", func(t *testing.T) {
		att := newTestAttachment(func(a *model.Attachments) {
			a.ID = uuid.New()
			a.IdxAnnouncementID = "att-unproc-parent-001"
			a.OriginalFilename = "processing-after-claim.pdf"
			a.IsDownloaded = false
			a.StoragePath = ""
		})
		require.NoError(t, store.InsertAttachment(ctx, nil, att))

		// Get the DB-generated ID
		var dbID uuid.UUID
		err := testDB.db.QueryRowContext(ctx,
			"SELECT id FROM attachments WHERE original_filename = $1",
			"processing-after-claim.pdf",
		).Scan(&dbID)
		require.NoError(t, err)

		// Claim the attachment to set is_processing=true
		claimed, err := store.ClaimAttachments(ctx, dbID)
		require.NoError(t, err)
		require.Len(t, claimed, 1)

		// Now it should be excluded from unprocessed
		attachments, err := store.UnprocessedAttachments(ctx)
		assert.NoError(t, err)
		for _, a := range attachments {
			assert.NotEqual(t, "processing-after-claim.pdf", a.OriginalFilename)
		}
	})

	t.Run("excludes 404 error attachments", func(t *testing.T) {
		att := newTestAttachment(func(a *model.Attachments) {
			a.ID = uuid.New()
			a.IdxAnnouncementID = "att-unproc-parent-001"
			a.OriginalFilename = "notfound.pdf"
			a.IsDownloaded = false
			a.IsProcessing = false
			a.StoragePath = ""
			a.Error = "status_code=404"
		})
		require.NoError(t, store.InsertAttachment(ctx, nil, att))

		attachments, err := store.UnprocessedAttachments(ctx)
		assert.NoError(t, err)
		for _, a := range attachments {
			assert.NotEqual(t, "notfound.pdf", a.OriginalFilename)
		}
	})
}

func TestClaimAttachments_Integration(t *testing.T) {
	testDB := setupTestDB(t)
	store := newTestAttachmentStoreWithDB(testDB.db)
	ctx := context.Background()

	// Insert parent announcement
	annStore := newTestAnnouncementStoreWithDB(testDB.db)
	parentAnn := newTestAnnouncement(func(a *model.Announcements) {
		a.IdxID = "att-claim-parent-001"
	})
	require.NoError(t, annStore.InsertAnnouncement(ctx, nil, parentAnn))

	t.Run("empty ids returns error", func(t *testing.T) {
		_, err := store.ClaimAttachments(ctx)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "attachments id is empty")
	})

	t.Run("claim unprocessed attachment", func(t *testing.T) {
		att := newTestAttachment(func(a *model.Attachments) {
			a.ID = uuid.New()
			a.IdxAnnouncementID = "att-claim-parent-001"
			a.OriginalFilename = "claim-test.pdf"
			a.IsDownloaded = false
			a.StoragePath = ""
		})
		require.NoError(t, store.InsertAttachment(ctx, nil, att))

		var dbID uuid.UUID
		err := testDB.db.QueryRowContext(ctx,
			"SELECT id FROM attachments WHERE original_filename = $1",
			"claim-test.pdf",
		).Scan(&dbID)
		require.NoError(t, err)

		claimed, err := store.ClaimAttachments(ctx, dbID)
		assert.NoError(t, err)
		assert.Len(t, claimed, 1)
		assert.True(t, claimed[0].IsProcessing)
	})

	// NOTE: IsProcessing is in DefaultColumns for INSERT (defaults to false).
	// We can't directly insert with IsProcessing=true. After claiming,
	// re-claiming should return empty because the WHERE requires IsProcessing=false.
	t.Run("cannot re-claim already claimed attachment", func(t *testing.T) {
		att := newTestAttachment(func(a *model.Attachments) {
			a.ID = uuid.New()
			a.IdxAnnouncementID = "att-claim-parent-001"
			a.OriginalFilename = "reclaim-test.pdf"
			a.IsDownloaded = false
			a.StoragePath = ""
		})
		require.NoError(t, store.InsertAttachment(ctx, nil, att))

		var dbID uuid.UUID
		err := testDB.db.QueryRowContext(ctx,
			"SELECT id FROM attachments WHERE original_filename = $1",
			"reclaim-test.pdf",
		).Scan(&dbID)
		require.NoError(t, err)

		// First claim should succeed
		claimed, err := store.ClaimAttachments(ctx, dbID)
		require.NoError(t, err)
		assert.Len(t, claimed, 1)

		// Second claim should return empty (already processing)
		claimed, err = store.ClaimAttachments(ctx, dbID)
		assert.NoError(t, err)
		assert.Len(t, claimed, 0)
	})

	t.Run("cannot claim downloaded attachment", func(t *testing.T) {
		att := newTestAttachment(func(a *model.Attachments) {
			a.ID = uuid.New()
			a.IdxAnnouncementID = "att-claim-parent-001"
			a.OriginalFilename = "already-downloaded.pdf"
			a.IsDownloaded = true
			a.StoragePath = "some/path.pdf"
		})
		require.NoError(t, store.InsertAttachment(ctx, nil, att))

		var dbID uuid.UUID
		err := testDB.db.QueryRowContext(ctx,
			"SELECT id FROM attachments WHERE original_filename = $1",
			"already-downloaded.pdf",
		).Scan(&dbID)
		require.NoError(t, err)

		claimed, err := store.ClaimAttachments(ctx, dbID)
		assert.NoError(t, err)
		assert.Len(t, claimed, 0)
	})
}

func TestUnclaimAttachments_Integration(t *testing.T) {
	testDB := setupTestDB(t)
	store := newTestAttachmentStoreWithDB(testDB.db)
	ctx := context.Background()

	// Type assert to concrete type since UnclaimAttachments is not on the interface
	attStore, ok := store.(*attachmentStore)
	require.True(t, ok, "store should be *attachmentStore")

	// Insert parent announcement
	annStore := newTestAnnouncementStoreWithDB(testDB.db)
	parentAnn := newTestAnnouncement(func(a *model.Announcements) {
		a.IdxID = "att-unclaim-parent-001"
	})
	require.NoError(t, annStore.InsertAnnouncement(ctx, nil, parentAnn))

	t.Run("empty ids returns error", func(t *testing.T) {
		_, err := attStore.UnclaimAttachments(ctx)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "attachments id is empty")
	})

	t.Run("unclaim a claimed attachment", func(t *testing.T) {
		att := newTestAttachment(func(a *model.Attachments) {
			a.ID = uuid.New()
			a.IdxAnnouncementID = "att-unclaim-parent-001"
			a.OriginalFilename = "unclaim-test.pdf"
			a.IsDownloaded = false
			a.StoragePath = ""
		})
		require.NoError(t, attStore.InsertAttachment(ctx, nil, att))

		// Get the DB-generated ID
		var dbID uuid.UUID
		err := testDB.db.QueryRowContext(ctx,
			"SELECT id FROM attachments WHERE original_filename = $1",
			"unclaim-test.pdf",
		).Scan(&dbID)
		require.NoError(t, err)

		// First claim the attachment
		claimed, err := attStore.ClaimAttachments(ctx, dbID)
		require.NoError(t, err)
		require.Len(t, claimed, 1)
		assert.True(t, claimed[0].IsProcessing)

		// Now unclaim it
		unclaimed, err := attStore.UnclaimAttachments(ctx, dbID)
		assert.NoError(t, err)
		assert.Len(t, unclaimed, 1)
		assert.False(t, unclaimed[0].IsProcessing)
	})
}

func TestUpdateAttachmentResult_Integration(t *testing.T) {
	testDB := setupTestDB(t)
	store := newTestAttachmentStoreWithDB(testDB.db)
	ctx := context.Background()

	// Insert parent announcement
	annStore := newTestAnnouncementStoreWithDB(testDB.db)
	parentAnn := newTestAnnouncement(func(a *model.Announcements) {
		a.IdxID = "att-update-parent-001"
	})
	require.NoError(t, annStore.InsertAnnouncement(ctx, nil, parentAnn))

	t.Run("update attachment with success", func(t *testing.T) {
		att := newTestAttachment(func(a *model.Attachments) {
			a.ID = uuid.New()
			a.IdxAnnouncementID = "att-update-parent-001"
			a.OriginalFilename = "update-success.pdf"
			a.IsDownloaded = false
		})
		require.NoError(t, store.InsertAttachment(ctx, nil, att))

		// Get the DB-generated ID
		var dbID uuid.UUID
		err := testDB.db.QueryRowContext(ctx,
			"SELECT id FROM attachments WHERE original_filename = $1",
			"update-success.pdf",
		).Scan(&dbID)
		require.NoError(t, err)

		att.ID = dbID
		att.IsDownloaded = true
		att.StoragePath = "bbca/2025-01-15_laporan.pdf"
		att.Checksum = "abc123"

		err = store.UpdateAttachmentResult(ctx, &att)
		assert.NoError(t, err)
	})

	t.Run("update attachment with error", func(t *testing.T) {
		att := newTestAttachment(func(a *model.Attachments) {
			a.ID = uuid.New()
			a.IdxAnnouncementID = "att-update-parent-001"
			a.OriginalFilename = "update-error.pdf"
			a.IsDownloaded = false
		})
		require.NoError(t, store.InsertAttachment(ctx, nil, att))

		// Get the DB-generated ID
		var dbID uuid.UUID
		err := testDB.db.QueryRowContext(ctx,
			"SELECT id FROM attachments WHERE original_filename = $1",
			"update-error.pdf",
		).Scan(&dbID)
		require.NoError(t, err)

		att.ID = dbID
		att.Error = "download failed"
		att.IsDownloaded = false

		err = store.UpdateAttachmentResult(ctx, &att)
		assert.NoError(t, err)
	})
}
