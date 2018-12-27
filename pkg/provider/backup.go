package provider

import (
	dbv1alpha1 "github.com/isotoma/db-operator/pkg/apis/db/v1alpha1"
)

type BackupPhase string

const (
	Starting  BackupPhase = "Starting"
	BackingUp BackupPhase = "BackingUp"
	Completed BackupPhase = "Completed"
)

// Backup represents a backup
type Backup struct {
	Namespace string
	Name      string
	backup    dbv1alpha1.Backup
	Database  Database
	Phase     BackupPhase
}

// SetPhase sets the phase on the underlying db-operator resource
func (b *Backup) SetPhase(phase BackupPhase) error {
	return nil
}
