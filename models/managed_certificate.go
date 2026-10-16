package models

import (
	"time"

	"gorm.io/gorm"
)

const (
	ManagedCertificateStatusPending  = "pending"
	ManagedCertificateStatusReady    = "ready"
	ManagedCertificateStatusRenewing = "renewing"
	ManagedCertificateStatusExpired  = "expired"
	ManagedCertificateStatusError    = "error"
	ManagedCertificateStatusRevoked  = "revoked"

	CertificateChallengeHTTP01    = "http-01"
	CertificateChallengeTLSALPN01 = "tls-alpn-01"

	CertificateJobActionIssue  = "issue"
	CertificateJobActionRenew  = "renew"
	CertificateJobActionRevoke = "revoke"

	CertificateJobStatusQueued  = "queued"
	CertificateJobStatusRunning = "running"
	CertificateJobStatusSuccess = "success"
	CertificateJobStatusFailed  = "failed"
)

// ManagedCertificate is the control-plane record for a certificate handled by
// Caddy on a target Server. It intentionally contains metadata only; private
// keys and certificate bytes stay in Caddy's persistent storage on that Server.
type ManagedCertificate struct {
	gorm.Model
	UserID        int               `gorm:"index;not null"`
	TenantID      int               `gorm:"index;not null"`
	Name          string            `gorm:"type:varchar(128);not null"`
	Domains       GormArray[string] `gorm:"type:text;not null"`
	PrimaryDomain string            `gorm:"type:varchar(255);index;not null"`
	ServerID      string            `gorm:"type:varchar(255);index;not null"`
	Challenge     string            `gorm:"type:varchar(32);not null"`
	// Keep the API's explicit false value when updating or creating a record.
	// The service layer applies true when the optional request field is omitted;
	// a GORM default tag would otherwise omit false and let SQLite write true.
	AutoRenew    bool   `gorm:"not null"`
	Issuer       string `gorm:"type:varchar(128)"`
	Fingerprint  string `gorm:"type:varchar(128)"`
	StorageID    string `gorm:"type:varchar(255)"`
	Status       string `gorm:"type:varchar(32);index;not null"`
	ErrorMessage string `gorm:"type:text"`
	IssuedAt     *time.Time
	ExpiresAt    *time.Time `gorm:"index"`
}

func (ManagedCertificate) TableName() string { return "managed_certificates" }

// CertificateJob records a requested Caddy operation. The worker that talks
// to Caddy is deliberately separate from this persistence layer.
type CertificateJob struct {
	gorm.Model
	UserID        int    `gorm:"index;not null"`
	TenantID      int    `gorm:"index;not null"`
	CertificateID uint   `gorm:"index;not null"`
	Action        string `gorm:"type:varchar(32);not null"`
	Status        string `gorm:"type:varchar(32);index;not null"`
	ErrorMessage  string `gorm:"type:text"`
	Attempts      uint   `gorm:"not null;default:0"`
	StartedAt     *time.Time
	FinishedAt    *time.Time
}

func (CertificateJob) TableName() string { return "certificate_jobs" }
