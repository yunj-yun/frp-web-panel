package certificate

import (
	"fmt"
	"net"
	"regexp"
	"sort"
	"strings"

	"github.com/VaalaCat/frp-panel/common"
	"github.com/VaalaCat/frp-panel/models"
	"github.com/VaalaCat/frp-panel/pb"
	"github.com/VaalaCat/frp-panel/services/app"
	"github.com/VaalaCat/frp-panel/services/dao"
)

var certificateDomainPattern = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?(?:\.[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?)+$`)

func ListManagedCertificates(ctx *app.Context, req *pb.ListManagedCertificatesRequest) (*pb.ListManagedCertificatesResponse, error) {
	userInfo := common.GetUserInfo(ctx)
	if !userInfo.Valid() {
		return certificateListError(pb.RespCode_RESP_CODE_UNAUTHORIZED, "user is not authorized"), nil
	}
	page, pageSize := certificatePage(req.GetPage(), req.GetPageSize())
	keyword := req.GetKeyword()
	query := dao.NewQuery(ctx)
	certificates, err := query.ListManagedCertificates(userInfo, page, pageSize, keyword)
	if err != nil {
		return nil, err
	}
	total, err := query.CountManagedCertificates(userInfo, keyword)
	if err != nil {
		return nil, err
	}
	items := make([]*pb.ManagedCertificate, 0, len(certificates))
	for _, certificate := range certificates {
		items = append(items, managedCertificateToPB(certificate))
	}
	return &pb.ListManagedCertificatesResponse{
		Status:       successStatus(),
		Certificates: items,
		Total:        uint32(total),
	}, nil
}

func CreateManagedCertificate(ctx *app.Context, req *pb.CreateManagedCertificateRequest) (*pb.CreateManagedCertificateResponse, error) {
	userInfo := common.GetUserInfo(ctx)
	if !userInfo.Valid() {
		return &pb.CreateManagedCertificateResponse{Status: status(pb.RespCode_RESP_CODE_UNAUTHORIZED, "user is not authorized")}, nil
	}
	if req == nil {
		return nil, fmt.Errorf("request is required")
	}
	domains, primaryDomain, err := normalizeDomains(req.GetDomains())
	if err != nil {
		return nil, err
	}
	challenge, err := normalizeChallenge(req.GetChallenge())
	if err != nil {
		return nil, err
	}
	serverID := strings.TrimSpace(req.GetServerId())
	if serverID == "" {
		return nil, fmt.Errorf("server id is required")
	}
	if _, err := dao.NewQuery(ctx).GetServerByServerID(userInfo, serverID); err != nil {
		return nil, fmt.Errorf("server is not available: %w", err)
	}
	name := strings.TrimSpace(req.GetName())
	if name == "" {
		name = primaryDomain
	}
	if len(name) > 128 {
		return nil, fmt.Errorf("certificate name is too long")
	}
	autoRenew := true
	if req.AutoRenew != nil {
		autoRenew = req.GetAutoRenew()
	}
	record := &models.ManagedCertificate{
		Name:          name,
		Domains:       domains,
		PrimaryDomain: primaryDomain,
		ServerID:      serverID,
		Challenge:     challenge,
		AutoRenew:     autoRenew,
		Status:        models.ManagedCertificateStatusPending,
	}
	if err := dao.NewMutation(ctx).CreateManagedCertificate(userInfo, record); err != nil {
		return nil, err
	}
	triggerGatewaySync(ctx, userInfo, serverID)
	return &pb.CreateManagedCertificateResponse{Status: successStatus(), Certificate: managedCertificateToPB(record)}, nil
}

func UpdateManagedCertificate(ctx *app.Context, req *pb.UpdateManagedCertificateRequest) (*pb.UpdateManagedCertificateResponse, error) {
	userInfo := common.GetUserInfo(ctx)
	if !userInfo.Valid() {
		return &pb.UpdateManagedCertificateResponse{Status: status(pb.RespCode_RESP_CODE_UNAUTHORIZED, "user is not authorized")}, nil
	}
	if req == nil || req.GetId() == 0 {
		return nil, fmt.Errorf("certificate id is required")
	}
	query := dao.NewQuery(ctx)
	existing, err := query.GetManagedCertificate(userInfo, uint(req.GetId()))
	if err != nil {
		return nil, err
	}
	domains, primaryDomain, err := normalizeDomains(req.GetDomains())
	if err != nil {
		return nil, err
	}
	challenge, err := normalizeChallenge(req.GetChallenge())
	if err != nil {
		return nil, err
	}
	serverID := strings.TrimSpace(req.GetServerId())
	if serverID == "" {
		return nil, fmt.Errorf("server id is required")
	}
	if _, err := query.GetServerByServerID(userInfo, serverID); err != nil {
		return nil, fmt.Errorf("server is not available: %w", err)
	}
	name := strings.TrimSpace(req.GetName())
	if name == "" {
		name = primaryDomain
	}
	if len(name) > 128 {
		return nil, fmt.Errorf("certificate name is too long")
	}
	autoRenew := existing.AutoRenew
	if req.AutoRenew != nil {
		autoRenew = req.GetAutoRenew()
	}
	oldServerID := existing.ServerID
	existing.Name = name
	existing.Domains = domains
	existing.PrimaryDomain = primaryDomain
	existing.ServerID = serverID
	existing.Challenge = challenge
	existing.AutoRenew = autoRenew
	if err := dao.NewMutation(ctx).UpdateManagedCertificate(userInfo, existing); err != nil {
		return nil, err
	}
	triggerGatewaySync(ctx, userInfo, oldServerID)
	if oldServerID != serverID {
		triggerGatewaySync(ctx, userInfo, serverID)
	}
	return &pb.UpdateManagedCertificateResponse{Status: successStatus(), Certificate: managedCertificateToPB(existing)}, nil
}

func DeleteManagedCertificate(ctx *app.Context, req *pb.DeleteManagedCertificateRequest) (*pb.DeleteManagedCertificateResponse, error) {
	userInfo := common.GetUserInfo(ctx)
	if !userInfo.Valid() {
		return &pb.DeleteManagedCertificateResponse{Status: status(pb.RespCode_RESP_CODE_UNAUTHORIZED, "user is not authorized")}, nil
	}
	if req == nil || req.GetId() == 0 {
		return nil, fmt.Errorf("certificate id is required")
	}
	certificate, err := dao.NewQuery(ctx).GetManagedCertificate(userInfo, uint(req.GetId()))
	if err != nil {
		return nil, err
	}
	if count, err := dao.NewQuery(ctx).CountGatewayRoutesByCertificate(userInfo, uint(req.GetId())); err != nil {
		return nil, err
	} else if count > 0 {
		return nil, fmt.Errorf("certificate is still used by %d gateway route(s)", count)
	}
	if err := dao.NewMutation(ctx).DeleteManagedCertificate(userInfo, uint(req.GetId())); err != nil {
		return nil, err
	}
	triggerGatewaySync(ctx, userInfo, certificate.ServerID)
	return &pb.DeleteManagedCertificateResponse{Status: successStatus()}, nil
}

func RequestCertificate(ctx *app.Context, req *pb.RequestCertificateRequest) (*pb.RequestCertificateResponse, error) {
	userInfo := common.GetUserInfo(ctx)
	if !userInfo.Valid() {
		return &pb.RequestCertificateResponse{Status: status(pb.RespCode_RESP_CODE_UNAUTHORIZED, "user is not authorized")}, nil
	}
	if req == nil || req.GetCertificateId() == 0 {
		return nil, fmt.Errorf("certificate id is required")
	}
	if _, err := dao.NewQuery(ctx).GetManagedCertificate(userInfo, uint(req.GetCertificateId())); err != nil {
		return nil, err
	}
	job, err := dao.NewMutation(ctx).QueueCertificateJob(userInfo, uint(req.GetCertificateId()), models.CertificateJobActionIssue)
	if err != nil {
		return nil, err
	}
	return &pb.RequestCertificateResponse{Status: successStatus(), Job: certificateJobToPB(job)}, nil
}

func ListCertificateJobs(ctx *app.Context, req *pb.ListCertificateJobsRequest) (*pb.ListCertificateJobsResponse, error) {
	userInfo := common.GetUserInfo(ctx)
	if !userInfo.Valid() {
		return &pb.ListCertificateJobsResponse{Status: status(pb.RespCode_RESP_CODE_UNAUTHORIZED, "user is not authorized")}, nil
	}
	if req == nil || req.GetCertificateId() == 0 {
		return nil, fmt.Errorf("certificate id is required")
	}
	query := dao.NewQuery(ctx)
	if _, err := query.GetManagedCertificate(userInfo, uint(req.GetCertificateId())); err != nil {
		return nil, err
	}
	page, pageSize := certificatePage(req.GetPage(), req.GetPageSize())
	jobs, err := query.ListCertificateJobs(userInfo, uint(req.GetCertificateId()), page, pageSize)
	if err != nil {
		return nil, err
	}
	total, err := query.CountCertificateJobs(userInfo, uint(req.GetCertificateId()))
	if err != nil {
		return nil, err
	}
	items := make([]*pb.CertificateJob, 0, len(jobs))
	for _, job := range jobs {
		items = append(items, certificateJobToPB(job))
	}
	return &pb.ListCertificateJobsResponse{Status: successStatus(), Jobs: items, Total: uint32(total)}, nil
}

func certificatePage(page, pageSize uint32) (int, int) {
	if page == 0 {
		page = 1
	}
	if pageSize == 0 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	return int(page), int(pageSize)
}

func normalizeChallenge(challenge string) (string, error) {
	challenge = strings.ToLower(strings.TrimSpace(challenge))
	if challenge == "" {
		challenge = models.CertificateChallengeHTTP01
	}
	if challenge != models.CertificateChallengeHTTP01 && challenge != models.CertificateChallengeTLSALPN01 {
		return "", fmt.Errorf("unsupported certificate challenge")
	}
	return challenge, nil
}

func normalizeDomains(input []string) (models.GormArray[string], string, error) {
	seen := make(map[string]struct{}, len(input))
	domains := make(models.GormArray[string], 0, len(input))
	for _, raw := range input {
		domain := strings.TrimSpace(raw)
		domain = strings.TrimSuffix(domain, ".")
		domain = strings.ToLower(strings.TrimSpace(domain))
		if domain == "" {
			continue
		}
		if net.ParseIP(domain) != nil || strings.HasPrefix(domain, "*.") || !certificateDomainPattern.MatchString(domain) {
			return nil, "", fmt.Errorf("invalid certificate domain %q", raw)
		}
		if _, ok := seen[domain]; ok {
			continue
		}
		seen[domain] = struct{}{}
		domains = append(domains, domain)
	}
	if len(domains) == 0 {
		return nil, "", fmt.Errorf("at least one certificate domain is required")
	}
	if len(domains) > 10 {
		return nil, "", fmt.Errorf("at most 10 certificate domains are supported")
	}
	// Keep the request deterministic so updates do not create needless agent
	// changes when the user enters SANs in a different order.
	sort.Strings(domains)
	return domains, domains[0], nil
}

func status(code pb.RespCode, message string) *pb.Status {
	return &pb.Status{Code: code, Message: message}
}

func successStatus() *pb.Status { return status(pb.RespCode_RESP_CODE_SUCCESS, "ok") }

func certificateListError(code pb.RespCode, message string) *pb.ListManagedCertificatesResponse {
	return &pb.ListManagedCertificatesResponse{Status: status(code, message), Certificates: []*pb.ManagedCertificate{}}
}

func managedCertificateToPB(certificate *models.ManagedCertificate) *pb.ManagedCertificate {
	if certificate == nil {
		return nil
	}
	item := &pb.ManagedCertificate{
		Id:            uint32(certificate.ID),
		Name:          certificate.Name,
		Domains:       append([]string(nil), certificate.Domains...),
		PrimaryDomain: certificate.PrimaryDomain,
		ServerId:      certificate.ServerID,
		Challenge:     certificate.Challenge,
		AutoRenew:     certificate.AutoRenew,
		Issuer:        certificate.Issuer,
		Fingerprint:   certificate.Fingerprint,
		StorageId:     certificate.StorageID,
		Status:        certificate.Status,
		ErrorMessage:  certificate.ErrorMessage,
		CreatedAt:     certificate.CreatedAt.UnixMilli(),
		UpdatedAt:     certificate.UpdatedAt.UnixMilli(),
	}
	if certificate.IssuedAt != nil {
		item.IssuedAt = certificate.IssuedAt.UnixMilli()
	}
	if certificate.ExpiresAt != nil {
		item.ExpiresAt = certificate.ExpiresAt.UnixMilli()
	}
	return item
}

func certificateJobToPB(job *models.CertificateJob) *pb.CertificateJob {
	if job == nil {
		return nil
	}
	item := &pb.CertificateJob{
		Id:            uint32(job.ID),
		CertificateId: uint32(job.CertificateID),
		Action:        job.Action,
		Status:        job.Status,
		ErrorMessage:  job.ErrorMessage,
		Attempts:      uint32(job.Attempts),
		CreatedAt:     job.CreatedAt.UnixMilli(),
		UpdatedAt:     job.UpdatedAt.UnixMilli(),
	}
	if job.StartedAt != nil {
		item.StartedAt = job.StartedAt.UnixMilli()
	}
	if job.FinishedAt != nil {
		item.FinishedAt = job.FinishedAt.UnixMilli()
	}
	return item
}
