package dao

import (
	"fmt"
	"strings"
	"time"

	"github.com/VaalaCat/frp-panel/models"
	"gorm.io/gorm"
)

type ManagedCertificateQuery interface {
	ListManagedCertificates(userInfo models.UserInfo, page, pageSize int, keyword string) ([]*models.ManagedCertificate, error)
	CountManagedCertificates(userInfo models.UserInfo, keyword string) (int64, error)
	GetManagedCertificate(userInfo models.UserInfo, id uint) (*models.ManagedCertificate, error)
	ListCertificateJobs(userInfo models.UserInfo, certificateID uint, page, pageSize int) ([]*models.CertificateJob, error)
	CountCertificateJobs(userInfo models.UserInfo, certificateID uint) (int64, error)
	AdminListRunnableCertificateJobs(limit int) ([]*models.CertificateJob, error)
	AdminGetManagedCertificate(certificateID uint) (*models.ManagedCertificate, error)
	AdminListManagedCertificatesByServer(userID, tenantID int, serverID string) ([]*models.ManagedCertificate, error)
	AdminListManagedCertificates() ([]*models.ManagedCertificate, error)
}

type ManagedCertificateMutation interface {
	CreateManagedCertificate(userInfo models.UserInfo, certificate *models.ManagedCertificate) error
	UpdateManagedCertificate(userInfo models.UserInfo, certificate *models.ManagedCertificate) error
	DeleteManagedCertificate(userInfo models.UserInfo, id uint) error
	QueueCertificateJob(userInfo models.UserInfo, certificateID uint, action string) (*models.CertificateJob, error)
	AdminClaimCertificateJob(jobID uint, now time.Time) (*models.CertificateJob, error)
	AdminCompleteCertificateJob(jobID, certificateID uint, userID, tenantID int, status, errorMessage string, issuedAt *time.Time) error
	AdminUpdateManagedCertificateStatus(certificateID uint, userID, tenantID int, status, errorMessage, issuer, fingerprint, storageID string, issuedAt, expiresAt *time.Time) error
}

type managedCertificateQuery struct{ *queryImpl }
type managedCertificateMutation struct{ *mutationImpl }

func newManagedCertificateQuery(base *queryImpl) ManagedCertificateQuery {
	return &managedCertificateQuery{base}
}

func newManagedCertificateMutation(base *mutationImpl) ManagedCertificateMutation {
	return &managedCertificateMutation{base}
}

func certificateScope(userInfo models.UserInfo) *models.ManagedCertificate {
	return &models.ManagedCertificate{UserID: userInfo.GetUserID(), TenantID: userInfo.GetTenantID()}
}

func jobScope(userInfo models.UserInfo) *models.CertificateJob {
	return &models.CertificateJob{UserID: userInfo.GetUserID(), TenantID: userInfo.GetTenantID()}
}

func (q *managedCertificateQuery) ListManagedCertificates(userInfo models.UserInfo, page, pageSize int, keyword string) ([]*models.ManagedCertificate, error) {
	if page < 1 || pageSize < 1 || pageSize > 100 {
		return nil, fmt.Errorf("invalid page or page size")
	}

	db := q.ctx.GetApp().GetDBManager().GetDefaultDB().Where(certificateScope(userInfo))
	if keyword = strings.TrimSpace(keyword); keyword != "" {
		pattern := "%" + keyword + "%"
		db = db.Where("name LIKE ? OR primary_domain LIKE ?", pattern, pattern)
	}

	var certificates []*models.ManagedCertificate
	err := db.Order("created_at DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&certificates).Error
	return certificates, err
}

func (q *managedCertificateQuery) CountManagedCertificates(userInfo models.UserInfo, keyword string) (int64, error) {
	db := q.ctx.GetApp().GetDBManager().GetDefaultDB().Model(&models.ManagedCertificate{}).Where(certificateScope(userInfo))
	if keyword = strings.TrimSpace(keyword); keyword != "" {
		pattern := "%" + keyword + "%"
		db = db.Where("name LIKE ? OR primary_domain LIKE ?", pattern, pattern)
	}
	var count int64
	err := db.Count(&count).Error
	return count, err
}

func (q *managedCertificateQuery) GetManagedCertificate(userInfo models.UserInfo, id uint) (*models.ManagedCertificate, error) {
	if id == 0 {
		return nil, fmt.Errorf("invalid certificate id")
	}
	certificate := &models.ManagedCertificate{}
	err := q.ctx.GetApp().GetDBManager().GetDefaultDB().Where(certificateScope(userInfo)).First(certificate, id).Error
	if err != nil {
		return nil, err
	}
	return certificate, nil
}

func (q *managedCertificateQuery) ListCertificateJobs(userInfo models.UserInfo, certificateID uint, page, pageSize int) ([]*models.CertificateJob, error) {
	if certificateID == 0 || page < 1 || pageSize < 1 || pageSize > 100 {
		return nil, fmt.Errorf("invalid certificate id, page or page size")
	}
	db := q.ctx.GetApp().GetDBManager().GetDefaultDB().Where(jobScope(userInfo)).Where("certificate_id = ?", certificateID)
	var jobs []*models.CertificateJob
	err := db.Order("created_at DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&jobs).Error
	return jobs, err
}

func (q *managedCertificateQuery) CountCertificateJobs(userInfo models.UserInfo, certificateID uint) (int64, error) {
	if certificateID == 0 {
		return 0, fmt.Errorf("invalid certificate id")
	}
	var count int64
	err := q.ctx.GetApp().GetDBManager().GetDefaultDB().Model(&models.CertificateJob{}).
		Where(jobScope(userInfo)).Where("certificate_id = ?", certificateID).Count(&count).Error
	return count, err
}

// AdminListRunnableCertificateJobs is intentionally not tenant scoped: the
// master scheduler is the only caller and must service every tenant. The
// caller still receives the tenant/user identifiers stored on each job and
// uses them when loading all related records.
func (q *managedCertificateQuery) AdminListRunnableCertificateJobs(limit int) ([]*models.CertificateJob, error) {
	if limit < 1 || limit > 200 {
		return nil, fmt.Errorf("invalid certificate job limit")
	}
	var jobs []*models.CertificateJob
	err := q.ctx.GetApp().GetDBManager().GetDefaultDB().Where("status = ?", models.CertificateJobStatusQueued).
		Order("created_at ASC").Limit(limit).Find(&jobs).Error
	return jobs, err
}

func (q *managedCertificateQuery) AdminGetManagedCertificate(certificateID uint) (*models.ManagedCertificate, error) {
	if certificateID == 0 {
		return nil, fmt.Errorf("invalid certificate id")
	}
	certificate := &models.ManagedCertificate{}
	if err := q.ctx.GetApp().GetDBManager().GetDefaultDB().First(certificate, certificateID).Error; err != nil {
		return nil, err
	}
	return certificate, nil
}

func (q *managedCertificateQuery) AdminListManagedCertificatesByServer(userID, tenantID int, serverID string) ([]*models.ManagedCertificate, error) {
	if userID == 0 || tenantID < 0 || strings.TrimSpace(serverID) == "" {
		return nil, fmt.Errorf("user, tenant and server are required")
	}
	var certificates []*models.ManagedCertificate
	err := q.ctx.GetApp().GetDBManager().GetDefaultDB().Where(&models.ManagedCertificate{
		UserID: userID, TenantID: tenantID, ServerID: strings.TrimSpace(serverID),
	}).Where("status <> ?", models.ManagedCertificateStatusRevoked).Order("primary_domain ASC").Find(&certificates).Error
	return certificates, err
}

func (q *managedCertificateQuery) AdminListManagedCertificates() ([]*models.ManagedCertificate, error) {
	var certificates []*models.ManagedCertificate
	err := q.ctx.GetApp().GetDBManager().GetDefaultDB().Where("status <> ?", models.ManagedCertificateStatusRevoked).Order("server_id ASC, primary_domain ASC").Find(&certificates).Error
	return certificates, err
}

func (m *managedCertificateMutation) CreateManagedCertificate(userInfo models.UserInfo, certificate *models.ManagedCertificate) error {
	if certificate == nil {
		return fmt.Errorf("certificate is required")
	}
	certificate.UserID = userInfo.GetUserID()
	certificate.TenantID = userInfo.GetTenantID()
	return m.ctx.GetApp().GetDBManager().GetDefaultDB().Create(certificate).Error
}

func (m *managedCertificateMutation) UpdateManagedCertificate(userInfo models.UserInfo, certificate *models.ManagedCertificate) error {
	if certificate == nil || certificate.ID == 0 {
		return fmt.Errorf("certificate is required")
	}
	db := m.ctx.GetApp().GetDBManager().GetDefaultDB()
	var existing models.ManagedCertificate
	if err := db.Where(certificateScope(userInfo)).First(&existing, certificate.ID).Error; err != nil {
		return err
	}
	existing.Name = certificate.Name
	existing.Domains = certificate.Domains
	existing.PrimaryDomain = certificate.PrimaryDomain
	existing.ServerID = certificate.ServerID
	existing.Challenge = certificate.Challenge
	existing.AutoRenew = certificate.AutoRenew
	existing.Status = models.ManagedCertificateStatusPending
	existing.Issuer = ""
	existing.Fingerprint = ""
	existing.StorageID = ""
	existing.ErrorMessage = ""
	existing.IssuedAt = nil
	existing.ExpiresAt = nil
	return db.Save(&existing).Error
}

func (m *managedCertificateMutation) DeleteManagedCertificate(userInfo models.UserInfo, id uint) error {
	if id == 0 {
		return fmt.Errorf("invalid certificate id")
	}
	db := m.ctx.GetApp().GetDBManager().GetDefaultDB()
	var certificate models.ManagedCertificate
	if err := db.Where(certificateScope(userInfo)).First(&certificate, id).Error; err != nil {
		return err
	}
	return db.Delete(&certificate).Error
}

func (m *managedCertificateMutation) QueueCertificateJob(userInfo models.UserInfo, certificateID uint, action string) (*models.CertificateJob, error) {
	if certificateID == 0 || action == "" {
		return nil, fmt.Errorf("invalid certificate job")
	}
	db := m.ctx.GetApp().GetDBManager().GetDefaultDB()
	var job models.CertificateJob
	err := db.Transaction(func(tx *gorm.DB) error {
		var certificate models.ManagedCertificate
		if err := tx.Where(certificateScope(userInfo)).First(&certificate, certificateID).Error; err != nil {
			return err
		}
		if err := tx.Where(jobScope(userInfo)).Where("certificate_id = ? AND status IN ?", certificateID, []string{
			models.CertificateJobStatusQueued,
			models.CertificateJobStatusRunning,
		}).Order("created_at DESC").First(&job).Error; err == nil {
			return nil
		} else if err != gorm.ErrRecordNotFound {
			return err
		}

		certificate.Status = models.ManagedCertificateStatusPending
		certificate.ErrorMessage = ""
		if err := tx.Save(&certificate).Error; err != nil {
			return err
		}
		job = models.CertificateJob{
			UserID:        userInfo.GetUserID(),
			TenantID:      userInfo.GetTenantID(),
			CertificateID: certificateID,
			Action:        action,
			Status:        models.CertificateJobStatusQueued,
		}
		return tx.Create(&job).Error
	})
	if err != nil {
		return nil, err
	}
	return &job, nil
}

// AdminClaimCertificateJob atomically moves a queued job to running. The
// conditional update prevents two scheduler ticks (or two Master replicas)
// from applying the same Caddy configuration concurrently.
func (m *managedCertificateMutation) AdminClaimCertificateJob(jobID uint, now time.Time) (*models.CertificateJob, error) {
	if jobID == 0 {
		return nil, fmt.Errorf("invalid certificate job id")
	}
	db := m.ctx.GetApp().GetDBManager().GetDefaultDB()
	result := db.Model(&models.CertificateJob{}).Where("id = ? AND status = ?", jobID, models.CertificateJobStatusQueued).
		Updates(map[string]interface{}{
			"status":        models.CertificateJobStatusRunning,
			"attempts":      gorm.Expr("attempts + 1"),
			"started_at":    now,
			"error_message": "",
		})
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected == 0 {
		return nil, gorm.ErrRecordNotFound
	}
	job := &models.CertificateJob{}
	if err := db.First(job, jobID).Error; err != nil {
		return nil, err
	}
	return job, nil
}

func (m *managedCertificateMutation) AdminCompleteCertificateJob(jobID, certificateID uint, userID, tenantID int, status, errorMessage string, issuedAt *time.Time) error {
	if jobID == 0 || certificateID == 0 || userID == 0 || tenantID < 0 {
		return fmt.Errorf("invalid certificate completion")
	}
	if status != models.CertificateJobStatusSuccess && status != models.CertificateJobStatusFailed {
		return fmt.Errorf("invalid certificate job status")
	}
	db := m.ctx.GetApp().GetDBManager().GetDefaultDB()
	now := time.Now()
	return db.Transaction(func(tx *gorm.DB) error {
		var job models.CertificateJob
		if err := tx.Where("id = ? AND certificate_id = ? AND user_id = ? AND tenant_id = ?", jobID, certificateID, userID, tenantID).First(&job).Error; err != nil {
			return err
		}
		updates := map[string]interface{}{
			"status":        status,
			"error_message": strings.TrimSpace(errorMessage),
			"finished_at":   now,
		}
		if err := tx.Model(&job).Updates(updates).Error; err != nil {
			return err
		}
		// A successful Caddy configuration load only proves that the desired
		// routes were accepted. ACME may still be waiting for HTTP-01/TLS-ALPN-01
		// validation, so the certificate remains pending until the Server Agent
		// reports a parsed certificate with real NotBefore/NotAfter metadata.
		certUpdates := map[string]interface{}{"error_message": strings.TrimSpace(errorMessage)}
		if status == models.CertificateJobStatusFailed {
			certUpdates["status"] = models.ManagedCertificateStatusError
		}
		if status == models.CertificateJobStatusSuccess && issuedAt != nil {
			certUpdates["status"] = models.ManagedCertificateStatusReady
			certUpdates["issued_at"] = issuedAt
		}
		return tx.Model(&models.ManagedCertificate{}).Where("id = ? AND user_id = ? AND tenant_id = ?", certificateID, userID, tenantID).Updates(certUpdates).Error
	})
}

// AdminUpdateManagedCertificateStatus stores certificate metadata reported by
// the Server Agent. It is intentionally scoped by the ownership tuple carried
// in the managed certificate record so a node cannot update another tenant's
// certificate by guessing its numeric ID.
func (m *managedCertificateMutation) AdminUpdateManagedCertificateStatus(certificateID uint, userID, tenantID int, status, errorMessage, issuer, fingerprint, storageID string, issuedAt, expiresAt *time.Time) error {
	if certificateID == 0 || userID == 0 || tenantID < 0 {
		return fmt.Errorf("invalid managed certificate status update")
	}
	switch status {
	case models.ManagedCertificateStatusPending, models.ManagedCertificateStatusReady, models.ManagedCertificateStatusRenewing, models.ManagedCertificateStatusExpired, models.ManagedCertificateStatusError, models.ManagedCertificateStatusRevoked:
	default:
		return fmt.Errorf("invalid managed certificate status")
	}
	updates := map[string]interface{}{
		"status":        status,
		"error_message": strings.TrimSpace(errorMessage),
		"issuer":        strings.TrimSpace(issuer),
		"fingerprint":   strings.TrimSpace(fingerprint),
		"storage_id":    strings.TrimSpace(storageID),
		"issued_at":     issuedAt,
		"expires_at":    expiresAt,
	}
	return m.ctx.GetApp().GetDBManager().GetDefaultDB().Model(&models.ManagedCertificate{}).
		Where("id = ? AND user_id = ? AND tenant_id = ?", certificateID, userID, tenantID).
		Updates(updates).Error
}
