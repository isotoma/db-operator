package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

// SecretKeyRef references to a kubernetes secret key
type SecretKeyRef struct {
	GenerateIfNotExists bool `json:"generateIfNotExists,omitempty"`
	Name string `json:"name"`
	Key  string `json:"key"`
}

// AwsSecretRef references a secret in AWS Secrets Manager
type AwsSecretRef struct {
	ARN string `json:"arn"`
	Key string `json:"key"`
}

// ValueFrom supports retrieving a credential from elsewhere
type ValueFrom struct {
	SecretKeyRef    SecretKeyRef `json:"secretKeyRef,omitempty"`
	AwsSecretKeyRef AwsSecretRef `json:"awsSecretKeyRef,omitempty"`
}

// Credential supports either a literal value, or retrieving from elsewhere
type Credential struct {
	Value     string    `json:"value,omitempty"`
	ValueFrom ValueFrom `json:"valueFrom,omitempty"`
}

// Credentials are literal credentials provided in the database resource
type Credentials struct {
	Username Credential `json:"username"`
	Password Credential `json:"password"`
}

// S3Backup provides destination storage for S3 backups
type S3Backup struct {
	Region string `json:"region"`
	Bucket string `json:"bucket"`
	Prefix string `json:"prefix"`
}

// NullBackup indicates there we don't want a backup
type NullBackup struct {
	DoNotBackup	bool `json:"doNotBackup,omitempty"`
}

// BackupTo may support other destinations than S3
type BackupTo struct {
	S3 S3Backup `json:"s3"`
	Null NullBackup `json:"null"`
}

// AwsCredentials are literal AWS credentials used for backups and secrets
type AwsCredentials struct {
	Region          string `json:"region"`
	AccessKeyID     string `json:"accessKeyId"`
	SecretAccessKey string `json:"secretAccessKey"`
}

// DatabaseSpec defines the desired state of Database
type DatabaseSpec struct {
	Provider          string            `json:"provider"`
	Name              string            `json:"name"`
	Connect           map[string]string `json:"connect"`
	MasterCredentials Credentials       `json:"masterCredentials"`
	UserCredentials   Credentials       `json:"userCredentials"`
	BackupTo          BackupTo          `json:"backupTo,omitempty"`
	AwsCredentials    AwsCredentials    `json:"awsCredentials,omitempty"`
}

// DatabaseStatus defines the observed state of Database
type DatabaseStatus struct {
	Phase DatabasePhase `json:"phase"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// Database is the Schema for the databases API
// +k8s:openapi-gen=true
type Database struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DatabaseSpec   `json:"spec,omitempty"`
	Status DatabaseStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// DatabaseList contains a list of Database
type DatabaseList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Database `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Database{}, &DatabaseList{})
}
