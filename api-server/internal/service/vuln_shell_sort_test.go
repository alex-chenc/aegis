package service

import (
	"sort"
	"testing"
)

func TestAggregateShellGenerationRank(t *testing.T) {
	cases := []struct {
		status string
		want   int
	}{
		{"generated", 0},
		{"GENERATED", 0},
		{"generating", 1},
		{"none", 2},
		{"pending", 2},
		{"failed", 2},
		{"", 2},
	}
	for _, tc := range cases {
		if got := AggregateShellGenerationRank(tc.status); got != tc.want {
			t.Fatalf("AggregateShellGenerationRank(%q)=%d want %d", tc.status, got, tc.want)
		}
	}
}

func TestCompareAggregateShellStatusOrder(t *testing.T) {
	items := []string{"none", "generating", "generated", "pending", "failed", "generating"}
	sort.SliceStable(items, func(i, j int) bool {
		return CompareAggregateShellStatus(items[i], items[j]) < 0
	})
	wantPrefix := []string{"generated", "generating", "generating"}
	for i, w := range wantPrefix {
		if items[i] != w {
			t.Fatalf("items[%d]=%q want %q; full=%v", i, items[i], w, items)
		}
	}
	// remaining should all be rank 2
	for _, s := range items[3:] {
		if AggregateShellGenerationRank(s) != 2 {
			t.Fatalf("expected bottom ranks for %q in %v", s, items)
		}
	}
}

func TestHostShellGenerationRank(t *testing.T) {
	cases := []struct {
		status string
		want   int
	}{
		{"generated", 0},
		{"generating", 1},
		{"failed", 2},
		{"pending", 3},
		{"", 3},
	}
	for _, tc := range cases {
		if got := hostShellGenerationRank(tc.status); got != tc.want {
			t.Fatalf("hostShellGenerationRank(%q)=%d want %d", tc.status, got, tc.want)
		}
	}
}

func TestHostShellGenerationSortStable(t *testing.T) {
	type row struct {
		hostID string
		status string
	}
	rows := []row{
		{"h3", "pending"},
		{"h2", "generated"},
		{"h1", "generating"},
		{"h0", "failed"},
		{"h4", "generated"},
	}
	sort.SliceStable(rows, func(i, j int) bool {
		ri := hostShellGenerationRank(rows[i].status)
		rj := hostShellGenerationRank(rows[j].status)
		if ri != rj {
			return ri < rj
		}
		return rows[i].hostID < rows[j].hostID
	})
	want := []string{"h2", "h4", "h1", "h0", "h3"}
	for i, id := range want {
		if rows[i].hostID != id {
			t.Fatalf("rows[%d].hostID=%s want %s; full=%v", i, rows[i].hostID, id, rows)
		}
	}
}
