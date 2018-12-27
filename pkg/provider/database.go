package provider

import (
	dbv1alpha1 "github.com/isotoma/db-operator/pkg/apis/db/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type DatabasePhase string

const (
	Creating                     DatabasePhase = "Creating"
	Created                      DatabasePhase = "Created"
	BackupRequested              DatabasePhase = "BackupRequested"
	BackupInProgress             DatabasePhase = "BackupInProgress"
	BackupCompleted              DatabasePhase = "BackupCompleted"
	DeletionRequested            DatabasePhase = "DeletionRequested"
	DeletionInProgress           DatabasePhase = "DeletionInProgress"
	Deleted                      DatabasePhase = "Deleted"
	BackupBeforeDeleteRequested  DatabasePhase = "BackupBeforeDeleteRequested"
	BackupBeforeDeleteInProgress DatabasePhase = "BackupBeforeDeleteInProgress"
	BackupBeforeDeleteCompleted  DatabasePhase = "BackupBeforeDeleteCompleted"
)

type Database struct {
	Namespace string
	Name      string
	k8sClient client.Client
	database  dbv1alpha1.Database
	secret    corev1.Secret
	Phase     DatabasePhase
}

func (d *Database) SetPhase(Phase DatabasePhase) error {
	return nil
}
