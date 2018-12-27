package provider

type BackupPhase string

const (
	Starting  BackupPhase = "Starting"
	BackingUp BackupPhase = "BackingUp"
	Completed BackupPhase = "Completed"
)

// Backup represents a backup
type Backup struct {
	Phase BackupPhase
}

// SetPhase sets the phase on the underlying db-operator resource
func (b *Backup) SetPhase(phase BackupPhase) error {
	return nil
}
