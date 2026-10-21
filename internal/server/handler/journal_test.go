package handler_test

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"todoapp/internal/models"
)

func TestJournalUpsertAndGet(t *testing.T) {
	ts := newTestServer(t, generousRate)
	token, _ := registerAndLogin(t, ts, "juser1", "journal-pass-123")

	// 1. Get on empty date -> returns 200 with entry: null
	res, body := ts.do(http.MethodGet, "/api/v1/journal?date=2026-10-20", token, nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", res.StatusCode, body)
	}
	var emptyResp models.JournalResponse
	if err := json.Unmarshal(body, &emptyResp); err != nil {
		t.Fatalf("unmarshal empty journal: %v", err)
	}
	if emptyResp.Entry != nil {
		t.Fatalf("expected nil entry, got %+v", emptyResp.Entry)
	}

	// 2. Upsert Journal
	saveReq := models.SaveJournalRequest{
		Date:  "2026-10-20",
		Title: "Progress Jurnal Hari Ini",
		Body:  "# Daily Notes\nStarted building the terminal app in Go.",
		Tags:  "golang, tui",
	}
	res, body = ts.do(http.MethodPut, "/api/v1/journal", token, saveReq)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", res.StatusCode, body)
	}
	var savedResp models.JournalResponse
	if err := json.Unmarshal(body, &savedResp); err != nil {
		t.Fatalf("unmarshal saved journal: %v", err)
	}
	if savedResp.Entry == nil || savedResp.Entry.Title != saveReq.Title || savedResp.Entry.Body != saveReq.Body || savedResp.Entry.Tags != saveReq.Tags {
		t.Fatalf("unexpected entry fields: %+v", savedResp.Entry)
	}

	// 3. Upsert again for the same date with ID (updates content)
	updateReq := models.SaveJournalRequest{
		ID:    savedResp.Entry.ID,
		Date:  "2026-10-20",
		Title: "Progress Jurnal Update",
		Body:  "# Daily Notes\nFinished building the terminal app in Go!",
		Tags:  "golang, tui, completed",
	}
	res, body = ts.do(http.MethodPut, "/api/v1/journal", token, updateReq)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", res.StatusCode, body)
	}
	var updatedResp models.JournalResponse
	json.Unmarshal(body, &updatedResp)
	if updatedResp.Entry.Title != updateReq.Title || updatedResp.Entry.Body != updateReq.Body || updatedResp.Entry.Tags != updateReq.Tags {
		t.Fatalf("update failed: %+v", updatedResp.Entry)
	}

	// 4. Get by date returns updated
	res, body = ts.do(http.MethodGet, "/api/v1/journal?date=2026-10-20", token, nil)
	json.Unmarshal(body, &savedResp)
	if savedResp.Entry == nil || savedResp.Entry.Title != updateReq.Title || savedResp.Entry.Body != updateReq.Body {
		t.Fatalf("get failed after update: %+v", savedResp.Entry)
	}
}

func TestJournalMultiEntriesAndIDDelete(t *testing.T) {
	ts := newTestServer(t, generousRate)
	token, _ := registerAndLogin(t, ts, "multijournal", "pass-123456")

	// 1. Create first note
	req1 := models.SaveJournalRequest{
		Date:  "2026-10-22",
		Title: "Catatan Pagi",
		Body:  "Bangun pagi dan ngopi.",
		Tags:  "morning",
	}
	res, body := ts.do(http.MethodPut, "/api/v1/journal", token, req1)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("create note 1 failed: %d: %s", res.StatusCode, body)
	}
	var resp1 models.JournalResponse
	json.Unmarshal(body, &resp1)
	note1ID := resp1.Entry.ID

	// 2. Create second note on the same date (multi-notes)
	req2 := models.SaveJournalRequest{
		Date:  "2026-10-22",
		Title: "Catatan Sore",
		Body:  "Sore hari deploy backend.",
		Tags:  "afternoon",
	}
	res, body = ts.do(http.MethodPut, "/api/v1/journal", token, req2)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("create note 2 failed: %d: %s", res.StatusCode, body)
	}
	var resp2 models.JournalResponse
	json.Unmarshal(body, &resp2)
	note2ID := resp2.Entry.ID

	if note1ID == note2ID {
		t.Fatalf("expected different IDs for multiple notes, got %s", note1ID)
	}

	// 3. List entries for this date
	res, body = ts.do(http.MethodGet, "/api/v1/journal?date=2026-10-22", token, nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("get date failed: %d: %s", res.StatusCode, body)
	}
	var listResp models.JournalResponse
	json.Unmarshal(body, &listResp)
	if len(listResp.Entries) != 2 {
		t.Fatalf("expected 2 journal entries, got %d", len(listResp.Entries))
	}

	// 4. Delete note 1 by ID
	res, body = ts.do(http.MethodDelete, "/api/v1/journal?id="+note1ID, token, nil)
	if res.StatusCode != http.StatusNoContent {
		t.Fatalf("delete note 1 by ID failed: %d: %s", res.StatusCode, body)
	}

	// 5. Verify only note 2 remains
	res, body = ts.do(http.MethodGet, "/api/v1/journal?date=2026-10-22", token, nil)
	json.Unmarshal(body, &listResp)
	if len(listResp.Entries) != 1 {
		t.Fatalf("expected 1 remaining note, got %d", len(listResp.Entries))
	}
	if listResp.Entries[0].ID != note2ID {
		t.Fatalf("expected remaining note to be note2 (%s), got %s", note2ID, listResp.Entries[0].ID)
	}
}

func TestJournalAuthorization(t *testing.T) {
	ts := newTestServer(t, generousRate)
	tokenAlice, _ := registerAndLogin(t, ts, "jalice", "alice-pass-123")
	tokenBob, _ := registerAndLogin(t, ts, "jbob", "bob-pass-123")

	// Alice writes journal on 2026-10-20
	ts.do(http.MethodPut, "/api/v1/journal", tokenAlice, models.SaveJournalRequest{
		Date: "2026-10-20",
		Body: "Alice secret thoughts.",
	})

	// Bob reads journal on 2026-10-20 -> should get null
	res, body := ts.do(http.MethodGet, "/api/v1/journal?date=2026-10-20", tokenBob, nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", res.StatusCode)
	}
	var bobResp models.JournalResponse
	json.Unmarshal(body, &bobResp)
	if bobResp.Entry != nil {
		t.Fatalf("Bob should not see Alice's journal entry: %+v", bobResp.Entry)
	}

	// Bob attempts to delete Alice's entry -> 404
	res, _ = ts.do(http.MethodDelete, "/api/v1/journal?date=2026-10-20", tokenBob, nil)
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 for deleting non-existent entry, got %d", res.StatusCode)
	}
}

func TestJournalSearch(t *testing.T) {
	ts := newTestServer(t, generousRate)
	token, _ := registerAndLogin(t, ts, "searchuser", "search-pass-123")

	ts.do(http.MethodPut, "/api/v1/journal", token, models.SaveJournalRequest{
		Date:  "2026-10-18",
		Title: "Styling Charm",
		Body:  "Explored the Charm Lip Gloss styling library today.",
		Tags:  "frontend, design",
	})
	ts.do(http.MethodPut, "/api/v1/journal", token, models.SaveJournalRequest{
		Date:  "2026-10-19",
		Title: "Database Setup",
		Body:  "Implemented MariaDB prepared statements and migrations.",
		Tags:  "backend, go",
	})

	// Search by body: "Lip Gloss"
	res, body := ts.do(http.MethodGet, "/api/v1/journal/search?q=Lip+Gloss", token, nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("search failed (%d): %s", res.StatusCode, body)
	}
	var searchResp models.JournalSearchResponse
	json.Unmarshal(body, &searchResp)
	if len(searchResp.Matches) != 1 || searchResp.Matches[0].Date != "2026-10-18" {
		t.Fatalf("expected 1 match for Lip Gloss, got %+v", searchResp.Matches)
	}

	// Search by tag: "frontend"
	res, body = ts.do(http.MethodGet, "/api/v1/journal/search?q=frontend", token, nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("search by tag failed: %s", body)
	}
	var tagResp models.JournalSearchResponse
	json.Unmarshal(body, &tagResp)
	if len(tagResp.Matches) != 1 || tagResp.Matches[0].Date != "2026-10-18" {
		t.Fatalf("expected 1 match for tag 'frontend', got %+v", tagResp.Matches)
	}

	// Search by hashtag: "#design"
	res, body = ts.do(http.MethodGet, "/api/v1/journal/search?q=%23design", token, nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("search by hashtag failed: %s", body)
	}
	var hashResp models.JournalSearchResponse
	json.Unmarshal(body, &hashResp)
	if len(hashResp.Matches) != 1 || hashResp.Matches[0].Date != "2026-10-18" {
		t.Fatalf("expected 1 match for '#design', got %+v", hashResp.Matches)
	}

	// Search by title: "Database Setup"
	res, body = ts.do(http.MethodGet, "/api/v1/journal/search?q=Database+Setup", token, nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("search by title failed: %s", body)
	}
	var titleResp models.JournalSearchResponse
	json.Unmarshal(body, &titleResp)
	if len(titleResp.Matches) != 1 || titleResp.Matches[0].Date != "2026-10-19" {
		t.Fatalf("expected 1 match for title 'Database Setup', got %+v", titleResp.Matches)
	}

	// Search empty query -> 400
	res, _ = ts.do(http.MethodGet, "/api/v1/journal/search?q=", token, nil)
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for empty query, got %d", res.StatusCode)
	}
}

func TestJournalDeleteAndSyncSince(t *testing.T) {
	ts := newTestServer(t, generousRate)
	token, _ := registerAndLogin(t, ts, "syncjuser", "sync-pass-123")

	ts.do(http.MethodPut, "/api/v1/journal", token, models.SaveJournalRequest{
		Date: "2026-10-20",
		Body: "Temporary diary entry.",
	})

	time.Sleep(10 * time.Millisecond)
	sinceAnchor := time.Now().UTC().Format(time.RFC3339Nano)
	time.Sleep(10 * time.Millisecond)

	// Delete entry
	res, _ := ts.do(http.MethodDelete, "/api/v1/journal?date=2026-10-20", token, nil)
	if res.StatusCode != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", res.StatusCode)
	}

	// Verify get returns null
	res, body := ts.do(http.MethodGet, "/api/v1/journal?date=2026-10-20", token, nil)
	var getResp models.JournalResponse
	json.Unmarshal(body, &getResp)
	if getResp.Entry != nil {
		t.Fatalf("expected nil entry after delete, got %+v", getResp.Entry)
	}

	// Sync since -> should return deleted entry with deleted_at set
	res, body = ts.do(http.MethodGet, "/api/v1/journal?since="+sinceAnchor, token, nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 on sync, got %d", res.StatusCode)
	}
	var syncResp models.JournalListResponse
	json.Unmarshal(body, &syncResp)
	if len(syncResp.Entries) != 1 || syncResp.Entries[0].DeletedAt == nil {
		t.Fatalf("expected 1 deleted entry in sync, got %+v", syncResp.Entries)
	}
}
