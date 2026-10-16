package proxy

import (
	"testing"
	"time"

	"github.com/VaalaCat/frp-panel/models"
)

func TestTrafficHistoryDeltasUsesBaselineOutsideRange(t *testing.T) {
	base := time.Date(2026, 10, 7, 0, 0, 0, 0, time.FixedZone("CST", 8*60*60))
	rows := []*models.HistoryProxyStats{
		{ProxyID: 1, Name: "web", ClientID: "c1", ServerID: "s1", TrafficIn: 100, TrafficOut: 40, StatDate: base},
		{ProxyID: 1, Name: "web", ClientID: "c1", ServerID: "s1", TrafficIn: 160, TrafficOut: 55, StatDate: base.AddDate(0, 0, 1)},
		{ProxyID: 1, Name: "web", ClientID: "c1", ServerID: "s1", TrafficIn: 220, TrafficOut: 75, StatDate: base.AddDate(0, 0, 2)},
	}

	items, totalIn, totalOut := trafficHistoryDeltas(rows, base.AddDate(0, 0, 1), base.AddDate(0, 0, 3))
	if len(items) != 2 || totalIn != 120 || totalOut != 35 {
		t.Fatalf("unexpected range delta: items=%d in=%d out=%d", len(items), totalIn, totalOut)
	}
	if items[0].TrafficIn != 60 || items[0].TrafficOut != 15 || items[1].TrafficIn != 60 || items[1].TrafficOut != 20 {
		t.Fatalf("unexpected daily values: %+v", items)
	}
}

func TestTrafficHistoryDeltasTreatsCounterResetAsNewSegment(t *testing.T) {
	base := time.Date(2026, 10, 7, 0, 0, 0, 0, time.UTC)
	rows := []*models.HistoryProxyStats{
		{ProxyID: 2, Name: "tcp", TrafficIn: 900, TrafficOut: 800, StatDate: base},
		{ProxyID: 2, Name: "tcp", TrafficIn: 25, TrafficOut: 10, StatDate: base.AddDate(0, 0, 1)},
	}

	items, totalIn, totalOut := trafficHistoryDeltas(rows, base, base.AddDate(0, 0, 2))
	if len(items) != 2 || totalIn != 925 || totalOut != 810 {
		t.Fatalf("reset should start a new segment: items=%d in=%d out=%d", len(items), totalIn, totalOut)
	}
	if items[1].TrafficIn != 25 || items[1].TrafficOut != 10 {
		t.Fatalf("unexpected reset delta: %+v", items[1])
	}
}

func TestTrafficHistoryDeltasSortsSnapshotsBeforeDiffing(t *testing.T) {
	base := time.Date(2026, 10, 7, 0, 0, 0, 0, time.UTC)
	rows := []*models.HistoryProxyStats{
		{ProxyID: 3, Name: "udp", TrafficIn: 180, TrafficOut: 90, StatDate: base.AddDate(0, 0, 2)},
		{ProxyID: 3, Name: "udp", TrafficIn: 100, TrafficOut: 50, StatDate: base},
		{ProxyID: 3, Name: "udp", TrafficIn: 140, TrafficOut: 70, StatDate: base.AddDate(0, 0, 1)},
	}

	items, totalIn, totalOut := trafficHistoryDeltas(rows, base, base.AddDate(0, 0, 3))
	if len(items) != 3 || totalIn != 180 || totalOut != 90 {
		t.Fatalf("unexpected sorted delta: items=%d in=%d out=%d", len(items), totalIn, totalOut)
	}
	if items[1].TrafficIn != 40 || items[2].TrafficIn != 40 {
		t.Fatalf("snapshots were not sorted before diffing: %+v", items)
	}
}
