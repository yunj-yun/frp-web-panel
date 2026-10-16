package dao

import "github.com/yunj-yun/frp-web-panel/services/app"

type Query interface {
	CertQuery
	GatewayRouteQuery
	ManagedCertificateQuery
	ClientQuery
	PortSnapshotQuery
	PortReservationQuery
	ProxyQuery
	ServerQuery
	StatsQuery
	UserQuery
	WorkerQuery
}

type Mutation interface {
	CertMutation
	GatewayRouteMutation
	ManagedCertificateMutation
	ClientMutation
	PortSnapshotMutation
	PortReservationMutation
	ProxyMutation
	ServerMutation
	StatsMutation
	UserMutation
	WorkerMutation
	UserGroupMutation
}

// queryImpl/mutationImpl 是具体表级实现的基础结构（持有 ctx）。
type queryImpl struct {
	ctx *app.Context
}

type mutationImpl struct {
	ctx *app.Context
}

// compositeQuery / compositeMutation 组合各子领域实现，对外暴露统一入口。
type compositeQuery struct {
	CertQuery
	GatewayRouteQuery
	ManagedCertificateQuery
	ClientQuery
	PortSnapshotQuery
	PortReservationQuery
	ProxyQuery
	ServerQuery
	StatsQuery
	UserQuery
	WorkerQuery
}

type compositeMutation struct {
	CertMutation
	GatewayRouteMutation
	ManagedCertificateMutation
	ClientMutation
	PortSnapshotMutation
	PortReservationMutation
	ProxyMutation
	ServerMutation
	StatsMutation
	UserMutation
	WorkerMutation
	UserGroupMutation
}

func NewQuery(ctx *app.Context) Query {
	base := &queryImpl{ctx: ctx}
	return &compositeQuery{
		CertQuery:               newCertQuery(base),
		GatewayRouteQuery:       newGatewayRouteQuery(base),
		ManagedCertificateQuery: newManagedCertificateQuery(base),
		ClientQuery:             newClientQuery(base),
		PortSnapshotQuery:       newPortSnapshotQuery(base),
		PortReservationQuery:    newPortReservationQuery(base),
		ProxyQuery:              newProxyQuery(base),
		ServerQuery:             newServerQuery(base),
		StatsQuery:              newStatsQuery(base),
		UserQuery:               newUserQuery(base),
		WorkerQuery:             newWorkerQuery(base),
	}
}

func NewMutation(ctx *app.Context) Mutation {
	base := &mutationImpl{ctx: ctx}
	return &compositeMutation{
		CertMutation:               newCertMutation(base),
		GatewayRouteMutation:       newGatewayRouteMutation(base),
		ManagedCertificateMutation: newManagedCertificateMutation(base),
		ClientMutation:             newClientMutation(base),
		PortSnapshotMutation:       newPortSnapshotMutation(base),
		PortReservationMutation:    newPortReservationMutation(base),
		ProxyMutation:              newProxyMutation(base),
		ServerMutation:             newServerMutation(base),
		StatsMutation:              newStatsMutation(base),
		UserMutation:               newUserMutation(base),
		WorkerMutation:             newWorkerMutation(base),
		UserGroupMutation:          newUserGroupMutation(base),
	}
}

var (
	_ Query    = (*compositeQuery)(nil)
	_ Mutation = (*compositeMutation)(nil)
)
