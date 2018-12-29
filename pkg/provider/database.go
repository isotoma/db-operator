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

type BackupDestination int

const (
	BackupNone BackupDestination = 0
	BackupToS3 BackupDestination = 1
)

type S3Backup struct {
	Region string
	Bucket string
	Prefix string
}

type Database struct {
	Provider          *Provider
	Connection        map[string]string
	Username          string
	Password          string
	BackupDestination BackupDestination
	S3Backup          *S3Backup
	Phase             DatabasePhase
}

func (d *Database) SetPhase(phase DatabasePhase) error {
	return nil
}
