package proxy

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/VaalaCat/frp-panel/common"
	"github.com/VaalaCat/frp-panel/models"
	"github.com/VaalaCat/frp-panel/pb"
	"github.com/VaalaCat/frp-panel/services/app"
	"github.com/VaalaCat/frp-panel/services/dao"
)

const maxTrafficHistoryRange = 366 * 24 * time.Hour

type trafficHistoryKey struct {
	proxyID  int
	name     string
	clientID string
	serverID string
}

type trafficHistoryAggregate struct {
	item    *pb.TrafficHistoryItem
	lastIn  int64
	lastOut int64
}

// trafficHistoryDeltas is deliberately independent of the database and API
// context. It consumes cumulative snapshots in any input order, keeps a
// baseline outside the requested range, and turns counter resets into a new
// segment instead of reporting a negative value.
func trafficHistoryDeltas(rows []*models.HistoryProxyStats, start, end time.Time) ([]*pb.TrafficHistoryItem, int64, int64) {
	filteredRows := make([]*models.HistoryProxyStats, 0, len(rows))
	for _, row := range rows {
		if row != nil {
			filteredRows = append(filteredRows, row)
		}
	}
	rows = filteredRows
	sort.SliceStable(rows, func(i, j int) bool {
		leftAt := historySnapshotTime(rows[i])
		rightAt := historySnapshotTime(rows[j])
		if !leftAt.Equal(rightAt) {
			return leftAt.Before(rightAt)
		}
		if rows[i].ProxyID != rows[j].ProxyID {
			return rows[i].ProxyID < rows[j].ProxyID
		}
		if !rows[i].CreatedAt.Equal(rows[j].CreatedAt) {
			return rows[i].CreatedAt.Before(rows[j].CreatedAt)
		}
		return rows[i].ID < rows[j].ID
	})

	state := make(map[trafficHistoryKey]*trafficHistoryAggregate)
	items := make([]*pb.TrafficHistoryItem, 0, len(rows))
	for _, row := range rows {
		if row == nil {
			continue
		}
		key := trafficHistoryKey{proxyID: row.ProxyID, name: row.Name, clientID: row.ClientID, serverID: row.ServerID}
		previous := state[key]
		current := &trafficHistoryAggregate{lastIn: row.TrafficIn, lastOut: row.TrafficOut}
		trafficIn, trafficOut := row.TrafficIn, row.TrafficOut
		if previous != nil {
			trafficIn = row.TrafficIn - previous.lastIn
			trafficOut = row.TrafficOut - previous.lastOut
			if trafficIn < 0 {
				trafficIn = row.TrafficIn
			}
			if trafficOut < 0 {
				trafficOut = row.TrafficOut
			}
		}
		statAt := historySnapshotTime(row)
		current.item = &pb.TrafficHistoryItem{
			ProxyId: uint32(row.ProxyID), Name: row.Name, Type: row.Type,
			ClientId: row.ClientID, ServerId: row.ServerID,
			TrafficIn: trafficIn, TrafficOut: trafficOut, StatAt: statAt.UnixMilli(),
		}
		state[key] = current
		if statAt.Before(start) || !statAt.Before(end) {
			continue
		}
		items = append(items, current.item)
	}

	sort.SliceStable(items, func(i, j int) bool {
		if items[i].StatAt != items[j].StatAt {
			return items[i].StatAt < items[j].StatAt
		}
		return items[i].Name < items[j].Name
	})
	var totalIn, totalOut int64
	for _, item := range items {
		totalIn += item.TrafficIn
		totalOut += item.TrafficOut
	}
	return items, totalIn, totalOut
}

func historySnapshotTime(row *models.HistoryProxyStats) time.Time {
	if row == nil {
		return time.Time{}
	}
	if !row.StatDate.IsZero() {
		return row.StatDate
	}
	return row.CreatedAt
}

// ListTrafficHistory converts the cumulative snapshots already collected by
// the FRP server into daily deltas for a half-open date range.
func ListTrafficHistory(c *app.Context, req *pb.ListTrafficHistoryRequest) (*pb.ListTrafficHistoryResponse, error) {
	if req == nil {
		return nil, fmt.Errorf("request is required")
	}
	userInfo := common.GetUserInfo(c)
	if !userInfo.Valid() {
		return &pb.ListTrafficHistoryResponse{Status: &pb.Status{Code: pb.RespCode_RESP_CODE_UNAUTHORIZED, Message: "user is not authorized"}}, nil
	}
	clientID := strings.TrimSpace(req.GetClientId())
	serverID := strings.TrimSpace(req.GetServerId())
	if clientID == "" && serverID == "" && req.GetProxyId() == 0 {
		return nil, fmt.Errorf("a client, server or proxy scope is required")
	}
	start, end, err := trafficHistoryRange(req)
	if err != nil {
		return nil, err
	}
	rows, err := dao.NewQuery(c).ListHistoryStatsForRange(userInfo, clientID, serverID, uint(req.GetProxyId()), start, end)
	if err != nil {
		return nil, err
	}

	items, totalIn, totalOut := trafficHistoryDeltas(rows, start, end)
	return &pb.ListTrafficHistoryResponse{Status: &pb.Status{Code: pb.RespCode_RESP_CODE_SUCCESS, Message: "ok"}, Items: items, TotalTrafficIn: totalIn, TotalTrafficOut: totalOut}, nil
}

func trafficHistoryRange(req *pb.ListTrafficHistoryRequest) (time.Time, time.Time, error) {
	end := time.Now()
	if req.EndAt != nil && req.GetEndAt() > 0 {
		end = time.UnixMilli(req.GetEndAt())
	}
	start := end.Add(-30 * 24 * time.Hour)
	if req.StartAt != nil && req.GetStartAt() > 0 {
		start = time.UnixMilli(req.GetStartAt())
	}
	if end.Before(start) || end.Equal(start) {
		return time.Time{}, time.Time{}, fmt.Errorf("traffic history end must be after start")
	}
	if end.Sub(start) > maxTrafficHistoryRange {
		return time.Time{}, time.Time{}, fmt.Errorf("traffic history range cannot exceed 366 days")
	}
	return start, end, nil
}
