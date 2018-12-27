package provider

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
	Phase DatabasePhase
}

func (d *Database) SetPhase(Phase DatabasePhase) error {
	return nil
}
