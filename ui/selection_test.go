package ui

import (
	"testing"

	"0xADE/xpass/passcard"

	"gioui.org/layout"
	"gioui.org/widget"
)

func TestReselectAfterFilterChange_prefersPathWhenListReordered(t *testing.T) {
	ui := &UI{
		filtered: []passcard.StoredItem{
			{Name: "first", Path: "/store/a.gpg"},
			{Name: "second", Path: "/store/b.gpg"},
		},
		selectedIdx: 1,
	}
	ui.reselectAfterFilterChange("/store/b.gpg")
	if ui.selectedIdx != 1 {
		t.Fatalf("selectedIdx=%d want 1 (path still at same index)", ui.selectedIdx)
	}

	ui.filtered = []passcard.StoredItem{
		{Name: "second", Path: "/store/b.gpg"},
		{Name: "first", Path: "/store/a.gpg"},
	}
	ui.reselectAfterFilterChange("/store/b.gpg")
	if ui.selectedIdx != 0 {
		t.Fatalf("selectedIdx=%d want 0 after reorder", ui.selectedIdx)
	}
}

func TestReselectAfterFilterChange_missingPreferredResetsToZero(t *testing.T) {
	ui := &UI{
		filtered: []passcard.StoredItem{
			{Name: "only", Path: "/store/z.gpg"},
		},
		selectedIdx: 0,
	}
	ui.reselectAfterFilterChange("/store/missing.gpg")
	if ui.selectedIdx != 0 {
		t.Fatalf("selectedIdx=%d want 0", ui.selectedIdx)
	}
}

func TestReselectAfterFilterChange_emptyPreferredClampsIndex(t *testing.T) {
	ui := &UI{
		filtered:    []passcard.StoredItem{{Name: "a", Path: "/a.gpg"}},
		selectedIdx: 99,
	}
	ui.reselectAfterFilterChange("")
	if ui.selectedIdx != 0 {
		t.Fatalf("selectedIdx=%d want 0", ui.selectedIdx)
	}

	ui.filtered = []passcard.StoredItem{
		{Name: "a", Path: "/a.gpg"},
		{Name: "b", Path: "/b.gpg"},
	}
	ui.selectedIdx = 1
	ui.reselectAfterFilterChange("")
	if ui.selectedIdx != 1 {
		t.Fatalf("selectedIdx=%d want 1", ui.selectedIdx)
	}
}

func TestEnsureSelectedVisible_clampsAndScrollsWindow(t *testing.T) {
	ui := &UI{
		filtered: []passcard.StoredItem{
			{Name: "0", Path: "/0.gpg"},
			{Name: "1", Path: "/1.gpg"},
			{Name: "2", Path: "/2.gpg"},
			{Name: "3", Path: "/3.gpg"},
		},
		selectedIdx: 3,
		list: widget.List{
			List: layout.List{
				Axis: layout.Vertical,
			},
		},
	}
	ui.list.Position.First = 0
	ui.list.Position.Count = 2

	ui.ensureSelectedVisible()
	if ui.list.Position.First != 2 {
		t.Fatalf("Position.First=%d want 2 (show rows 2 and 3)", ui.list.Position.First)
	}

	ui.list.Position.First = 3
	ui.selectedIdx = 1
	ui.ensureSelectedVisible()
	if ui.list.Position.First != 1 {
		t.Fatalf("Position.First=%d want 1", ui.list.Position.First)
	}
}

func TestApplyDebouncedFilterResultIfCurrent_staleIgnored(t *testing.T) {
	ui := &UI{
		query: "current",
		filtered: []passcard.StoredItem{
			{Name: "keep", Path: "/keep.gpg"},
		},
		selectedIdx: 0,
	}
	stale := debouncedFilterResult{
		query: "old",
		items: []passcard.StoredItem{
			{Name: "wrong", Path: "/wrong.gpg"},
		},
	}
	if ui.applyDebouncedFilterResultIfCurrent(stale) {
		t.Fatal("expected stale result to be ignored")
	}
	if len(ui.filtered) != 1 || ui.filtered[0].Path != "/keep.gpg" {
		t.Fatalf("filtered=%v want single /keep.gpg", ui.filtered)
	}
}

func TestApplyDebouncedFilterResultIfCurrent_appliesWhenQueryMatches(t *testing.T) {
	ui := &UI{
		query: "ab",
		filtered: []passcard.StoredItem{
			{Name: "a", Path: "/a.gpg"},
		},
		selectedIdx: 0,
	}
	ui.list = widget.List{List: layout.List{Axis: layout.Vertical}}

	incoming := debouncedFilterResult{
		query: "ab",
		items: []passcard.StoredItem{
			{Name: "ab", Path: "/ab.gpg"},
			{Name: "a", Path: "/a.gpg"},
		},
	}
	if !ui.applyDebouncedFilterResultIfCurrent(incoming) {
		t.Fatal("expected result to apply")
	}
	if ui.selectedPath() != "/a.gpg" || ui.selectedIdx != 1 {
		t.Fatalf("selectedIdx=%d path=%q want index 1 /a.gpg", ui.selectedIdx, ui.selectedPath())
	}
}
