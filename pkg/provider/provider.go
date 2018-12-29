package provider

import (
	"context"
	"fmt"
	"os"

	dbv1alpha1 "github.com/isotoma/db-operator/pkg/apis/db/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

	"github.com/isotoma/db-operator/pkg/apis"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

type Provider struct {
	k8sclient client.Client
	backup    dbv1alpha1.Backup
	database  dbv1alpha1.Database
	secret    corev1.Secret
	Namespace string
	Database  string
	Backup    string
}

var log = logf.Log.WithName("provider-api")

func (p *Provider) connect() error {
	cfg := config.GetConfigOrDie()
	managerOptions := manager.Options{}
	mgr, err := manager.New(cfg, managerOptions)
	if err != nil {
		return err
	}
	log.Info("Adding APIs to scheme")
	err = apis.AddToScheme(mgr.GetScheme())
	if err != nil {
		return err
	}
	log.Info("Starting cache")
	var stopChan <-chan struct{}
	go func() {
		err := mgr.GetCache().Start(stopChan)
		if err != nil {
			panic(err)
		}
	}()
	mgr.GetCache().WaitForCacheSync(stopChan)
	log.Info("Getting client")
	p.k8sclient = mgr.GetClient()
	return nil
}

func (p *Provider) getResource(name string, dest runtime.Object) error {
	return p.k8sclient.Get(context.TODO(), types.NamespacedName{Namespace: p.Namespace, Name: name}, dest)
}

// Init prepares the Provider API for use and fetches data
func (p *Provider) Init() error {
	if p.Namespace == "" {
		p.Namespace = os.Getenv("DB_OPERATOR_NAMESPACE")
	}
	if p.Database == "" {
		p.Database = os.Getenv("DB_OPERATOR_DATABASE")
	}
	if p.Backup == "" {
		p.Backup = os.Getenv("DB_OPERATOR_BACKUP")
	}
	if err := p.connect(); err != nil {
		return err
	}
	if err := p.getResource(p.Database, &p.database); err != nil {
		return err
	}
	if err := p.getResource(p.Database, &p.secret); err != nil {
		return err
	}
	if err := p.getResource(p.Backup, &p.backup); err != nil {
		return err
	}
	return nil
}

func (p *Provider) readFromKubernetesSecret(s dbv1alpha1.SecretKeyRef) (string, error) {
	return "", nil
}

func (p *Provider) readFromAwsSecret(s dbv1alpha1.AwsSecretRef) (string, error) {
	return "", nil
}

func (p *Provider) getCredential(cred dbv1alpha1.Credential) (string, error) {
	if cred.Value != "" {
		return "", nil
	}
	if cred.ValueFrom.SecretKeyRef.Name != "" {
		return p.readFromKubernetesSecret(cred.ValueFrom.SecretKeyRef)
	}
	if cred.ValueFrom.AwsSecretKeyRef.ARN != "" {
		return p.readFromAwsSecret(cred.ValueFrom.AwsSecretKeyRef)
	}
	return "", fmt.Errorf("No credentials provided")
}

func (p *Provider) getDatabase() (Database, error) {
	var err error
	spec := p.database.Spec
	username, err := p.getCredential(spec.Credentials.Username)
	if err != nil {
		return Database{}, err
	}
	password, err := p.getCredential(spec.Credentials.Password)
	if err != nil {
		return Database{}, err
	}
	backupDestination := BackupNone
	var s3Backup *S3Backup
	if spec.BackupTo.S3.Bucket != "" {
		backupDestination = BackupToS3
		s3Backup = &S3Backup{
			Region: spec.BackupTo.S3.Region,
			Bucket: spec.BackupTo.S3.Bucket,
			Prefix: spec.BackupTo.S3.Prefix,
		}
	}
	return Database{
		Provider:          p,
		Connection:        p.database.Spec.Connect,
		Username:          username,
		Password:          password,
		BackupDestination: backupDestination,
		S3Backup:          s3Backup,
		Phase:             DatabasePhase(p.database.Status.Phase),
	}, nil

}
